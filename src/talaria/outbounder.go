package main

import (
	"github.com/Comcast/webpa-common/device"
	"github.com/Comcast/webpa-common/logging"
	"github.com/spf13/viper"
	"io"
	"io/ioutil"
	"net/http"
	"time"
)

const (
	// OutbounderKey is the Viper subkey which is expected to hold Outbounder configuration
	OutbounderKey = "device.outbound"

	EventPrefix                 = "event:"
	URLPrefix                   = "url:"
	DefaultDefaultEventEndpoint = "http://localhost:8090/api/v2/notify"
	DefaultAssumeScheme         = "https"
	DefaultAllowedScheme        = "https"

	DefaultMethod                            = "POST"
	DefaultWorkerPoolSize                    = 100
	DefaultOutboundQueueSize                 = 1000
	DefaultRequestTimeout      time.Duration = 15 * time.Second
	DefaultClientTimeout       time.Duration = 3 * time.Second
	DefaultMaxIdleConns                      = 0
	DefaultMaxIdleConnsPerHost               = 100
	DefaultIdleConnTimeout     time.Duration = 0
)

// listener is the internal device Listener type that dispatches requests over HTTP.
type listener struct {
	logger          logging.Logger
	outbounds       chan<- *outboundEnvelope
	envelopeFactory envelopeFactory
}

func (l *listener) onDeviceEvent(e *device.Event) {
	if e.Type != device.MessageReceived {
		return
	}

	envelopes, err := l.envelopeFactory(e.Message.To(), e.Contents)
	if err != nil {
		l.logger.Error("Unable to create requests for device [%s]: %s", e.Device.ID(), err)
		return
	}

	for _, envelope := range envelopes {
		select {
		case <-envelope.done():
			l.logger.Error("Dropping outbound message for device [%s]: %s->%s", e.Device.ID(), e.Message.From(), e.Message.To())
			envelope.cancel()
		case l.outbounds <- envelope:
		}
	}
}

// workerPool describes a pool of goroutines that dispatch http.Request objects to
// a transactor function
type workerPool struct {
	logger     logging.Logger
	outbounds  <-chan *outboundEnvelope
	transactor func(*http.Request) (*http.Response, error)
}

// transact performs all the logic necessary to fulfill an outbound request.
// This method ensures that the Context associated with the request is properly cancelled.
func (wp *workerPool) transact(e *outboundEnvelope) {
	defer e.cancel()

	response, err := wp.transactor(e.request)
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
	for e := range wp.outbounds {
		wp.transact(e)
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
	Routing             Routing
	WorkerPoolSize      int
	OutboundQueueSize   int
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
		Routing: Routing{
			Method:                DefaultMethod,
			Timeout:               DefaultRequestTimeout,
			AssumeScheme:          DefaultAssumeScheme,
			AllowedSchemes:        []string{DefaultAllowedScheme},
			DefaultEventEndpoints: []string{DefaultDefaultEventEndpoint},
		},
		WorkerPoolSize:      DefaultWorkerPoolSize,
		OutboundQueueSize:   DefaultOutboundQueueSize,
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
			logger:          o.Logger,
			outbounds:       outbounds,
			envelopeFactory: o.Routing.NewEnvelopeFactory(),
		}
	)

	workerPool.run(o.WorkerPoolSize)
	return listener.onDeviceEvent
}
