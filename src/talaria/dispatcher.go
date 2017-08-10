package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Comcast/webpa-common/device"
	"github.com/Comcast/webpa-common/logging"
	"github.com/Comcast/webpa-common/wrp"
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
	logger            logging.Logger
	urlFilter         URLFilter
	method            string
	timeout           time.Duration
	authorizationKeys []string
	eventEndpoints    map[string][]string
	outbounds         chan<- *outboundEnvelope
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
		logger:            o.logger(),
		urlFilter:         urlFilter,
		method:            o.method(),
		timeout:           o.requestTimeout(),
		authorizationKeys: o.authKey(),
		eventEndpoints:    o.eventEndpoints(),
		outbounds:         outbounds,
	}, outbounds, nil
}

// send wraps the given request in an outboundEnvelope together with a cancellable context,
// then asynchronously sends that request to the outbounds channel.  This method will
// block on the outbound channel only as long as the context is not cancelled, i.e. does not time out.
// If the context is cancelled before the envelope can be queued, this method drops the message
// and returns an error.
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

// newRequest creates a basic HTTP request appropriate for this dispatcher
func (d *dispatcher) newRequest(url, contentType string, body io.Reader) (*http.Request, error) {
	request, err := http.NewRequest(d.method, url, body)
	if err == nil {
		request.Header.Set("Content-Type", contentType)

		// TODO: Need to work out how to handle authorization better, without basic auth
		if len(d.authorizationKeys) > 0 {
			request.Header.Set("Authorization", "Basic "+d.authorizationKeys[0])
		}
	}

	return request, err
}

func (d *dispatcher) dispatchEvent(eventType, contentType string, contents []byte) error {
	endpoints := d.eventEndpoints[eventType]
	if len(endpoints) == 0 {
		endpoints = d.eventEndpoints[DefaultEventType]
	}

	if len(endpoints) == 0 {
		// allow no endpoints, but log an error since this means that we're dropping
		// traffic explicitly because of configuration
		return fmt.Errorf("No endpoints configured for event: %s", eventType)
	}

	for _, url := range endpoints {
		request, err := d.newRequest(url, contentType, bytes.NewReader(contents))
		if err != nil {
			return err
		}

		if err := d.send(request); err != nil {
			return err
		}
	}

	return nil
}

func (d *dispatcher) dispatchTo(unfiltered string, contentType string, contents []byte) error {
	url, err := d.urlFilter.Filter(unfiltered)
	if err != nil {
		return err
	}

	request, err := d.newRequest(url, contentType, bytes.NewReader(contents))
	if err != nil {
		return err
	}

	return d.send(request)
}

func (d *dispatcher) OnDeviceEvent(event *device.Event) {
	if event.Type != device.MessageReceived {
		return
	}

	if routable, ok := event.Message.(wrp.Routable); ok {
		var (
			destination = routable.To()
			contentType = event.Format.ContentType()
		)

		if strings.HasPrefix(destination, EventPrefix) {
			eventType := destination[len(EventPrefix):]
			if err := d.dispatchEvent(eventType, contentType, event.Contents); err != nil {
				d.logger.Error("Error dispatching event [%s]: %s", destination, err)
			}
		} else if strings.HasPrefix(destination, DNSPrefix) {
			unfilteredURL := destination[len(DNSPrefix):]
			if err := d.dispatchTo(unfilteredURL, contentType, event.Contents); err != nil {
				d.logger.Error("Error dispatching to [%s]: %s", destination, err)
			}
		} else {
			d.logger.Error("Unable to route to [%s]", destination)
		}
	} else {
		d.logger.Error("Not a routable message type [%d].", event.Message.MessageType())
	}
}
