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

const (
	OutboundInFlightGauge         = "outbound_inflight"
	OutboundRequestDuration       = "outbound_request_duration_seconds"
	OutboundRequestCounter        = "outbound_requests"
	OutboundQueueSize             = "outbound_queue_size"
	OutboundDroppedMessageCounter = "outbound_dropped_messages"
	OutboundRetries               = "outbound_retries"
	GateStatus                    = "gate_status"
	DrainStatus                   = "drain_status"
	DrainCounter                  = "drain_count"
)

func Metrics() []xmetrics.Metric {
	return []xmetrics.Metric{
		xmetrics.Metric{
			Name: OutboundInFlightGauge,
			Type: "gauge",
			Help: "The number of active, in-flight requests from devices",
		},
		xmetrics.Metric{
			Name:    OutboundRequestDuration,
			Type:    "histogram",
			Help:    "The durations of outbound requests from devices",
			Buckets: []float64{.25, .5, 1, 2.5, 5, 10},
		},
		xmetrics.Metric{
			Name:       OutboundRequestCounter,
			Type:       "counter",
			Help:       "The count of outbound requests",
			LabelNames: []string{"code"},
		},
		xmetrics.Metric{
			Name: OutboundQueueSize,
			Type: "gauge",
			Help: "The current number of requests waiting to be sent outbound",
		},
		xmetrics.Metric{
			Name: OutboundDroppedMessageCounter,
			Type: "counter",
			Help: "The total count of messages dropped due to a full outbound queue",
		},
		xmetrics.Metric{
			Name: OutboundRetries,
			Type: "counter",
			Help: "The total count of outbound HTTP retries",
		},
		xmetrics.Metric{
			Name: GateStatus,
			Type: "gauge",
			Help: "Indicates whether the device gate is open (1.0) or closed (0.0)",
		},
		xmetrics.Metric{
			Name: DrainStatus,
			Type: "gauge",
			Help: "Indicates whether a device drain operation is currently running",
		},
		xmetrics.Metric{
			Name: DrainCounter,
			Type: "counter",
			Help: "The total count of devices disconnected due to a drain since the server started",
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
