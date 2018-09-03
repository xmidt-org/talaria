/**
 * Copyright 2017 Comcast Cable Communications Management, LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/Comcast/webpa-common/wrp"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Comcast/webpa-common/device"
	"github.com/Comcast/webpa-common/event"
	"github.com/Comcast/webpa-common/logging"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/go-kit/kit/metrics"
)

var ErrOutboundQueueFull = errors.New("Outbound message queue full")

const (
	MessageReceivedDispatcher = "received"
	ConnectDispatcher         = "connect"
	DisconnectDispatcher      = "disconnect"
)

// outboundEnvelope is a tuple of information related to handling an asynchronous HTTP request
type outboundEnvelope struct {
	request *http.Request
	cancel  func()
}

// eventTypeContextKey is the internal key type for storing the event type
type eventTypeContextKey struct{}

// Dispatcher handles the creation and routing of HTTP requests in response to device events.
// A Dispatcher represents the send side for enqueuing HTTP requests.
type Dispatcher interface {
	// DispatcherName returns the dispatcher name
	DispatcherName() string
	// OnDeviceEvent is the device.Listener function that processes outbound events.  Inject
	// this function as a device listener for a manager.
	OnDeviceEvent(*device.Event)
}

// dispatcher is the internal Dispatcher implementation
type dispatcher struct {
	errorLog          log.Logger
	urlFilter         URLFilter
	method            string
	timeout           time.Duration
	authorizationKeys []string
	eventMap          event.MultiMap
	queueSize         metrics.Gauge
	droppedMessages   metrics.Counter
	outbounds         chan<- outboundEnvelope
	dispatcherName    string
}

// connectDispatcher is the internal Dispatcher implementation for connect events.
// 'Inherits' from the dispatcher struct so that we can reuse the same dispatcher methods.
type connectDispatcher struct {
	*dispatcher
}

// disconnectDispatcher is the internal Dispatcher implementation for disconnect events
// 'Inherits' from the dispatcher struct so that we can reuse the same dispatcher methods.
type disconnectDispatcher struct {
	*dispatcher
}

// NewDispatcher constructs a Dispatcher which sends envelopes via the returned channel.
// The channel may be used to spawn one or more workers to process the envelopes.
func NewDispatcher(om OutboundMeasures, o *Outbounder, urlFilter URLFilter) (Dispatcher, <-chan outboundEnvelope, error) {
	if urlFilter == nil {
		var err error
		urlFilter, err = NewURLFilter(o)
		if err != nil {
			return nil, nil, err
		}
	}

	outbounds := make(chan outboundEnvelope, o.outboundQueueSize())
	logger := o.logger()
	eventMap, err := o.eventMap()
	if err != nil {
		return nil, nil, err
	}

	logger.Log(level.Key(), level.InfoValue(), "eventMap", eventMap)

	return &dispatcher{
		errorLog:          logging.Error(logger),
		urlFilter:         urlFilter,
		method:            o.method(),
		timeout:           o.requestTimeout(),
		authorizationKeys: o.authKey(),
		eventMap:          eventMap,
		queueSize:         om.QueueSize,
		droppedMessages:   om.DroppedMessages,
		outbounds:         outbounds,
		dispatcherName:    MessageReceivedDispatcher,
	}, outbounds, nil
}

// NewConnectDispatcher constructs a Dispatcher which sends envelopes of connect type via the returned channel.
func NewConnectDispatcher(om OutboundMeasures, o *Outbounder, urlFilter URLFilter) (Dispatcher, <-chan outboundEnvelope, error) {
	if urlFilter == nil {
		var err error
		urlFilter, err = NewURLFilter(o)
		if err != nil {
			return nil, nil, err
		}
	}

	outbounds := make(chan outboundEnvelope, o.outboundQueueSize())
	logger := o.logger()
	eventMap, err := o.eventMap()

	if err != nil {
		return nil, nil, err
	}

	logger.Log(level.Key(), level.InfoValue(), "eventMap", eventMap)

	return &connectDispatcher{
		dispatcher: &dispatcher{
			errorLog:          logging.Error(logger),
			urlFilter:         urlFilter,
			method:            o.method(),
			timeout:           o.requestTimeout(),
			authorizationKeys: o.authKey(),
			eventMap:          eventMap,
			queueSize:         om.QueueSize,
			droppedMessages:   om.DroppedMessages,
			outbounds:         outbounds,
			dispatcherName:    ConnectDispatcher,
		}}, outbounds, nil
}

// NewDisconnectDispatcher constructs a Dispatcher which sends envelopes of disconnect type via the returned channel.
func NewDisconnectDispatcher(om OutboundMeasures, o *Outbounder, urlFilter URLFilter) (Dispatcher, <-chan outboundEnvelope, error) {
	if urlFilter == nil {
		var err error
		urlFilter, err = NewURLFilter(o)
		if err != nil {
			return nil, nil, err
		}
	}

	outbounds := make(chan outboundEnvelope, o.outboundQueueSize())
	logger := o.logger()
	eventMap, err := o.eventMap()

	if err != nil {
		return nil, nil, err
	}

	logger.Log(level.Key(), level.InfoValue(), "eventMap", eventMap)

	return &disconnectDispatcher{
		dispatcher: &dispatcher{
			errorLog:          logging.Error(logger),
			urlFilter:         urlFilter,
			method:            o.method(),
			timeout:           o.requestTimeout(),
			authorizationKeys: o.authKey(),
			eventMap:          eventMap,
			queueSize:         om.QueueSize,
			droppedMessages:   om.DroppedMessages,
			outbounds:         outbounds,
			dispatcherName:    DisconnectDispatcher,
		}}, outbounds, nil
}

// send wraps the given request in an outboundEnvelope together with a cancellable context,
// then asynchronously sends that request to the outbounds channel.  This method will
// block on the outbound channel only as long as the context is not cancelled, i.e. does not time out.
// If the context is cancelled before the envelope can be queued, this method drops the message
// and returns an error.
func (d *dispatcher) send(parent context.Context, request *http.Request) error {
	// increment the queue size first, so that we always keep a positive queue size
	d.queueSize.Add(1.0)
	ctx, cancel := context.WithTimeout(parent, d.timeout)

	select {
	case d.outbounds <- outboundEnvelope{request.WithContext(ctx), cancel}:
		return nil

	default:
		d.queueSize.Add(-1.0) // the message never made it to the queue
		d.droppedMessages.Add(1.0)
		return ErrOutboundQueueFull
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
	endpoints, ok := d.eventMap.Get(eventType, DefaultEventType)
	if !ok {
		// allow no endpoints, but log an error since this means that we're dropping
		// traffic explicitly because of configuration
		return fmt.Errorf("No endpoints configured for event: %s", eventType)
	}

	ctx := context.WithValue(
		context.Background(), eventTypeContextKey{},
		eventType,
	)

	for _, url := range endpoints {
		request, err := d.newRequest(url, contentType, bytes.NewReader(contents))
		if err != nil {
			return err
		}

		if err := d.send(ctx, request); err != nil {
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

	return d.send(
		context.WithValue(context.Background(), eventTypeContextKey{}, DNSPrefix),
		request,
	)
}

func (d *dispatcher) routeMessage(event *device.Event) {
	if routable, ok := event.Message.(wrp.Routable); ok {
		var (
			destination = routable.To()
			contentType = event.Format.ContentType()
		)

		if strings.HasPrefix(destination, EventPrefix) {
			eventType := destination[len(EventPrefix):]
			if err := d.dispatchEvent(eventType, contentType, event.Contents); err != nil {
				d.errorLog.Log(logging.MessageKey(), "Error dispatching event", "destination", destination, logging.ErrorKey(), err)
			}
		} else if strings.HasPrefix(destination, DNSPrefix) {
			unfilteredURL := destination[len(DNSPrefix):]
			if err := d.dispatchTo(unfilteredURL, contentType, event.Contents); err != nil {
				d.errorLog.Log(logging.MessageKey(), "Error dispatching to endpoint", "destination", destination, logging.ErrorKey(), err)
			}
		} else {
			d.errorLog.Log(logging.MessageKey(), "Unroutable destination", "destination", destination)
		}
	}
}

func (d *dispatcher) DispatcherName() string {
	return d.dispatcherName
}

func (d *connectDispatcher) DispatcherName() string {
	return d.dispatcher.dispatcherName
}

func (d *disconnectDispatcher) DispatcherName() string {
	return d.dispatcher.dispatcherName
}

func (d *dispatcher) OnDeviceEvent(event *device.Event) {
	if event.Type != device.MessageReceived {
		return
	}

	d.routeMessage(event)
}

func (d *connectDispatcher) OnDeviceEvent(event *device.Event) {
	if event.Type != device.Connect {
		return
	}

	d.dispatcher.routeMessage(event)
}

func (d *disconnectDispatcher) OnDeviceEvent(event *device.Event) {
	if event.Type != device.Disconnect {
		return
	}

	d.dispatcher.routeMessage(event)
}
