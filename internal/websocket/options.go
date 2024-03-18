// SPDX-FileCopyrightText: 2023 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package websocket

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/xmidt-org/retry"
	"github.com/xmidt-org/sallust"
	"github.com/xmidt-org/wrp-go/v3"
	"github.com/xmidt-org/xmidt-agent/internal/websocket/event"
	"go.uber.org/zap"
)

const (
	expectContinueTimeoutDefault = 1 * time.Second
	idleConnTimeoutDefault       = 10 * time.Second
	tlsHandshakeTimeoutDefault
	fetchUrlTimeoutDefault = 30 * time.Second
	pingIntervalDefault
	connectionTimeoutDefault
	keepAliveIntervalDefault
	pingTimeoutDefault     = 90 * time.Second
	maxMessageBytesDefault = 256 * 1024
)

// DeviceID sets the device ID for the WS connection.
func DeviceID(id wrp.DeviceID) Option {
	return optionFunc(
		func(ws *Websocket) error {
			ws.id = id
			if ws.additionalHeaders == nil {
				ws.additionalHeaders = http.Header{}
			}
			ws.additionalHeaders.Set("X-Webpa-Device-Name", string(id))
			return nil
		})
}

// URL sets the URL for the WS connection.
func URL(url string) Option {
	return optionFunc(
		func(ws *Websocket) error {
			ws.urlFetcher = func(context.Context) (string, error) {
				return url, nil
			}
			return nil
		})
}

// FetchURL sets the FetchURL for the WS connection.
func FetchURL(f func(context.Context) (string, error)) Option {
	return optionFunc(
		func(ws *Websocket) error {
			ws.urlFetcher = f
			return nil
		})
}

// FetchURLTimeout sets the FetchURLTimeout for the WS connection.
// If this is not set, the default is 30 seconds.
func FetchURLTimeout(d time.Duration) Option {
	return optionFunc(
		func(ws *Websocket) error {
			if d < 0 {
				return fmt.Errorf("%w: negative FetchURLTimeout", ErrMisconfiguredWS)
			} else if d == 0 {
				d = fetchUrlTimeoutDefault
			}

			ws.urlFetchingTimeout = d
			return nil
		})
}

// CredentialsDecorator provides the credentials decorator for the WS connection.
func CredentialsDecorator(f func(http.Header) error) Option {
	return optionFunc(
		func(ws *Websocket) error {
			ws.credDecorator = f
			return nil
		})
}

// PingInterval sets the time expected between PINGs for the WS connection.
// If this is not set, the default is 30 seconds.
func PingInterval(d time.Duration) Option {
	return optionFunc(
		func(ws *Websocket) error {
			if d < 0 {
				return fmt.Errorf("%w: negative PingInterval", ErrMisconfiguredWS)
			} else if d == 0 {
				d = pingIntervalDefault
			}

			ws.pingInterval = d
			return nil
		})
}

// PingTimeout sets the maximum time allowed between PINGs for the WS connection
// before the connection is closed.  If this is not set, the default is 90 seconds.
func PingTimeout(d time.Duration) Option {
	return optionFunc(
		func(ws *Websocket) error {
			if d < 0 {
				return fmt.Errorf("%w: negative PingTimeout", ErrMisconfiguredWS)
			} else if d == 0 {
				d = pingTimeoutDefault
			}

			ws.pingTimeout = d
			return nil
		})
}

// KeepAliveInterval sets the keep alive interval for the WS connection.
// If this is not set, the default is 30 seconds.
func KeepAliveInterval(d time.Duration) Option {
	return optionFunc(
		func(ws *Websocket) error {
			if d < 0 {
				return fmt.Errorf("%w: negative KeepAliveInterval", ErrMisconfiguredWS)
			} else if d == 0 {
				d = keepAliveIntervalDefault
			}

			ws.keepAliveInterval = d
			return nil
		})
}

// TLSHandshakeTimeout sets the TLS handshake timeout for the WS connection.
// If this is not set, the default is 10 seconds.
func TLSHandshakeTimeout(d time.Duration) Option {
	return optionFunc(
		func(ws *Websocket) error {
			if d < 0 {
				return fmt.Errorf("%w: negative TLSHandshakeTimeout", ErrMisconfiguredWS)
			} else if d == 0 {
				d = tlsHandshakeTimeoutDefault
			}

			ws.tlsHandshakeTimeout = d
			return nil
		})
}

// IdleConnTimeout sets the idle connection timeout for the WS connection.
// If this is not set, the default is 10 seconds.
func IdleConnTimeout(d time.Duration) Option {
	return optionFunc(
		func(ws *Websocket) error {
			if d < 0 {
				return fmt.Errorf("%w: negative IdleConnTimeout", ErrMisconfiguredWS)
			} else if d == 0 {
				d = idleConnTimeoutDefault
			}

			ws.idleConnTimeout = d
			return nil
		})
}

// ExpectContinueTimeout sets the expect continue timeout for the WS connection.
// If this is not set, the default is 1 second.
func ExpectContinueTimeout(d time.Duration) Option {
	return optionFunc(
		func(ws *Websocket) error {
			if d < 0 {
				return fmt.Errorf("%w: negative ExpectContinueTimeout", ErrMisconfiguredWS)
			} else if d == 0 {
				d = expectContinueTimeoutDefault
			}

			ws.expectContinueTimeout = d
			return nil
		})
}

// WithIPv4 sets whether or not to allow IPv4 for the WS connection.  If this
// is not set, the default is true.
func WithIPv4(with ...bool) Option {
	with = append(with, true)
	return optionFunc(
		func(ws *Websocket) error {
			ws.withIPv4 = with[0]
			return nil
		})
}

// WithIPv6 sets whether or not to allow IPv6 for the WS connection.  If this
// is not set, the default is true.
func WithIPv6(with ...bool) Option {
	with = append(with, true)
	return optionFunc(
		func(ws *Websocket) error {
			ws.withIPv6 = with[0]
			return nil
		})
}

// ConnectTimeout sets the timeout for the WS connection.  If this is not set,
// the default is 30 seconds.
func ConnectTimeout(d time.Duration) Option {
	return optionFunc(
		func(ws *Websocket) error {
			if d < 0 {
				return fmt.Errorf("%w: negative ConnectTimeout", ErrMisconfiguredWS)
			} else if d == 0 {
				d = connectionTimeoutDefault
			}

			ws.connectTimeout = d
			return nil
		})
}

// AdditionalHeaders sets the additional headers for the WS connection.
func AdditionalHeaders(headers http.Header) Option {
	return optionFunc(
		func(ws *Websocket) error {
			for k, values := range headers {
				for _, value := range values {
					ws.additionalHeaders.Add(k, value)
				}
			}
			return nil
		})
}

// Once sets whether or not to only attempt to connect once.
func Once(once ...bool) Option {
	once = append(once, true)
	return optionFunc(
		func(ws *Websocket) error {
			ws.once = once[0]
			return nil
		})
}

// Logger sets the zap logger.
func Logger(l *zap.Logger) Option {
	return optionFunc(
		func(ws *Websocket) error {
			if l == nil {
				l = sallust.Default()
			}

			ws.l = l
			return nil
		})
}

// NowFunc sets the now function for the WS connection.
func NowFunc(f func() time.Time) Option {
	return optionFunc(
		func(ws *Websocket) error {
			if f == nil {
				return fmt.Errorf("%w: nil NowFunc", ErrMisconfiguredWS)
			}

			ws.nowFunc = f
			return nil
		})
}

// RetryPolicy sets the retry policy factory used for delaying between retry
// attempts for reconnection.
func RetryPolicy(pf retry.PolicyFactory) Option {
	return optionFunc(
		func(ws *Websocket) error {
			ws.retryPolicyFactory = pf
			return nil
		})
}

// MaxMessageBytes sets the maximum message size sent or received in bytes.
func MaxMessageBytes(bytes int64) Option {
	return optionFunc(
		func(ws *Websocket) error {
			if bytes == 0 {
				bytes = maxMessageBytesDefault
			}

			ws.maxMessageBytes = bytes
			return nil
		})
}

// AddMessageListener adds a message listener to the WS connection.
// The listener will be called for every message received from the WS.
func AddMessageListener(listener event.MsgListener, cancel ...*event.CancelFunc) Option {
	return optionFunc(
		func(ws *Websocket) error {
			var ignored event.CancelFunc
			cancel = append(cancel, &ignored)
			*cancel[0] = event.CancelFunc(ws.msgListeners.Add(listener))
			return nil
		})
}

// AddConnectListener adds a connect listener to the WS connection.
func AddConnectListener(listener event.ConnectListener, cancel ...*event.CancelFunc) Option {
	return optionFunc(
		func(ws *Websocket) error {
			var ignored event.CancelFunc
			cancel = append(cancel, &ignored)
			*cancel[0] = event.CancelFunc(ws.connectListeners.Add(listener))
			return nil
		})
}

// AddDisconnectListener adds a disconnect listener to the WS connection.
func AddDisconnectListener(listener event.DisconnectListener, cancel ...*event.CancelFunc) Option {
	return optionFunc(
		func(ws *Websocket) error {
			var ignored event.CancelFunc
			cancel = append(cancel, &ignored)
			*cancel[0] = event.CancelFunc(ws.disconnectListeners.Add(listener))
			return nil
		})
}

// AddHeartbeatListener adds a heartbeat listener to the WS connection.
func AddHeartbeatListener(listener event.HeartbeatListener, cancel ...*event.CancelFunc) Option {
	return optionFunc(
		func(ws *Websocket) error {
			var ignored event.CancelFunc
			cancel = append(cancel, &ignored)
			*cancel[0] = event.CancelFunc(ws.heartbeatListeners.Add(listener))
			return nil
		})
}
