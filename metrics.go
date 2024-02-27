// SPDX-FileCopyrightText: 2017 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-kit/kit/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/xmidt-org/webpa-common/v2/xhttp"

	// nolint:staticcheck
	"github.com/xmidt-org/webpa-common/v2/xmetrics"
)

// Metric names
const (
	OutboundInFlightGauge              = "outbound_inflight"
	OutboundRequestDuration            = "outbound_request_duration_seconds"
	OutboundRequestCounter             = "outbound_requests"
	OutboundRequestSizeBytes           = "outbound_request_size"
	TotalOutboundEvents                = "total_outbound_events"
	OutboundQueueSize                  = "outbound_queue_size"
	OutboundDroppedMessageCounter      = "outbound_dropped_messages"
	OutboundRetries                    = "outbound_retries"
	OutboundAckSuccessCounter          = "outbound_ack_success"
	OutboundAckFailureCounter          = "outbound_ack_failure"
	OutboundAckSuccessLatencyHistogram = "outbound_ack_success_latency_seconds"
	OutboundAckFailureLatencyHistogram = "outbound_ack_failure_latency_seconds"

	GateStatus   = "gate_status"
	DrainStatus  = "drain_status"
	DrainCounter = "drain_count"

	InboundWRPMessageCounter = "inbound_wrp_messages"
)

// Metric label names
const (
	outcomeLabel   = "outcome"
	reasonLabel    = "reason"
	qosLevelLabel  = "qos_level"
	partnerIDLabel = "partner_id"
	messageType    = "message_type"
	urlLabel       = "url"
	codeLabel      = "code"
	eventLabel     = "event"
)

// label values
const (
	accepted = "accepted"
	rejected = "rejected"
	unknown  = "unknown"

	deviceNotFound = "device_not_found"
	invalidWRPDest = "invalid_wrp_dest"

	missingDeviceCredential = "missing_device_cred"
	// nolint:gosec
	missingWRPCredential = "missing_wrp_cred"
	incompleteCheck      = "incomplete_check"
	denied               = "denied"
	authorized           = "authorized"

	// dropped message & outbound event reasons
	unroutableDestinationReason           = "unroutable_destination"
	encodeErrReason                       = "encoding_err"
	fullQueueReason                       = "full outbound queue"
	genericDoReason                       = "do_error"
	deadlineExceededReason                = "context_deadline_exceeded"
	contextCanceledReason                 = "context_canceled"
	addressErrReason                      = "address_error"
	parseAddrErrReason                    = "parse_address_error"
	invalidAddrReason                     = "invalid_address"
	dnsErrReason                          = "dns_error"
	hostNotFoundReason                    = "host_not_found"
	connClosedReason                      = "connection_closed"
	opErrReason                           = "op_error"
	networkErrReason                      = "unknown_network_err"
	notSupportedEventReason               = "unsupported event"
	noEndpointConfiguredForEventReason    = "no_endpoint_configured_for_event"
	urlSchemeNotAllowedReason             = "url_scheme_not_allowed"
	malformedHTTPRequestReason            = "malformed_http_request"
	panicReason                           = "panic"
	connectionUnexpectedlyClosedEOFReason = "connection_unexpectedly_closed_eof"
	noErrReason                           = "no_err"
	expectedCodeReason                    = "expected_code"

	// dropped message codes
	non202Code         = "non202"
	expected202Code    = "202"
	messageDroppedCode = "message_dropped"

	// outbound event delivery outcomes
	successOutcome = "success"
	failureOutcome = "failure"
)

func Metrics() []xmetrics.Metric {
	return []xmetrics.Metric{
		{
			Name: OutboundInFlightGauge,
			Type: xmetrics.GaugeType,
			Help: "The number of active, in-flight requests from devices",
		},
		{
			Name:       OutboundRequestDuration,
			Type:       "histogram",
			Help:       "The durations of outbound requests from devices",
			LabelNames: []string{eventLabel, codeLabel, reasonLabel, urlLabel},
			Buckets:    []float64{.25, .5, 1, 2.5, 5, 10},
		},
		{
			Name:       OutboundRequestCounter,
			Type:       xmetrics.CounterType,
			Help:       "The count of outbound requests",
			LabelNames: []string{eventLabel, codeLabel, reasonLabel, urlLabel},
		},
		{
			Name:       OutboundRequestSizeBytes,
			Type:       xmetrics.HistogramType,
			Help:       "A histogram of request sizes for outbound requests",
			LabelNames: []string{eventLabel, codeLabel, reasonLabel, urlLabel},
			Buckets:    []float64{200, 500, 900, 1500, 3000, 6000, 12000, 24000, 48000, 96000, 192000, 384000, 768000, 1536000},
		},
		{
			Name:       TotalOutboundEvents,
			Type:       xmetrics.CounterType,
			Help:       "Total count of outbound events",
			LabelNames: []string{eventLabel, reasonLabel, urlLabel, outcomeLabel},
		},
		{
			Name: OutboundQueueSize,
			Type: xmetrics.GaugeType,
			Help: "The current number of requests waiting to be sent outbound",
		},
		{
			Name:       OutboundDroppedMessageCounter,
			Type:       xmetrics.CounterType,
			Help:       "The total count of messages dropped",
			LabelNames: []string{eventLabel, codeLabel, reasonLabel, urlLabel},
		},
		{
			Name: OutboundRetries,
			Type: xmetrics.CounterType,
			Help: "The total count of outbound HTTP retries",
		},
		{
			Name:       OutboundAckSuccessCounter,
			Type:       xmetrics.CounterType,
			Help:       "Number of outbound WRP acks",
			LabelNames: []string{qosLevelLabel, partnerIDLabel, messageType},
		},
		{
			Name:       OutboundAckFailureCounter,
			Type:       xmetrics.CounterType,
			Help:       "Number of outbound WRP ack failures",
			LabelNames: []string{qosLevelLabel, partnerIDLabel, messageType},
		},
		{
			Name:       OutboundAckSuccessLatencyHistogram,
			Type:       xmetrics.HistogramType,
			Help:       "A histogram of latencies for successful outbound WRP acks",
			LabelNames: []string{qosLevelLabel, partnerIDLabel, messageType},
			Buckets:    []float64{0.0625, 0.125, .25, .5, 1, 5, 10, 20, 40, 80, 160},
		},
		{
			Name:       OutboundAckFailureLatencyHistogram,
			Type:       xmetrics.HistogramType,
			Help:       "A histogram of latencies for failed outbound WRP acks",
			LabelNames: []string{qosLevelLabel, partnerIDLabel, messageType},
			Buckets:    []float64{0.0625, 0.125, .25, .5, 1, 5, 10, 20, 40, 80, 160},
		},
		{
			Name: GateStatus,
			Type: xmetrics.GaugeType,
			Help: "Indicates whether the device gate is open (1.0) or closed (0.0)",
		},
		{
			Name: DrainStatus,
			Type: xmetrics.GaugeType,
			Help: "Indicates whether a device drain operation is currently running",
		},
		{
			Name: DrainCounter,
			Type: xmetrics.CounterType,
			Help: "The total count of devices disconnected due to a drain since the server started",
		},
		{
			Name:       InboundWRPMessageCounter,
			Type:       xmetrics.CounterType,
			Help:       "Number of inbound WRP Messages successfully decoded and ready to route to device",
			LabelNames: []string{outcomeLabel, reasonLabel},
		},
	}
}

type HistogramVec interface {
	prometheus.Collector
	With(prometheus.Labels) prometheus.Observer
	CurryWith(prometheus.Labels) (prometheus.ObserverVec, error)
	GetMetricWith(prometheus.Labels) (prometheus.Observer, error)
	GetMetricWithLabelValues(...string) (prometheus.Observer, error)
	MustCurryWith(labels prometheus.Labels) (o prometheus.ObserverVec)
	WithLabelValues(lvs ...string) (o prometheus.Observer)
}

type CounterVec interface {
	prometheus.Collector
	With(prometheus.Labels) prometheus.Counter
}

type OutboundMeasures struct {
	InFlight          prometheus.Gauge
	RequestDuration   HistogramVec
	RequestCounter    CounterVec
	RequestSize       HistogramVec
	OutboundEvents    CounterVec
	QueueSize         metrics.Gauge
	Retries           metrics.Counter
	DroppedMessages   CounterVec
	AckSuccess        CounterVec
	AckFailure        CounterVec
	AckSuccessLatency HistogramVec
	AckFailureLatency HistogramVec
}

func NewOutboundMeasures(r xmetrics.Registry) OutboundMeasures {
	return OutboundMeasures{
		InFlight:        r.NewGaugeVec(OutboundInFlightGauge).WithLabelValues(),
		RequestDuration: r.NewHistogramVec(OutboundRequestDuration),
		RequestCounter:  r.NewCounterVec(OutboundRequestCounter),
		RequestSize:     r.NewHistogramVec(OutboundRequestSizeBytes),
		OutboundEvents:  r.NewCounterVec(TotalOutboundEvents),
		QueueSize:       r.NewGauge(OutboundQueueSize),
		Retries:         r.NewCounter(OutboundRetries),
		DroppedMessages: r.NewCounterVec(OutboundDroppedMessageCounter),
		AckSuccess:      r.NewCounterVec(OutboundAckSuccessCounter),
		AckFailure:      r.NewCounterVec(OutboundAckFailureCounter),
		// 0 is for the unused `buckets` argument in xmetrics.Registry.NewHistogram
		AckSuccessLatency: r.NewHistogramVec(OutboundAckSuccessLatencyHistogram),
		// 0 is for the unused `buckets` argument in xmetrics.Registry.NewHistogram
		AckFailureLatency: r.NewHistogramVec(OutboundAckFailureLatencyHistogram),
	}
}
func InstrumentOutboundSize(obs HistogramVec, next http.RoundTripper) promhttp.RoundTripperFunc {
	return promhttp.RoundTripperFunc(func(request *http.Request) (*http.Response, error) {
		eventType, ok := request.Context().Value(eventTypeContextKey{}).(string)
		if !ok {
			eventType = unknown
		}

		response, err := next.RoundTrip(request)
		size := computeApproximateRequestSize(request)

		var labels prometheus.Labels
		if err != nil {
			code := messageDroppedCode
			if response != nil {
				code = strconv.Itoa(response.StatusCode)
			}

			labels = prometheus.Labels{eventLabel: eventType, codeLabel: code, reasonLabel: getDoErrReason(err), urlLabel: request.URL.String()}
		} else {
			labels = prometheus.Labels{eventLabel: eventType, codeLabel: strconv.Itoa(response.StatusCode), reasonLabel: expectedCodeReason, urlLabel: request.URL.String()}
			if response.StatusCode != http.StatusAccepted {
				labels[reasonLabel] = non202Code
			}
		}

		obs.With(labels).Observe(float64(size))

		return response, err
	})
}
func InstrumentOutboundDuration(obs HistogramVec, next http.RoundTripper) promhttp.RoundTripperFunc {
	return promhttp.RoundTripperFunc(func(request *http.Request) (*http.Response, error) {
		eventType, ok := request.Context().Value(eventTypeContextKey{}).(string)
		if !ok {
			eventType = unknown
		}

		start := time.Now()
		response, err := next.RoundTrip(request)
		delta := time.Since(start).Seconds()

		var labels prometheus.Labels
		if err != nil {
			code := messageDroppedCode
			if response != nil {
				code = strconv.Itoa(response.StatusCode)
			}

			labels = prometheus.Labels{eventLabel: eventType, codeLabel: code, reasonLabel: getDoErrReason(err), urlLabel: request.URL.String()}
		} else {
			labels = prometheus.Labels{eventLabel: eventType, codeLabel: strconv.Itoa(response.StatusCode), reasonLabel: expectedCodeReason, urlLabel: request.URL.String()}
			if response.StatusCode != http.StatusAccepted {
				labels[reasonLabel] = non202Code
			}
		}

		obs.With(labels).Observe(delta)

		return response, err
	})
}

func InstrumentOutboundCounter(counter CounterVec, next http.RoundTripper) promhttp.RoundTripperFunc {
	return promhttp.RoundTripperFunc(func(request *http.Request) (*http.Response, error) {
		eventType, ok := request.Context().Value(eventTypeContextKey{}).(string)
		if !ok {
			eventType = unknown
		}

		response, err := next.RoundTrip(request)

		var labels prometheus.Labels
		if err != nil {
			code := messageDroppedCode
			if response != nil {
				code = strconv.Itoa(response.StatusCode)
			}

			labels = prometheus.Labels{eventLabel: eventType, codeLabel: code, reasonLabel: getDoErrReason(err), urlLabel: request.URL.String()}
		} else {
			labels = prometheus.Labels{eventLabel: eventType, codeLabel: strconv.Itoa(response.StatusCode), reasonLabel: noErrReason, urlLabel: request.URL.String()}
			if response.StatusCode != http.StatusAccepted {
				labels[reasonLabel] = non202Code
			}
		}

		counter.With(labels).Inc()

		return response, err
	})
}

// NewOutboundRoundTripper produces an http.RoundTripper from the configured Outbounder
// that is also decorated with appropriate metrics.
func NewOutboundRoundTripper(om OutboundMeasures, o *Outbounder) http.RoundTripper {
	// TODO add tests for NewOutboundRoundTripper
	// nolint:bodyclose
	return promhttp.RoundTripperFunc(xhttp.RetryTransactor(
		// use the default should retry predicate ...
		xhttp.RetryOptions{
			Logger:  o.logger(),
			Retries: o.retries(),
			Counter: om.Retries,
		},
		InstrumentOutboundCounter(
			om.RequestCounter,
			InstrumentOutboundSize(
				om.RequestSize,
				InstrumentOutboundDuration(
					om.RequestDuration,
					promhttp.InstrumentRoundTripperInFlight(om.InFlight, o.transport()),
				),
			),
		),
	))
}

func computeApproximateRequestSize(r *http.Request) int {
	s := 0
	if r.URL != nil {
		s += len(r.URL.String())
	}

	s += len(r.Method)
	s += len(r.Proto)
	for name, values := range r.Header {
		s += len(name)
		for _, value := range values {
			s += len(value)
		}
	}
	s += len(r.Host)

	// N.B. r.Form and r.MultipartForm are assumed to be included in r.URL.

	if r.ContentLength != -1 {
		s += int(r.ContentLength)
	}
	return s
}
