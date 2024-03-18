// SPDX-FileCopyrightText: 2023 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package websocket

import (
	"context"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/xmidt-org/eventor"
	"github.com/xmidt-org/retry"
	"github.com/xmidt-org/wrp-go/v3"
	"github.com/xmidt-org/xmidt-agent/internal/websocket/event"
	"go.uber.org/zap"
	nhws "nhooyr.io/websocket"
)

var (
	ErrMisconfiguredWS = errors.New("misconfigured WS")
	ErrClosed          = errors.New("websocket closed")
	ErrInvalidMsgType  = errors.New("invalid message type")
	ErrPing            = errors.New("ws ping failed")
)

// emptyBuffer is solely used as an address of a global empty buffer.
// This sentinel value will reset pointers of the writePump's encoder
// such that the gc can clean things up.
var emptyBuffer = []byte{}

type Websocket struct {
	// id is the device ID for the WS connection.
	id wrp.DeviceID

	// urlFetcher is the URLFetcher for the WS connection.
	urlFetcher func(context.Context) (string, error)

	// urlFetchingTimeout is the URLFetchingTimeout for the WS connection.
	urlFetchingTimeout time.Duration

	// credDecorator is the credentials decorator for the WS connection.
	credDecorator func(http.Header) error

	// pingInterval is the ping interval allowed for the WS connection.
	pingInterval time.Duration

	// pingTimeout is the ping timeout for the WS connection.
	pingTimeout time.Duration

	// connectTimeout is the connect timeout for the WS connection.
	connectTimeout time.Duration

	// keepAliveInterval is the keep alive interval for the WS connection.
	keepAliveInterval time.Duration

	// idleConnTimeout is the idle connection timeout for the WS connection.
	idleConnTimeout time.Duration

	// tlsHandshakeTimeout is the TLS handshake timeout for the WS connection.
	tlsHandshakeTimeout time.Duration

	// expectContinueTimeout is the expect continue timeout for the WS connection.
	expectContinueTimeout time.Duration

	// additionalHeaders are any additional headers for the WS connection.
	additionalHeaders http.Header

	// maxMessageBytes is the largest allowable message to send or receive.
	maxMessageBytes int64

	// withIPv4 is whether or not to allow IPv4 for the WS connection.
	withIPv4 bool

	// withIPv6 is whether or not to allow IPv6 for the WS connection.
	withIPv6 bool

	// connectListeners are the connect listeners for the WS connection.
	connectListeners eventor.Eventor[event.ConnectListener]

	// disconnectListeners are the disconnect listeners for the WS connection.
	disconnectListeners eventor.Eventor[event.DisconnectListener]

	// heartbeatListeners are the heartbeat listeners for the WS connection.
	heartbeatListeners eventor.Eventor[event.HeartbeatListener]

	// msgListeners are the message listeners for messages from the WS.
	msgListeners eventor.Eventor[event.MsgListener]

	// nowFunc is the now function for the WS connection.
	nowFunc func() time.Time

	// retryPolicyFactory is the retry policy factory for the WS connection.
	retryPolicyFactory retry.PolicyFactory

	// once is whether or not to only attempt to connect once.
	once bool

	m        sync.Mutex
	wg       sync.WaitGroup
	shutdown context.CancelFunc
	l        *zap.Logger

	conn *nhws.Conn
}

// Option is a functional option type for WS.
type Option interface {
	apply(*Websocket) error
}

type optionFunc func(*Websocket) error

func (f optionFunc) apply(c *Websocket) error {
	return f(c)
}

// New creates a new WS connection with the given options.
func New(opts ...Option) (*Websocket, error) {
	var ws Websocket

	defaults := []Option{
		NowFunc(time.Now),
		FetchURLTimeout(fetchUrlTimeoutDefault),
		PingInterval(pingIntervalDefault),
		PingTimeout(pingTimeoutDefault),
		ConnectTimeout(connectionTimeoutDefault),
		KeepAliveInterval(keepAliveIntervalDefault),
		IdleConnTimeout(idleConnTimeoutDefault),
		TLSHandshakeTimeout(tlsHandshakeTimeoutDefault),
		ExpectContinueTimeout(expectContinueTimeoutDefault),
		MaxMessageBytes(maxMessageBytesDefault),
		WithIPv4(),
		WithIPv6(),
		Once(false),

		/*
			This retry policy gives us a very good approximation of the prior
			policy.  The important things about this policy are:

			1. The backoff increases up to the max.
			2. There is jitter that spreads the load so windows do not overlap.

			iteration | parodus   | this implementation
			----------+-----------+----------------
			0         | 0-1s      |   0.666 -  1.333
			1         | 1s-3s     |   1.333 -  2.666
			2         | 3s-7s     |   2.666 -  5.333
			3         | 7s-15s    |   5.333 -  10.666
			4         | 15s-31s   |  10.666 -  21.333
			5         | 31s-63s   |  21.333 -  42.666
			6         | 63s-127s  |  42.666 -  85.333
			7         | 127s-255s |  85.333 - 170.666
			8         | 255s-511s | 170.666 - 341.333
			9         | 255s-511s |           341.333
			n         | 255s-511s |           341.333
		*/
		RetryPolicy(&retry.Config{
			Interval:    time.Second,
			Multiplier:  2.0,
			Jitter:      1.0 / 3.0,
			MaxInterval: 341*time.Second + 333*time.Millisecond,
		}),
		WithIPv4(),
		WithIPv6(),
	}

	opts = append(defaults, opts...)

	opts = append(opts,
		validateDeviceID(),
		validateURL(),
		validateIPMode(),
	)

	for _, opt := range opts {
		if opt != nil {
			if err := opt.apply(&ws); err != nil {
				return nil, err
			}
		}
	}

	return &ws, nil
}

// Start starts the websocket connection and a long running goroutine to maintain
// the connection.
func (ws *Websocket) Start() {
	ws.m.Lock()
	defer ws.m.Unlock()

	if ws.shutdown != nil {
		return
	}

	var ctx context.Context
	ctx, ws.shutdown = context.WithCancel(context.Background())

	go ws.run(ctx)
}

// Stop stops the websocket connection.
func (ws *Websocket) Stop() {
	ws.m.Lock()
	shutdown := ws.shutdown
	ws.m.Unlock()

	if shutdown != nil {
		shutdown()
	}

	ws.wg.Wait()
}

// Send sends the provided WRP message through the existing websocket.  This
// call synchronously blocks until the write is complete.
func (ws *Websocket) Send(ctx context.Context, msg wrp.Message) error {
	err := ErrClosed

	ws.m.Lock()
	if ws.conn != nil {
		err = ws.conn.Write(ctx, nhws.MessageBinary, wrp.MustEncode(&msg, wrp.Msgpack))
	}
	ws.m.Unlock()

	return err
}

func (ws *Websocket) run(ctx context.Context) {
	ws.wg.Add(1)
	defer ws.wg.Done()

	pingTicker := time.NewTicker(ws.pingInterval)
	decoder := wrp.NewDecoder(nil, wrp.Msgpack)
	encoder := wrp.NewEncoder(nil, wrp.Msgpack)
	mode := ws.nextMode(ipv4)

	policy := ws.retryPolicyFactory.NewPolicy(ctx)

	for {
		var next time.Duration

		mode = ws.nextMode(mode)
		cEvent := event.Connect{
			Started: ws.nowFunc(),
			Mode:    mode.ToEvent(),
		}

		conn, _, dialErr := ws.dial(ctx, mode) //nolint:bodyclose
		cEvent.At = ws.nowFunc()

		if dialErr == nil {
			ws.connectListeners.Visit(func(l event.ConnectListener) {
				l.OnConnect(cEvent)
			})

			// Reset the retry policy on a successful connection.
			policy = ws.retryPolicyFactory.NewPolicy(ctx)

			// Store the connection so writing can take place.
			ws.m.Lock()
			ws.conn = conn
			ws.m.Unlock()

			// Read loop
			for {
				var msg wrp.Message
				typ, reader, err := ws.conn.Reader(ctx)
				if err == nil {
					if typ != nhws.MessageBinary {
						err = ErrInvalidMsgType
					} else {
						decoder.Reset(reader)
						err = decoder.Decode(&msg)
					}
				}

				if err != nil {
					ws.m.Lock()
					ws.conn = nil
					ws.m.Unlock()

					// The websocket gave us an unexpected message, or a message
					// that could not be decoded.  Close & reconnect.
					_ = conn.Close(nhws.StatusUnsupportedData, limit(err.Error()))

					dEvent := event.Disconnect{
						At:  ws.nowFunc(),
						Err: err,
					}
					ws.disconnectListeners.Visit(func(l event.DisconnectListener) {
						l.OnDisconnect(dEvent)
					})

					break
				}

				ws.msgListeners.Visit(func(l event.MsgListener) {
					l.OnMessage(msg)
				})
				var frameContents []byte
				// nolint: typecheck

				// if the request was in a format other than Msgpack, or if the caller did not pass
				// Contents, then do the encoding here.
				encoder.ResetBytes(&frameContents)
				err = encoder.Encode(msg)
				encoder.ResetBytes(&emptyBuffer)
				if err != nil {
					ws.l.Error("xmidt-agent failed to response to wrp message", zap.Error(err))
					continue
				}

				ws.conn.Write(ctx, nhws.MessageBinary, frameContents)
			}
		}

		if ws.once {
			return
		}

		next, _ = policy.Next()

		if dialErr != nil {
			cEvent.Err = dialErr
			cEvent.RetryingAt = ws.nowFunc().Add(next)
			ws.connectListeners.Visit(func(l event.ConnectListener) {
				l.OnConnect(cEvent)
			})
		}

		select {
		case <-time.After(next):
		case <-ctx.Done():
			return
		}
	}
}

func (ws *Websocket) dial(ctx context.Context, mode ipMode) (*nhws.Conn, *http.Response, error) {
	fetchCtx, cancel := context.WithTimeout(ctx, ws.urlFetchingTimeout)
	defer cancel()
	url, err := ws.urlFetcher(fetchCtx)
	if err != nil {
		return nil, nil, err
	}

	conn, resp, err := nhws.Dial(ctx, url,
		&nhws.DialOptions{
			HTTPHeader: ws.additionalHeaders,
			HTTPClient: &http.Client{
				Transport: ws.getRT(mode),
				Timeout:   ws.connectTimeout,
			},
		},
	)
	if err != nil {
		return nil, resp, err
	}

	conn.SetReadLimit(ws.maxMessageBytes)
	return conn, resp, err
}

type custRT struct {
	transport http.Transport
}

func (rt *custRT) RoundTrip(r *http.Request) (*http.Response, error) {
	return rt.transport.RoundTrip(r)
}

// getRT returns a custom RoundTripper for the WS connection.
func (ws *Websocket) getRT(mode ipMode) *custRT {
	dialer := &net.Dialer{
		Timeout:   ws.connectTimeout,
		KeepAlive: ws.keepAliveInterval,
		DualStack: false,
	}

	return &custRT{
		transport: http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialer.DialContext(ctx, string(mode), addr)
			},
			MaxIdleConns:          1,
			MaxIdleConnsPerHost:   1,
			MaxConnsPerHost:       1,
			IdleConnTimeout:       ws.idleConnTimeout,
			TLSHandshakeTimeout:   ws.tlsHandshakeTimeout,
			ExpectContinueTimeout: ws.expectContinueTimeout,
		},
	}
}

func (ws *Websocket) nextMode(mode ipMode) ipMode {
	if mode == ipv4 && ws.withIPv6 {
		return ipv6
	}

	if mode == ipv6 && ws.withIPv4 {
		return ipv4
	}

	return mode
}

func limit(s string) string {
	if len(s) > 125 {
		return s[:125]
	}
	return s
}
