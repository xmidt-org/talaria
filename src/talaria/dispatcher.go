package main

import (
	"bytes"
	"context"
	"github.com/Comcast/webpa-common/device"
	"github.com/Comcast/webpa-common/logging"
	"net/http"
	"strings"
	"time"
)

// outboundEnvelope is a tuple of information related to handling an asynchronous HTTP request
type outboundEnvelope struct {
	request *http.Request
	cancel  func()
}

// Dispatcher handles the creation and routing of HTTP requests in response to device events.
// A Dispatcher represents the send side for enqueuing HTTP requests.
type Dispatcher interface {
	// OnDeviceEvent is the device.Listener function that processes outbound events.  Inject
	// this function as a device listener for a manager.
	OnDeviceEvent(*device.Event)
}

// dispatcher is the internal Dispatcher implementation
type dispatcher struct {
	logger                logging.Logger
	urlFilter             URLFilter
	method                string
	timeout               time.Duration
	defaultEventEndpoints []string
	eventEndpoints        map[string][]string
	outbounds             chan<- *outboundEnvelope
}

// NewDispatcher constructs a Dispatcher which sends envelopes via the returned channel.
// The channel may be used to spawn one or more workers to process the envelopes.
func NewDispatcher(o *Outbounder, urlFilter URLFilter) (Dispatcher, <-chan *outboundEnvelope, error) {
	if urlFilter == nil {
		var err error
		urlFilter, err = NewURLFilter(o)
		if err != nil {
			return nil, nil, err
		}
	}

	outbounds := make(chan *outboundEnvelope, o.outboundQueueSize())
	return &dispatcher{
		logger:                o.logger(),
		urlFilter:             urlFilter,
		method:                o.method(),
		timeout:               o.requestTimeout(),
		defaultEventEndpoints: o.defaultEventEndpoints(),
		eventEndpoints:        o.eventEndpoints(),
		outbounds:             outbounds,
	}, outbounds, nil
}

func (d *dispatcher) send(request *http.Request) error {
	var (
		ctx, cancel = context.WithTimeout(request.Context(), d.timeout)
		envelope    = &outboundEnvelope{request.WithContext(ctx), cancel}
	)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case d.outbounds <- envelope:
		return nil
	}
}

func (d *dispatcher) dispatchEvent(eventType string, contents []byte) error {
	endpoints, ok := d.eventEndpoints[eventType]
	if !ok {
		endpoints = d.defaultEventEndpoints
	}

	for _, url := range endpoints {
		request, err := http.NewRequest(d.method, url, bytes.NewReader(contents))
		if err != nil {
			return err
		}

		if err := d.send(request); err != nil {
			return err
		}
	}

	return nil
}

func (d *dispatcher) dispatchTo(unfiltered string, contents []byte) error {
	url, err := d.urlFilter.Filter(unfiltered)
	if err != nil {
		return err
	}

	request, err := http.NewRequest(d.method, url, bytes.NewReader(contents))
	if err != nil {
		return err
	}

	return d.send(request)
}

func (d *dispatcher) OnDeviceEvent(event *device.Event) {
	if event.Type != device.MessageReceived {
		return
	}

	destination := event.Message.To()
	if strings.HasPrefix(destination, EventPrefix) {
		if err := d.dispatchEvent(destination[:len(EventPrefix)], event.Contents); err != nil {
			d.logger.Error("Error dispatching event [%s]: %s", destination, err)
		}
	} else if strings.HasPrefix(destination, DNSPrefix) {
		if err := d.dispatchTo(destination[:len(DNSPrefix)], event.Contents); err != nil {
			d.logger.Error("Error dispatching to [%s]: %s", destination, err)
		}
	} else {
		d.logger.Error("Unable to route to [%s]", destination)
	}
}
