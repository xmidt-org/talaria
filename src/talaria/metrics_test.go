package main

import (
	"github.com/Comcast/webpa-common/xmetrics"
)

// NewTestOutboundMeasures creates an OutboundMeasures appropriate for a testing environment
func NewTestOutboundMeasures() OutboundMeasures {
	return NewOutboundMeasures(xmetrics.MustNewRegistry(nil, Metrics))
}
