// SPDX-FileCopyrightText: 2017 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	// TODO replace httpaux/retry with github.com/xmidt-org/retry
	// nolint:staticcheck
	"github.com/xmidt-org/httpaux/retry"
	"github.com/xmidt-org/touchstone"
)

// Metric names
const (
	OutboundInFlightGauge              = "outbound_inflight"
	OutboundRequestDuration            = "outbound_request_duration_seconds"
	OutboundRequestCounter             = "outbound_requests"
	OutboundRequestSizeBytes           = "outbound_request_size"
	TotalOutboundEvents                = "total_outbound_events"
	KafkaTotalOutboundEvents           = "kafka_total_outbound_events"
	OutboundQueueSize                  = "outbound_queue_size"
	OutboundDroppedMessageCounter      = "outbound_dropped_messages"
	KafkaOutboundDroppedMessageCounter = "kafka_outbound_dropped_messages"
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
	codeLabel      = "code"
	schemeLabel    = "scheme"
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
	non202CodeReason                      = "non202"
	kafkaSendFailure                      = "kafka_send_failure"

	// dropped message codes
	messageDroppedCode = "message_dropped"

	// outbound event delivery outcomes
	successOutcome = "success"
	failureOutcome = "failure"
)

type HistogramVec interface {
	prometheus.Collector
	With(labels prometheus.Labels) prometheus.Observer
	CurryWith(labels prometheus.Labels) (prometheus.ObserverVec, error)
	GetMetricWith(labels prometheus.Labels) (prometheus.Observer, error)
	GetMetricWithLabelValues(lvs ...string) (prometheus.Observer, error)
	MustCurryWith(labels prometheus.Labels) (o prometheus.ObserverVec)
	WithLabelValues(lvs ...string) (o prometheus.Observer)
}

type CounterVec interface {
	prometheus.Collector
	CurryWith(labels prometheus.Labels) (*prometheus.CounterVec, error)
	GetMetricWith(labels prometheus.Labels) (prometheus.Counter, error)
	GetMetricWithLabelValues(lvs ...string) (prometheus.Counter, error)
	MustCurryWith(labels prometheus.Labels) *prometheus.CounterVec
	With(labels prometheus.Labels) prometheus.Counter
	WithLabelValues(lvs ...string) prometheus.Counter
}

type GaugeVec interface {
	prometheus.Collector
	CurryWith(labels prometheus.Labels) (*prometheus.GaugeVec, error)
	GetMetricWith(labels prometheus.Labels) (prometheus.Gauge, error)
	GetMetricWithLabelValues(lvs ...string) (prometheus.Gauge, error)
	MustCurryWith(labels prometheus.Labels) *prometheus.GaugeVec
	With(labels prometheus.Labels) prometheus.Gauge
	WithLabelValues(lvs ...string) prometheus.Gauge
}

type OutboundMeasures struct {
	InFlight             prometheus.Gauge
	RequestDuration      HistogramVec
	RequestCounter       CounterVec
	RequestSize          HistogramVec
	OutboundEvents       CounterVec
	QueueSize            prometheus.Gauge
	Retries              prometheus.Counter
	DroppedMessages      CounterVec
	KafkaDroppedMessages CounterVec
	KafkaOutboundEvents  CounterVec
	AckSuccess           CounterVec
	AckFailure           CounterVec
	AckSuccessLatency    HistogramVec
	AckFailureLatency    HistogramVec
}

func NewOutboundMeasures(tf *touchstone.Factory) (om OutboundMeasures, errs error) {
	var err error

	om.InFlight, err = tf.NewGauge(prometheus.GaugeOpts{
		Name: OutboundInFlightGauge,
		Help: "The number of active, in-flight requests from devices",
	})
	errs = errors.Join(errs, err)

	om.RequestDuration, err = tf.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: OutboundRequestDuration,
			Help: "The durations of outbound requests from devices",
			// Each bucket is at most 10% wider than the previous one),
			// which will result in each power of two divided into 8 buckets.
			// (e.g. there will be 8 buckets between 1
			// and 2, same as between 2 and 4, and 4 and 8, etc.).
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramZeroThreshold:    0.25,
			NativeHistogramMaxBucketNumber:  10,
			NativeHistogramMinResetDuration: time.Hour * 24 * 7,
			NativeHistogramMaxZeroThreshold: 0.5,
			// Disable exemplars.
			NativeHistogramMaxExemplars: -1,
			NativeHistogramExemplarTTL:  time.Minute * 5,
		},
		[]string{schemeLabel, codeLabel, reasonLabel}...,
	)
	errs = errors.Join(errs, err)

	om.RequestCounter, err = tf.NewCounterVec(
		prometheus.CounterOpts{
			Name: OutboundRequestCounter,
			Help: "The count of outbound requests",
		},
		[]string{schemeLabel, codeLabel, reasonLabel}...,
	)
	errs = errors.Join(errs, err)

	om.RequestSize, err = tf.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: OutboundRequestSizeBytes,
			Help: "A histogram of request sizes for outbound requests",
			// Each bucket is at most 10% wider than the previous one),
			// which will result in each power of two divided into 8 buckets.
			// (e.g. there will be 8 buckets between 1
			// and 2, same as between 2 and 4, and 4 and 8, etc.).
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramZeroThreshold:    200,
			NativeHistogramMaxBucketNumber:  10,
			NativeHistogramMinResetDuration: time.Hour * 24 * 7,
			NativeHistogramMaxZeroThreshold: 1500,
			// Disable exemplars.
			NativeHistogramMaxExemplars: -1,
			NativeHistogramExemplarTTL:  time.Minute * 5,
		},
		[]string{schemeLabel, codeLabel}...,
	)
	errs = errors.Join(errs, err)

	om.OutboundEvents, err = tf.NewCounterVec(
		prometheus.CounterOpts{
			Name: TotalOutboundEvents,
			Help: "Total count of outbound events",
		},
		[]string{schemeLabel, reasonLabel, outcomeLabel}...,
	)
	errs = errors.Join(errs, err)

	om.KafkaOutboundEvents, err = tf.NewCounterVec(
		prometheus.CounterOpts{
			Name: KafkaTotalOutboundEvents,
			Help: "Total count of kafkaoutbound events",
		},
		[]string{schemeLabel, reasonLabel, outcomeLabel}...,
	)
	errs = errors.Join(errs, err)

	om.QueueSize, err = tf.NewGauge(
		prometheus.GaugeOpts{
			Name: OutboundQueueSize,
			Help: "The current number of requests waiting to be sent outbound",
		},
	)
	errs = errors.Join(errs, err)

	om.Retries, err = tf.NewCounter(
		prometheus.CounterOpts{
			Name: OutboundRetries,
			Help: "The total count of outbound HTTP retries",
		},
	)
	errs = errors.Join(errs, err)

	om.DroppedMessages, err = tf.NewCounterVec(
		prometheus.CounterOpts{
			Name: OutboundDroppedMessageCounter,
			Help: "The total count of messages dropped",
		},
		[]string{schemeLabel, codeLabel, reasonLabel}...,
	)
	errs = errors.Join(errs, err)

	om.KafkaDroppedMessages, err = tf.NewCounterVec(
		prometheus.CounterOpts{
			Name: KafkaOutboundDroppedMessageCounter,
			Help: "The total count of kafka messages dropped",
		},
		[]string{schemeLabel, codeLabel, reasonLabel}...,
	)
	errs = errors.Join(errs, err)

	om.AckSuccess, err = tf.NewCounterVec(
		prometheus.CounterOpts{
			Name: OutboundAckSuccessCounter,
			Help: "Number of outbound WRP acks",
		},
		[]string{qosLevelLabel, partnerIDLabel, messageType}...,
	)
	errs = errors.Join(errs, err)

	om.AckFailure, err = tf.NewCounterVec(
		prometheus.CounterOpts{
			Name: OutboundAckFailureCounter,
			Help: "Number of outbound WRP ack failures",
		},
		[]string{qosLevelLabel, partnerIDLabel, messageType}...,
	)
	errs = errors.Join(errs, err)

	om.AckSuccessLatency, err = tf.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: OutboundAckSuccessLatencyHistogram,
			Help: "A histogram of latencies for successful outbound WRP acks",
			// Each bucket is at most 10% wider than the previous one),
			// which will result in each power of two divided into 8 buckets.
			// (e.g. there will be 8 buckets between 1
			// and 2, same as between 2 and 4, and 4 and 8, etc.).
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramZeroThreshold:    0.0625,
			NativeHistogramMaxBucketNumber:  11,
			NativeHistogramMinResetDuration: time.Hour * 24 * 7,
			NativeHistogramMaxZeroThreshold: 0.125,
			// Disable exemplars.
			NativeHistogramMaxExemplars: -1,
			NativeHistogramExemplarTTL:  time.Minute * 5,
		},
		[]string{qosLevelLabel, partnerIDLabel, messageType}...,
	)
	errs = errors.Join(errs, err)

	om.AckFailureLatency, err = tf.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: OutboundAckFailureLatencyHistogram,
			Help: "A histogram of latencies for failed outbound WRP acks",
			// Each bucket is at most 10% wider than the previous one),
			// which will result in each power of two divided into 8 buckets.
			// (e.g. there will be 8 buckets between 1
			// and 2, same as between 2 and 4, and 4 and 8, etc.).
			NativeHistogramBucketFactor:     1.1,
			NativeHistogramZeroThreshold:    0.0625,
			NativeHistogramMaxBucketNumber:  11,
			NativeHistogramMinResetDuration: time.Hour * 24 * 7,
			NativeHistogramMaxZeroThreshold: 0.125,
			// Disable exemplars.
			NativeHistogramMaxExemplars: -1,
			NativeHistogramExemplarTTL:  time.Minute * 5,
		},
		[]string{qosLevelLabel, partnerIDLabel, messageType}...,
	)

	return om, errors.Join(errs, err)
}

func InstrumentOutboundSize(obs HistogramVec, next http.RoundTripper) promhttp.RoundTripperFunc {
	return promhttp.RoundTripperFunc(func(request *http.Request) (*http.Response, error) {
		scheme, ok := request.Context().Value(schemeContextKey{}).(string)
		if !ok {
			scheme = unknown
		}

		response, err := next.RoundTrip(request)
		size := computeApproximateRequestSize(request)

		var labels prometheus.Labels
		if err != nil {
			code := messageDroppedCode
			if response != nil {
				code = strconv.Itoa(response.StatusCode)
			}

			labels = prometheus.Labels{schemeLabel: scheme, codeLabel: code}
		} else {
			labels = prometheus.Labels{schemeLabel: scheme, codeLabel: strconv.Itoa(response.StatusCode)}
		}

		obs.With(labels).Observe(float64(size))

		return response, err
	})
}

func InstrumentOutboundDuration(obs HistogramVec, next http.RoundTripper) promhttp.RoundTripperFunc {
	return promhttp.RoundTripperFunc(func(request *http.Request) (*http.Response, error) {
		scheme, ok := request.Context().Value(schemeContextKey{}).(string)
		if !ok {
			scheme = unknown
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

			labels = prometheus.Labels{schemeLabel: scheme, codeLabel: code, reasonLabel: getDoErrReason(err)}
		} else {
			labels = prometheus.Labels{schemeLabel: scheme, codeLabel: strconv.Itoa(response.StatusCode), reasonLabel: expectedCodeReason}
			if response.StatusCode != http.StatusAccepted {
				labels[reasonLabel] = non202CodeReason
			}
		}

		obs.With(labels).Observe(delta)

		return response, err
	})
}

func InstrumentOutboundCounter(counter CounterVec, next http.RoundTripper) promhttp.RoundTripperFunc {
	return promhttp.RoundTripperFunc(func(request *http.Request) (*http.Response, error) {
		scheme, ok := request.Context().Value(schemeContextKey{}).(string)
		if !ok {
			scheme = unknown
		}

		response, err := next.RoundTrip(request)

		var labels prometheus.Labels
		if err != nil {
			code := messageDroppedCode
			if response != nil {
				code = strconv.Itoa(response.StatusCode)
			}

			labels = prometheus.Labels{schemeLabel: scheme, codeLabel: code, reasonLabel: getDoErrReason(err)}
		} else {
			labels = prometheus.Labels{schemeLabel: scheme, codeLabel: strconv.Itoa(response.StatusCode), reasonLabel: noErrReason}
			if response.StatusCode != http.StatusAccepted {
				labels[reasonLabel] = non202CodeReason
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
	return promhttp.RoundTripperFunc(
		retry.New(
			retry.Config{
				Retries: o.retries(),
				Check: func(resp *http.Response, err error) bool {
					shouldRetry := retry.DefaultCheck(resp, err)
					if shouldRetry {
						om.Retries.Add(1)
					}

					return shouldRetry
				},
			},
			&http.Client{
				Transport: InstrumentOutboundCounter(
					om.RequestCounter,
					InstrumentOutboundSize(
						om.RequestSize,
						InstrumentOutboundDuration(
							om.RequestDuration,
							promhttp.InstrumentRoundTripperInFlight(om.InFlight, o.transport()),
						),
					),
				),
			},
		).Do,
	)
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
