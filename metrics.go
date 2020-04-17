package main

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-kit/kit/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/xmidt-org/webpa-common/xhttp"
	"github.com/xmidt-org/webpa-common/xmetrics"
)

// Metric names
const (
	OutboundInFlightGauge         = "outbound_inflight"
	OutboundRequestDuration       = "outbound_request_duration_seconds"
	OutboundRequestCounter        = "outbound_requests"
	OutboundQueueSize             = "outbound_queue_size"
	OutboundDroppedMessageCounter = "outbound_dropped_messages"
	OutboundRetries               = "outbound_retries"

	GateStatus   = "gate_status"
	DrainStatus  = "drain_status"
	DrainCounter = "drain_count"

	ReceivedWRPMessageCount = "received_wrp_message_total"
)

// Metric label names
const (
	OutcomeLabel = "outcome"
	ReasonLabel  = "reason"
)

// label values
const (
	Accepted = "accepted"
	Rejected = "rejected"

	DeviceNotFound = "device_not_found"
	InvalidWRPDest = "invalid_wrp_dest"

	MissingDeviceCredential = "missing_device_cred"
	MissingWRPCredential    = "missing_wrp_cred"
	CheckExecFail           = "check_exec_fail"
	Unauthorized            = "unauthorized"
	Authorized              = "authorized"
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
			LabelNames: []string{"code"},
		},
		{
			Name: OutboundQueueSize,
			Type: xmetrics.GaugeType,
			Help: "The current number of requests waiting to be sent outbound",
		},
		{
			Name: OutboundDroppedMessageCounter,
			Type: xmetrics.CounterType,
			Help: "The total count of messages dropped due to a full outbound queue",
		},
		{
			Name: OutboundRetries,
			Type: xmetrics.CounterType,
			Help: "The total count of outbound HTTP retries",
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
			Name:       ReceivedWRPMessageCount,
			Type:       xmetrics.CounterType,
			Help:       "Number of WRP Messages successfully decoded and ready to route to device.",
			LabelNames: []string{OutcomeLabel, ReasonLabel},
		},
	}
}

type OutboundMeasures struct {
	InFlight        prometheus.Gauge
	RequestDuration prometheus.Observer
	RequestCounter  *prometheus.CounterVec
	QueueSize       metrics.Gauge
	Retries         metrics.Counter
	DroppedMessages metrics.Counter
}

func NewOutboundMeasures(r xmetrics.Registry) OutboundMeasures {
	return OutboundMeasures{
		InFlight:        r.NewGaugeVec(OutboundInFlightGauge).WithLabelValues(),
		RequestDuration: r.NewHistogramVec(OutboundRequestDuration).WithLabelValues(),
		RequestCounter:  r.NewCounterVec(OutboundRequestCounter),
		QueueSize:       r.NewGauge(OutboundQueueSize),
		Retries:         r.NewCounter(OutboundRetries),
		DroppedMessages: r.NewCounter(OutboundDroppedMessageCounter),
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
		if err == nil {
			// use "200" as the result from a 0 or negative status code, to be consistent with other golang APIs
			labels := prometheus.Labels{"code": "200"}
			if response.StatusCode > 0 {
				labels["code"] = strconv.Itoa(response.StatusCode)
			}

			counter.With(labels).Inc()
		}

		return response, err
	})
}

// NewOutboundRoundTripper produces an http.RoundTripper from the configured Outbounder
// that is also decorated with appropriate metrics.
func NewOutboundRoundTripper(om OutboundMeasures, o *Outbounder) http.RoundTripper {
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
