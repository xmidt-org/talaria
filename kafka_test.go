// SPDX-FileCopyrightText: 2026 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

//go:build integration

package main

import (
	"testing"
)

func TestReceiveOnlineEvent(t *testing.T) {
	runIt(t, testConfig{
		configFile:   "talaria_no_kafka_template.yaml",
		writeToKafka: false,
	})
}

func TestReceiveOnlineEventWithKafka(t *testing.T) {
	runIt(t, testConfig{
		configFile:   "talaria_template.yaml",
		writeToKafka: true,
	})
}
