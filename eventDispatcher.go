// SPDX-FileCopyrightText: 2017 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/go-kit/kit/metrics"
	"github.com/xmidt-org/webpa-common/v2/device"
	"github.com/xmidt-org/webpa-common/v2/event"

	// nolint:staticcheck
	"github.com/xmidt-org/wrp-go/v3"
)

// eventDispatcher is an internal Dispatcher implementation that sends envelopes
// via the returned channel. The channel may be used to spawn one or more workers
// to process the envelopes
type eventDispatcher struct {
	errorLog         *zap.Logger
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

	logger.Info("eventMap created", zap.Any("eventMap", eventMap))

	return &eventDispatcher{
		errorLog:         logger,
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
		d.errorLog.Error("Error nil event")
		return
	}

	switch event.Type {
	case device.Connect:
		eventType, message := newOnlineMessage(d.source, event.Device)
		if err := d.encodeAndDispatchEvent(eventType, wrp.Msgpack, message); err != nil {
			d.errorLog.Error("Error dispatching online event", zap.Any("eventType", eventType), zap.Any("destination", message.Destination), zap.Error(err))
		}

	case device.Disconnect:
		eventType, message := newOfflineMessage(d.source, event.Device)
		if err := d.encodeAndDispatchEvent(eventType, wrp.Msgpack, message); err != nil {
			d.errorLog.Error("Error dispatching offline event", zap.Any("eventType", eventType), zap.Any("destination", message.Destination), zap.Error(err))
		}

	case device.MessageReceived:
		if routable, ok := event.Message.(wrp.Routable); ok {
			destination := routable.To()
			contentType := event.Format.ContentType()
			if strings.HasPrefix(destination, EventPrefix) {
				eventType := destination[len(EventPrefix):]
				if err := d.dispatchEvent(eventType, contentType, event.Contents); err != nil {
					d.errorLog.Error("Error dispatching event", zap.Any("eventType", eventType), zap.Any("destination", destination), zap.Error(err))
				}
			} else if strings.HasPrefix(destination, DNSPrefix) {
				unfilteredURL := destination[len(DNSPrefix):]
				if err := d.dispatchTo(unfilteredURL, contentType, event.Contents); err != nil {
					d.errorLog.Error("Error dispatching to endpoint", zap.Any("destination", destination), zap.Error(err))
				}
			} else {
				d.errorLog.Error("Unroutable destination", zap.Any("destination", destination))
			}
		}
	}
}

// send wraps the given request in an outboundEnvelope together with a cancellable context,
// then asynchronously sends that request to the outbounds channel.  This method will
// block on the outbound channel only as long as the context is not canceled, i.e. does not time out.
// If the context is canceled before the envelope can be queued, this method drops the message
// and returns an error.
func (d *eventDispatcher) send(parent context.Context, request *http.Request) error {
	// increment the queue size first, so that we always keep a positive queue size
	d.queueSize.Add(1.0)
	ctx, cancel := context.WithTimeout(parent, d.timeout)
	eventType, ok := ctx.Value(eventTypeContextKey{}).(string)
	if !ok {
		eventType = unknown
	}

	select {
	case d.outbounds <- outboundEnvelope{request.WithContext(ctx), cancel}:
		return nil

	default:
		d.queueSize.Add(-1.0) // the message never made it to the queue
		d.droppedMessages.With(eventLabel, eventType, codeLabel, "", reasonLabel, fullQueue, urlLabel, request.URL.String()).Add(1.0)

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
