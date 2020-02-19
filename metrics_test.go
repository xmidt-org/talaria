package main

import (
	"github.com/xmidt-org/webpa-common/xmetrics"
)

// NewTestOutboundMeasures creates an OutboundMeasures appropriate for a testing environment
func NewTestOutboundMeasures() OutboundMeasures {
	return NewOutboundMeasures(xmetrics.MustNewRegistry(nil, Metrics))
}
