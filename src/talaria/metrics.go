package main

import (
	"net/http"
	"time"

	"github.com/Comcast/webpa-common/xmetrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	OutboundInFlightGauge   = "outbound_inflight"
	OutboundRequestDuration = "outbound_request_duration_seconds"
)

func Metrics() []xmetrics.Metric {
	return []xmetrics.Metric{
		xmetrics.Metric{
			Name: OutboundInFlightGauge,
			Type: "gauge",
			Help: "The number of active, in-flight requests from devices",
		},
		xmetrics.Metric{
			Name:       OutboundRequestDuration,
			Type:       "histogram",
			LabelNames: []string{"uri"},
		},
	}
}

type OutboundMeasures struct {
	InFlight        prometheus.Gauge
	RequestDuration prometheus.ObserverVec
}

func NewOutboundMeasures(r xmetrics.Registry) OutboundMeasures {
	return OutboundMeasures{
		InFlight:        r.NewGaugeVec(OutboundInFlightGauge).WithLabelValues(),
		RequestDuration: r.NewHistogramVec(OutboundRequestDuration),
	}
}

func InstrumentOutboundRequestDuration(obs prometheus.ObserverVec, next http.RoundTripper) promhttp.RoundTripperFunc {
	return promhttp.RoundTripperFunc(func(request *http.Request) (*http.Response, error) {
		start := time.Now()
		response, err := next.RoundTrip(request)
		if err == nil {
			obs.With(prometheus.Labels{"uri": request.URL.String()}).Observe(time.Since(start).Seconds())
		}

		return response, err
	})
}

// NewOutboundRoundTripper produces an http.RoundTripper from the configured Outbounder
// that is also decorated with appropriate metrics.
func NewOutboundRoundTripper(om OutboundMeasures, o *Outbounder) http.RoundTripper {
	return InstrumentOutboundRequestDuration(
		om.RequestDuration,
		promhttp.InstrumentRoundTripperInFlight(om.InFlight, o.transport()),
	)
}
