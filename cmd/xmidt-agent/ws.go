package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/xmidt-org/wrp-go/v3"
	"github.com/xmidt-org/wrp-go/v3/wrphttp"
	"github.com/xmidt-org/xmidt-agent/internal/websocket"
	"github.com/xmidt-org/xmidt-agent/internal/websocket/event"
	"go.uber.org/fx"
	"go.uber.org/zap"
)

var (
	ErrClientConfig = errors.New("client configuration error")
)

type wsIn struct {
	fx.In
	DeviceID wrp.DeviceID
	Logger   *zap.Logger
	CLI      *CLI
	Client   Client
}

type wsOut struct {
	fx.Out
	WS         *websocket.Websocket
	CancelList []event.CancelFunc
}

func provideWS(in wsIn) (wsOut, error) {
	opts := []websocket.Option{
		websocket.DeviceID(in.DeviceID),
		websocket.FetchURL(func(ctx context.Context) (string, error) {
			client := &http.Client{
				CheckRedirect: func(req *http.Request, via []*http.Request) error {
					return http.ErrUseLastResponse
				},
			}
			req, err := http.NewRequestWithContext(ctx, "GET", in.Client.FetchURL, nil)
			if err != nil {
				return "", fmt.Errorf("failed to fetch ws url: %s", err)
			}

			req.Header.Set(wrphttp.DestinationHeader, string(in.DeviceID.Bytes()))
			resp, err := client.Do(req)
			if err != nil {
				return "", fmt.Errorf("failed to fetch ws url: %s: %s", getDoErrReason(err), err)
			}

			defer resp.Body.Close()
			if resp.StatusCode != http.StatusTemporaryRedirect {
				respBody, _ := io.ReadAll(resp.Body)
				return "", fmt.Errorf("failed to fetch ws url: unexpected status code (expected: %d): %d: %s", http.StatusTemporaryRedirect, resp.StatusCode, respBody)
			}

			url, err := resp.Location()
			if err != nil {
				return "", fmt.Errorf("failed to fetch ws url: %s", err)
			}

			return url.String(), nil
		}),
		websocket.RetryPolicy(in.Client.RetryPolicy),
		websocket.Logger(in.Logger),
		websocket.NowFunc(time.Now),
		websocket.FetchURLTimeout(in.Client.FetchURLTimeout),
		websocket.PingInterval(in.Client.PingInterval),
		websocket.PingTimeout(in.Client.PingTimeout),
		websocket.ConnectTimeout(in.Client.ConnectTimeout),
		websocket.KeepAliveInterval(in.Client.KeepAliveInterval),
		websocket.IdleConnTimeout(in.Client.IdleConnTimeout),
		websocket.TLSHandshakeTimeout(in.Client.TLSHandshakeTimeout),
		websocket.ExpectContinueTimeout(in.Client.ExpectContinueTimeout),
		websocket.MaxMessageBytes(in.Client.MaxMessageBytes),
		websocket.WithIPv6(!in.Client.DisableV6),
		websocket.WithIPv4(!in.Client.DisableV4),
		websocket.Once(in.Client.Once),
	}

	var msg, con, discon event.CancelFunc
	if in.CLI.Dev {
		opts = append(opts,
			websocket.AddMessageListener(
				event.MsgListenerFunc(
					func(m wrp.Message) {
						in.Logger.Info("message listener", zap.Any("msg", m))
					}), &msg),
			websocket.AddConnectListener(
				event.ConnectListenerFunc(
					func(e event.Connect) {
						in.Logger.Info("connect listener", zap.Any("event", e))
					}), &con),
			websocket.AddDisconnectListener(
				event.DisconnectListenerFunc(
					func(e event.Disconnect) {
						in.Logger.Info("disconnect listener", zap.Any("event", e))
					}), &discon),
		)
	}

	if in.Client.FetchURL == "" {
		return wsOut{}, fmt.Errorf("%w: client FetchURL can't be empty", ErrClientConfig)
	}

	ws, err := websocket.New(opts...)
	return wsOut{
		WS:         ws,
		CancelList: []event.CancelFunc{msg, con, discon},
	}, err
}

const (
	genericDoReason                       = "do_error"
	deadlineExceededReason                = "context_deadline_exceeded"
	contextCanceledReason                 = "context_canceled"
	addressErrReason                      = "address_error"
	parseAddrErrReason                    = "parse_address_error"
	invalidAddrReason                     = "invalid_address"
	dnsErrReason                          = "dns_error"
	hostNotFoundReason                    = "host_not_found"
	connClosedReason                      = "connection_closed"
	opErrReason                           = "op_error"
	networkErrReason                      = "unknown_network_err"
	connectionUnexpectedlyClosedEOFReason = "connection_unexpectedly_closed_eof"
	noErrReason                           = "no_err"
)

func getDoErrReason(err error) string {
	var d *net.DNSError
	if err == nil {
		return noErrReason
	} else if errors.Is(err, context.DeadlineExceeded) {
		return deadlineExceededReason
	} else if errors.Is(err, context.Canceled) {
		return contextCanceledReason
	} else if errors.Is(err, &net.AddrError{}) {
		return addressErrReason
	} else if errors.Is(err, &net.ParseError{}) {
		return parseAddrErrReason
	} else if errors.Is(err, net.InvalidAddrError("")) {
		return invalidAddrReason
	} else if errors.As(err, &d) {
		if d.IsNotFound {
			return hostNotFoundReason
		}
		return dnsErrReason
	} else if errors.Is(err, net.ErrClosed) {
		return connClosedReason
	} else if errors.Is(err, &net.OpError{}) {
		return opErrReason
	} else if errors.Is(err, net.UnknownNetworkError("")) {
		return networkErrReason
	}

	// nolint: errorlint
	if err, ok := err.(*url.Error); ok {
		if strings.TrimSpace(strings.ToLower(err.Unwrap().Error())) == "eof" {
			return connectionUnexpectedlyClosedEOFReason
		}
	}

	return genericDoReason
}
