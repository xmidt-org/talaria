package main

import (
	// nolint:staticcheck
	"github.com/xmidt-org/webpa-common/v2/xmetrics"
)

// NewTestOutboundMeasures creates an OutboundMeasures appropriate for a testing environment
func NewTestOutboundMeasures() OutboundMeasures {
	return NewOutboundMeasures(xmetrics.MustNewRegistry(nil, Metrics))
}
