// SPDX-FileCopyrightText: 2017 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"github.com/xmidt-org/sallust"
	"github.com/xmidt-org/touchstone"
)

// nolint:staticcheck

// NewTestOutboundMeasures creates an OutboundMeasures appropriate for a testing environment
func NewTestOutboundMeasures() (OutboundMeasures, error) {
	cfg := touchstone.Config{
		DefaultNamespace: "n",
		DefaultSubsystem: "s",
	}
	_, pr, err := touchstone.New(cfg)
	if err != nil {
		return OutboundMeasures{}, err
	}

	tf := touchstone.NewFactory(cfg, sallust.Default(), pr)
	return NewOutboundMeasures(tf)
}
