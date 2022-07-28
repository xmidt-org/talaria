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
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/go-kit/kit/metrics"
	"github.com/xmidt-org/webpa-common/v2/device"
	"github.com/xmidt-org/webpa-common/v2/event"
	"github.com/xmidt-org/webpa-common/v2/logging"
	"github.com/xmidt-org/wrp-go/v3"
)

// eventDispatcher is an internal Dispatcher implementation that sends envelopes
// via the returned channel. The channel may be used to spawn one or more workers
// to process the envelopes
type eventDispatcher struct {
	errorLog         log.Logger
	urlFilter        URLFilter
	method           string
	timeout          time.Duration
	authorizationKey string
	source           string
	eventMap         event.MultiMap
	queueSize        metrics.Gauge
	droppedMessages  metrics.Counter
	outbounds        chan<- outboundEnvelope
}

// NewEventDispatcher is an eventDispatcher factory which sends envelopes via
// the returned channel. The channel may be used to spawn one or more workers
// to process the envelopes.
func NewEventDispatcher(om OutboundMeasures, o *Outbounder, urlFilter URLFilter) (Dispatcher, <-chan outboundEnvelope, error) {
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

	return &eventDispatcher{
		errorLog:         logging.Error(logger),
		urlFilter:        urlFilter,
		method:           o.method(),
		timeout:          o.requestTimeout(),
		authorizationKey: o.authKey(),
		eventMap:         eventMap,
		queueSize:        om.QueueSize,
		source:           o.source(),
		droppedMessages:  om.DroppedMessages,
		outbounds:        outbounds,
	}, outbounds, nil
}

// OnDeviceEvent is the device.Listener function that processes outbound events.
func (d *eventDispatcher) OnDeviceEvent(event *device.Event) {
	// TODO improve how we test dispatchEvent & dispatchTo
	if event == nil {
		d.errorLog.Log(logging.MessageKey(), "Error nil event")
		return
	}

	switch event.Type {
	case device.Connect:
		eventType, message := newOnlineMessage(d.source, event.Device)
		if err := d.encodeAndDispatchEvent(eventType, wrp.Msgpack, message); err != nil {
			d.errorLog.Log(logging.MessageKey(), "Error dispatching online event", "eventType", eventType, "destination", message.Destination, logging.ErrorKey(), err)
		}

	case device.Disconnect:
		eventType, message := newOfflineMessage(d.source, event.Device)
		if err := d.encodeAndDispatchEvent(eventType, wrp.Msgpack, message); err != nil {
			d.errorLog.Log(logging.MessageKey(), "Error dispatching offline event", "eventType", eventType, "destination", message.Destination, logging.ErrorKey(), err)
		}

	case device.MessageReceived:
		if routable, ok := event.Message.(wrp.Routable); ok {
			destination := routable.To()
			contentType := event.Format.ContentType()
			if strings.HasPrefix(destination, EventPrefix) {
				eventType := destination[len(EventPrefix):]
				if err := d.dispatchEvent(eventType, contentType, event.Contents); err != nil {
					d.errorLog.Log(logging.MessageKey(), "Error dispatching event", "eventType", eventType, "destination", destination, logging.ErrorKey(), err)
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
}

// send wraps the given request in an outboundEnvelope together with a cancellable context,
// then asynchronously sends that request to the outbounds channel.  This method will
// block on the outbound channel only as long as the context is not cancelled, i.e. does not time out.
// If the context is cancelled before the envelope can be queued, this method drops the message
// and returns an error.
func (d *eventDispatcher) send(parent context.Context, request *http.Request) error {
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

// newRequest creates a basic HTTP request appropriate for this eventDispatcher.
func (d *eventDispatcher) newRequest(url, contentType string, body io.Reader) (*http.Request, error) {
	request, err := http.NewRequest(d.method, url, body)
	if err == nil {
		request.Header.Set("Content-Type", contentType)

		// TODO: Need to work out how to handle authorization better, without basic auth
		if len(d.authorizationKey) > 0 {
			request.Header.Set("Authorization", "Basic "+d.authorizationKey)
		}
	}

	return request, err
}

func (d *eventDispatcher) dispatchEvent(eventType, contentType string, contents []byte) error {
	endpoints, ok := d.eventMap.Get(eventType, DefaultEventType)
	if !ok {
		// allow no endpoints, but log an error since this means that we're dropping
		// traffic explicitly because of configuration
		return fmt.Errorf("no endpoints configured for event: %s", eventType)
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

func (d *eventDispatcher) encodeAndDispatchEvent(eventType string, format wrp.Format, message *wrp.Message) error {
	var (
		contents []byte
		encoder  = wrp.NewEncoderBytes(&contents, format)
	)

	if err := encoder.Encode(message); err != nil {
		return err
	}

	if err := d.dispatchEvent(eventType, format.ContentType(), contents); err != nil {
		return err
	}

	return nil
}

func (d *eventDispatcher) dispatchTo(unfiltered string, contentType string, contents []byte) error {
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
