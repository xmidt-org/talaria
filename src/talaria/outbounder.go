package main

import (
	"bytes"
	"context"
	"fmt"
	"github.com/Comcast/webpa-common/device"
	"github.com/Comcast/webpa-common/logging"
	"github.com/Comcast/webpa-common/wrp"
	"github.com/spf13/viper"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"time"
)

const (
	// OutbounderKey is the Viper subkey which is expected to hold Outbounder configuration
	OutbounderKey = "device.outbound"

	EventPrefix = "event:"
	URLPrefix   = "url:"

	DefaultMethod                            = "POST"
	DefaultEventEndpoint                     = "http://localhost:8090/api/v2/notify"
	DefaultAssumeScheme                      = "https"
	DefaultAllowedScheme                     = "https"
	DefaultWorkerPoolSize                    = 100
	DefaultOutboundQueueSize                 = 1000
	DefaultRequestTimeout      time.Duration = 15 * time.Second
	DefaultClientTimeout       time.Duration = 3 * time.Second
	DefaultMaxIdleConns                      = 0
	DefaultMaxIdleConnsPerHost               = 100
	DefaultIdleConnTimeout     time.Duration = 0
)

// outboundEnvelope is a tuple of information related to handling an asynchronous HTTP request
type outboundEnvelope struct {
	request *http.Request
	cancel  func()
}

// RequestFactory is a simple function type for creating an outbound HTTP request
// for a given WRP message.  This factory function type wraps the created HTTP request in
// an envelope with other data related to cancellation and queue management.
type requestFactory func(device.Interface, wrp.Routable, []byte) (*outboundEnvelope, error)

// listener is the internal device Listener type that dispatches requests over HTTP.
type listener struct {
	logger         logging.Logger
	outbounds      chan<- *outboundEnvelope
	requestFactory requestFactory
}

func (l *listener) OnDeviceEvent(e *device.Event) {
	if e.Type != device.MessageReceived {
		return
	}

	outbound, err := l.requestFactory(e.Device, e.Message, e.Contents)
	if err != nil {
		l.logger.Error("Unable to create request for device [%s]: %s", e.Device.ID(), err)
		return
	}

	select {
	case l.outbounds <- outbound:
	default:
		l.logger.Error("Dropping outbound message for device [%s]: %s->%s", e.Device.ID(), e.Message.From(), e.Message.To())
	}
}

// workerPool describes a pool of goroutines that dispatch http.Request objects to
// a transactor function
type workerPool struct {
	logger     logging.Logger
	outbounds  <-chan *outboundEnvelope
	transactor func(*http.Request) (*http.Response, error)
}

func (wp *workerPool) send(outbound *outboundEnvelope) {
	defer outbound.cancel()

	response, err := wp.transactor(outbound.request)
	if err != nil {
		wp.logger.Error("HTTP error: %s", err)
		return
	}

	if response.StatusCode < 400 {
		wp.logger.Debug("HTTP response status: %s", response.Status)
	} else {
		wp.logger.Error("HTTP response status: %s", response.Status)
	}

	io.Copy(ioutil.Discard, response.Body)
	response.Body.Close()
}

func (wp *workerPool) worker() {
	for outbound := range wp.outbounds {
		wp.send(outbound)
	}
}

func (wp *workerPool) run(workers int) {
	for repeat := 0; repeat < workers; repeat++ {
		go wp.worker()
	}
}

// Outbounder acts as a configurable endpoint for dispatching WRP messages from devices
// and handling any failed messages.
type Outbounder struct {
	Method              string
	EventEndpoint       string
	DeviceNameHeader    string
	AssumeScheme        string
	AllowedSchemes      []string
	WorkerPoolSize      int
	OutboundQueueSize   int
	RequestTimeout      time.Duration
	ClientTimeout       time.Duration
	MaxIdleConns        int
	MaxIdleConnsPerHost int
	IdleConnTimeout     time.Duration
	Logger              logging.Logger
}

// NewOutbounder returns an Outbounder unmarshalled from a Viper environment.
// This function allows the Viper instance to be nil, in which case a default
// Outbounder is returned.
func NewOutbounder(logger logging.Logger, v *viper.Viper) (o *Outbounder, err error) {
	o = &Outbounder{
		Method:              DefaultMethod,
		EventEndpoint:       DefaultEventEndpoint,
		DeviceNameHeader:    device.DefaultDeviceNameHeader,
		AssumeScheme:        DefaultAssumeScheme,
		AllowedSchemes:      []string{DefaultAllowedScheme},
		WorkerPoolSize:      DefaultWorkerPoolSize,
		OutboundQueueSize:   DefaultOutboundQueueSize,
		RequestTimeout:      DefaultRequestTimeout,
		ClientTimeout:       DefaultClientTimeout,
		MaxIdleConns:        DefaultMaxIdleConns,
		MaxIdleConnsPerHost: DefaultMaxIdleConnsPerHost,
		IdleConnTimeout:     DefaultIdleConnTimeout,
		Logger:              logger,
	}

	if v != nil {
		err = v.Unmarshal(o)
	}

	return
}

// newRoundTripper creates an HTTP RoundTripper (transport) using this Outbounder's configuration.
func (o *Outbounder) newRoundTripper() http.RoundTripper {
	return &http.Transport{
		MaxIdleConns:        o.MaxIdleConns,
		MaxIdleConnsPerHost: o.MaxIdleConnsPerHost,
		IdleConnTimeout:     o.IdleConnTimeout,
	}
}

// newTransactor returns a closure which can execute HTTP transactions
func (o *Outbounder) newTransactor() func(*http.Request) (*http.Response, error) {
	client := &http.Client{
		Transport: o.newRoundTripper(),
		Timeout:   o.ClientTimeout,
	}

	return client.Do
}

// newRequestFactory produces a RequestFactory function that creates an outbound HTTP request
// for a given WRP message from a specific device.
func (o *Outbounder) newRequestFactory() requestFactory {
	allowedSchemes := make(map[string]bool, len(o.AllowedSchemes))
	for _, scheme := range o.AllowedSchemes {
		allowedSchemes[scheme] = true
	}

	return func(d device.Interface, m wrp.Routable, c []byte) (outbound *outboundEnvelope, err error) {
		destination := m.To()
		var request *http.Request
		if strings.HasPrefix(destination, EventPrefix) {
			// route this to the configured endpoint that receives all events
			request, err = http.NewRequest(o.Method, o.EventEndpoint, bytes.NewReader(c))
		} else if strings.HasPrefix(destination, URLPrefix) {
			// route this to the given URL, subject to some validation
			if request, err = http.NewRequest(o.Method, destination[len(URLPrefix):], bytes.NewReader(c)); err == nil {
				if len(request.URL.Scheme) == 0 {
					// if no scheme is supplied, use the configured AssumeScheme
					request.URL.Scheme = o.AssumeScheme
				} else if !allowedSchemes[request.URL.Scheme] {
					err = fmt.Errorf("Scheme not allowed: %s", request.URL.Scheme)
				}
			}
		} else {
			err = fmt.Errorf("Bad WRP destination: %s", destination)
		}

		if err != nil {
			return
		}

		request.Header.Set(o.DeviceNameHeader, string(d.ID()))
		request.Header.Set("Content-Type", wrp.Msgpack.ContentType())
		// TODO: Need to set Convey?

		ctx, cancel := context.WithTimeout(request.Context(), o.RequestTimeout)
		outbound = &outboundEnvelope{
			request: request.WithContext(ctx),
			cancel:  cancel,
		}

		return
	}
}

// Start spawns all necessary goroutines and returns a device.Listener
func (o *Outbounder) Start() device.Listener {
	var (
		outbounds = make(chan *outboundEnvelope, o.OutboundQueueSize)

		workerPool = workerPool{
			logger:     o.Logger,
			outbounds:  outbounds,
			transactor: o.newTransactor(),
		}

		listener = &listener{
			logger:         o.Logger,
			requestFactory: o.newRequestFactory(),
			outbounds:      outbounds,
		}
	)

	workerPool.run(o.WorkerPoolSize)
	return listener.OnDeviceEvent
}
