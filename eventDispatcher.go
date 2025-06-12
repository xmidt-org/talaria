// SPDX-FileCopyrightText: 2017 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime/debug"
	"time"

	"go.uber.org/zap"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/xmidt-org/webpa-common/v2/device"
	"github.com/xmidt-org/webpa-common/v2/event"

	// nolint:staticcheck
	"github.com/xmidt-org/wrp-go/v3"
)

var (
	ErrorEncodingFailed               = errors.New("encoding failed")
	ErrorNoEndpointConfiguredForEvent = errors.New("no endpoints configured for event")
	ErrorMalformedHttpRequest         = errors.New("malformed http request")
	ErrorUnroutableDestination        = errors.New("unroutable destination")
	ErrorUnsupportedEvent             = errors.New("unsupported event")
)

// eventDispatcher is an internal Dispatcher implementation that sends envelopes
// via the returned channel. The channel may be used to spawn one or more workers
// to process the envelopes
type eventDispatcher struct {
	logger           *zap.Logger
	urlFilter        URLFilter
	method           string
	timeout          time.Duration
	authorizationKey string
	source           string
	eventMap         event.MultiMap
	queueSize        prometheus.Gauge
	droppedMessages  CounterVec
	outboundEvents   CounterVec
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
		logger:           logger,
		urlFilter:        urlFilter,
		method:           o.method(),
		timeout:          o.requestTimeout(),
		authorizationKey: o.authKey(),
		eventMap:         eventMap,
		queueSize:        om.QueueSize,
		source:           o.source(),
		droppedMessages:  om.DroppedMessages,
		outboundEvents:   om.OutboundEvents,
		outbounds:        outbounds,
	}, outbounds, nil
}

// OnDeviceEvent is the device.Listener function that processes outbound events.
func (d *eventDispatcher) OnDeviceEvent(event *device.Event) {
	// TODO improve how we test dispatchEvent & dispatchTo
	var (
		err    error
		scheme = unknown
	)

	defer func() {
		if r := recover(); nil != r {
			d.logger.Debug("stacktrace from panic", zap.String("stacktrace", string(debug.Stack())), zap.Any("panic", r))
			switch event.Type {
			case device.Connect, device.Disconnect, device.MessageReceived:
				d.logger.Error("Dropped message, event not sent", zap.String(schemeLabel, scheme), zap.String(codeLabel, messageDroppedCode), zap.String(reasonLabel, panicReason), zap.Any("panic", r))
				d.droppedMessages.With(prometheus.Labels{schemeLabel: scheme, codeLabel: messageDroppedCode, reasonLabel: panicReason}).Add(1.0)
				d.outboundEvents.With(prometheus.Labels{schemeLabel: scheme, reasonLabel: panicReason, outcomeLabel: failureOutcome}).Add(1.0)
			}
		}
	}()

	if event == nil {
		d.logger.Error("Error nil event")
		return
	}

	switch event.Type {
	case device.Connect:
		scheme = wrp.SchemeEvent
		eventType, message := newOnlineMessage(d.source, event.Device)
		_, err = d.encodeAndDispatchEvent(eventType, wrp.Msgpack, message)
		if err != nil {
			d.logger.Error("Error dispatching online event", zap.Any("eventType", eventType), zap.Any("destination", message.Destination), zap.Error(err))
		}

	case device.Disconnect:
		scheme = wrp.SchemeEvent
		eventType, message := newOfflineMessage(d.source, event.Device)
		_, err = d.encodeAndDispatchEvent(eventType, wrp.Msgpack, message)
		if err != nil {
			d.logger.Error("Error dispatching offline event", zap.Any("eventType", eventType), zap.Any("destination", message.Destination), zap.Error(err))
		}
	case device.MessageReceived:
		scheme, err = d.routeMessageReceivedEvent(event)
		if err != nil {
			scheme = unknown
		}
	default:
		err = ErrorUnsupportedEvent
	}

	var outboundEventsLabels prometheus.Labels
	if err != nil {
		reason := getDroppedMessageReason(err)
		outboundEventsLabels = prometheus.Labels{schemeLabel: scheme, reasonLabel: reason, outcomeLabel: failureOutcome}
		if errors.Is(err, ErrorUnsupportedEvent) {
			d.logger.Debug("Dropped message, event not sent", zap.String(schemeLabel, scheme), zap.String(codeLabel, messageDroppedCode), zap.String(reasonLabel, reason), zap.Error(err))
		} else {
			d.logger.Error("Dropped message, event not sent", zap.String(schemeLabel, scheme), zap.String(codeLabel, messageDroppedCode), zap.String(reasonLabel, reason), zap.Error(err))
		}

		d.droppedMessages.With(prometheus.Labels{schemeLabel: scheme, codeLabel: messageDroppedCode, reasonLabel: reason}).Add(1.0)
	} else {
		outboundEventsLabels = prometheus.Labels{schemeLabel: scheme, reasonLabel: noErrReason, outcomeLabel: successOutcome}
	}

	d.outboundEvents.With(outboundEventsLabels).Add(1.0)
}

func (d *eventDispatcher) routeMessageReceivedEvent(event *device.Event) (scheme string, err error) {
	routable, ok := event.Message.(wrp.Routable)
	if !ok {
		return "", errors.New("wrp event message is not routable")
	}

	destination := routable.To()
	contentType := event.Format.ContentType()
	var l wrp.Locator
	if l, err = wrp.ParseLocator(destination); err != nil {
		return "", err
	}

	scheme = l.Scheme
	eventType := l.Authority
	switch scheme {
	case wrp.SchemeEvent:
		_, err = d.dispatchEvent(eventType, contentType, event.Contents)
	case wrp.SchemeDNS:
		url := l.Authority + l.Ignored
		// `l.Authority + l.Ignored` is used because incoming dns events are expected to have the format `dns:some_url` or dns:some_scheme://some_url.
		_, err = d.dispatchTo(url, contentType, event.Contents)
	default:
		scheme = unknown
		err = ErrorUnroutableDestination
	}

	if err != nil {
		d.logger.Error("Error dispatching event", zap.String(schemeLabel, scheme), zap.Any("destination", destination), zap.Error(err))
	}

	return scheme, err
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
	select {
	case d.outbounds <- outboundEnvelope{request.WithContext(ctx), cancel}:
		return nil

	default:
		d.queueSize.Add(-1.0) // the message never made it to the queue

		return ErrOutboundQueueFull
	}
}

// newRequest creates a basic HTTP request appropriate for this eventDispatcher.
func (d *eventDispatcher) newRequest(url, contentType string, body io.Reader) (*http.Request, error) {
	request, err := http.NewRequest(d.method, url, body)
	if err != nil {
		return request, fmt.Errorf("%w: %s", ErrorMalformedHttpRequest, err)
	}

	request.Header.Set("Content-Type", contentType)
	// TODO: Need to work out how to handle authorization better, without basic auth
	if len(d.authorizationKey) > 0 {
		request.Header.Set("Authorization", "Basic "+d.authorizationKey)
	}

	return request, nil
}

func (d *eventDispatcher) dispatchEvent(eventType, contentType string, contents []byte) (string, error) {
	url := unknown
	endpoints, ok := d.eventMap.Get(eventType, DefaultEventType)
	if !ok {
		// allow no endpoints, but log an error since this means that we're dropping
		// traffic explicitly because of configuration
		return url, fmt.Errorf("%w: %s", ErrorNoEndpointConfiguredForEvent, eventType)
	}

	ctx := context.WithValue(
		context.Background(), schemeContextKey{},
		wrp.SchemeEvent,
	)

	for _, url = range endpoints {
		request, err := d.newRequest(url, contentType, bytes.NewReader(contents))
		if err != nil {
			return url, err
		}

		url = request.URL.String()
		if err := d.send(ctx, request); err != nil {
			return url, err
		}
	}

	return url, nil
}

func (d *eventDispatcher) encodeAndDispatchEvent(eventType string, format wrp.Format, message *wrp.Message) (string, error) {
	var (
		err error
		url = unknown
	)
	var (
		contents []byte
		encoder  = wrp.NewEncoderBytes(&contents, format)
	)

	if err = encoder.Encode(message); err != nil {
		return url, fmt.Errorf("%w; %s", ErrorEncodingFailed, err)
	}

	if url, err = d.dispatchEvent(eventType, format.ContentType(), contents); err != nil {
		return url, err
	}

	return url, nil
}

func (d *eventDispatcher) dispatchTo(unfiltered string, contentType string, contents []byte) (string, error) {
	var (
		err error
		url = unfiltered
	)

	url, err = d.urlFilter.Filter(unfiltered)
	if err != nil {
		return url, err
	}

	request, err := d.newRequest(url, contentType, bytes.NewReader(contents))
	if err != nil {
		return url, err
	}

	return request.URL.String(), d.send(
		context.WithValue(context.Background(), schemeContextKey{},
			wrp.SchemeDNS),
		request,
	)
}

func getDroppedMessageReason(err error) string {
	if err == nil {
		return noErrReason
	} else if errors.Is(err, ErrorEncodingFailed) {
		return encodeErrReason
	} else if errors.Is(err, ErrorNoEndpointConfiguredForEvent) {
		return noEndpointConfiguredForEventReason
	} else if errors.Is(err, ErrorURLSchemeNotAllowed) {
		return urlSchemeNotAllowedReason
	} else if errors.Is(err, ErrorMalformedHttpRequest) {
		return malformedHTTPRequestReason
	} else if errors.Is(err, ErrOutboundQueueFull) {
		return fullQueueReason
	} else if errors.Is(err, ErrorUnroutableDestination) {
		return unroutableDestinationReason
	} else if errors.Is(err, ErrorUnsupportedEvent) {
		return notSupportedEventReason
	}

	// check for http `Do` related errors
	return getDoErrReason(err)
}
