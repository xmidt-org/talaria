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
	unroutableDestinationReason        = "unroutable_destination"
	encodeErrReason                    = "encoding_err"
	fullQueueReason                    = "full outbound queue"
	genericDoReason                    = "do_error"
	deadlineExceededReason             = "context_deadline_exceeded"
	contextCanceledReason              = "context_canceled"
	addressErrReason                   = "address_error"
	parseAddrErrReason                 = "parse_address_error"
	invalidAddrReason                  = "invalid_address"
	dnsErrReason                       = "dns_error"
	hostNotFoundReason                 = "host_not_found"
	connClosedReason                   = "connection_closed"
	opErrReason                        = "op_error"
	networkErrReason                   = "unknown_network_err"
	notSupportedEventReason            = "unsupported event"
	noEndpointConfiguredForEventReason = "no_endpoint_configured_for_event"
	urlSchemeNotAllowedReason          = "url_scheme_not_allowed"
	malformedHTTPRequestReason         = "malformed_http_request"
	panicReason                        = "panic"
	noErrReason                        = "no_err"
	expectedCodeReason                 = "expected_code"

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
			Name:    OutboundRequestDuration,
			Type:    "histogram",
			Help:    "The durations of outbound requests from devices",
			Buckets: []float64{.25, .5, 1, 2.5, 5, 10},
		},
		{
			Name:       OutboundRequestCounter,
			Type:       xmetrics.CounterType,
			Help:       "The count of outbound requests",
			LabelNames: []string{eventLabel, codeLabel, reasonLabel, urlLabel},
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

type OutboundMeasures struct {
	InFlight          prometheus.Gauge
	RequestDuration   prometheus.Observer
	RequestCounter    *prometheus.CounterVec
	OutboundEvents    *prometheus.CounterVec
	QueueSize         metrics.Gauge
	Retries           metrics.Counter
	DroppedMessages   metrics.Counter
	AckSuccess        metrics.Counter
	AckFailure        metrics.Counter
	AckSuccessLatency metrics.Histogram
	AckFailureLatency metrics.Histogram
}

func NewOutboundMeasures(r xmetrics.Registry) OutboundMeasures {
	return OutboundMeasures{
		InFlight:        r.NewGaugeVec(OutboundInFlightGauge).WithLabelValues(),
		RequestDuration: r.NewHistogramVec(OutboundRequestDuration).WithLabelValues(),
		RequestCounter:  r.NewCounterVec(OutboundRequestCounter),
		OutboundEvents:  r.NewCounterVec(TotalOutboundEvents),
		QueueSize:       r.NewGauge(OutboundQueueSize),
		Retries:         r.NewCounter(OutboundRetries),
		DroppedMessages: r.NewCounter(OutboundDroppedMessageCounter),
		AckSuccess:      r.NewCounter(OutboundAckSuccessCounter),
		AckFailure:      r.NewCounter(OutboundAckFailureCounter),
		// 0 is for the unused `buckets` argument in xmetrics.Registry.NewHistogram
		AckSuccessLatency: r.NewHistogram(OutboundAckSuccessLatencyHistogram, 0),
		// 0 is for the unused `buckets` argument in xmetrics.Registry.NewHistogram
		AckFailureLatency: r.NewHistogram(OutboundAckFailureLatencyHistogram, 0),
	}
}

func InstrumentOutboundDuration(obs prometheus.Observer, next http.RoundTripper) promhttp.RoundTripperFunc {
	return promhttp.RoundTripperFunc(func(request *http.Request) (*http.Response, error) {
		start := time.Now()
		response, err := next.RoundTrip(request)
		if err == nil {
			obs.Observe(time.Since(start).Seconds())
		}

		return response, err
	})
}

func InstrumentOutboundCounter(counter *prometheus.CounterVec, next http.RoundTripper) promhttp.RoundTripperFunc {
	return promhttp.RoundTripperFunc(func(request *http.Request) (*http.Response, error) {
		response, err := next.RoundTrip(request)

		eventType, ok := request.Context().Value(eventTypeContextKey{}).(string)
		if !ok {
			eventType = unknown
		}
		if err == nil {
			labels := prometheus.Labels{eventLabel: eventType, codeLabel: strconv.Itoa(response.StatusCode), reasonLabel: expectedCodeReason, urlLabel: response.Request.URL.String()}
			if response.StatusCode != http.StatusAccepted {
				labels[reasonLabel] = non202Code
			}

			counter.With(labels).Inc()
		} else {
			labels := prometheus.Labels{eventLabel: eventType, codeLabel: strconv.Itoa(response.StatusCode), reasonLabel: getDoErrReason(err), urlLabel: response.Request.URL.String()}
			counter.With(labels).Inc()
		}

		return response, err
	})
}

// NewOutboundRoundTripper produces an http.RoundTripper from the configured Outbounder
// that is also decorated with appropriate metrics.
func NewOutboundRoundTripper(om OutboundMeasures, o *Outbounder) http.RoundTripper {
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
			InstrumentOutboundDuration(
				om.RequestDuration,
				promhttp.InstrumentRoundTripperInFlight(om.InFlight, o.transport()),
			),
		),
	))
}
