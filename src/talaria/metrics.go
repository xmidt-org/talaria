package main

import "github.com/Comcast/webpa-common/xmetrics"

const (
	OutboundInFlightGauge = "outbound_inflight"
)

func Metrics() []xmetrics.Metric {
	return []xmetrics.Metric{
		xmetrics.Metric{
			Name: OutboundInFlightGauge,
			Type: "gauge",
		},
	}
}
