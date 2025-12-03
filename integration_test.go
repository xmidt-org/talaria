// SPDX-FileCopyrightText: 2025 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

//go:build integration

package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestIntegration_ReceiveEvent tests basic event publishing to Kafka.
//
// Verifies:
// -  device connect and disconnect messages are published to kafka
// - Verification of message content in Kafka
// DO NOT ENABLE - not working because xmidt-agent is running in a test-container and
// is therefore still not able to connect with talaria which is running on the host.
// This is just a single test for now, but more can be added via table tests later. 
func TestIntegration_ReceiveEvent(t *testing.T) {
	// Disable Ryuk (testcontainers reaper) to avoid port mapping issues
	t.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")

	// 1. Start Kafka with dynamic port
	_, broker := setupKafka(t)
	t.Logf("Kafka broker started at: %s", broker)

	// 2. Start supporting services
	_ = setupThemis(t)

	// 3. Start Talaria with the dynamic Kafka broker
	cleanupTalaria := setupTalaria(t, broker)
	defer cleanupTalaria()

	time.Sleep(3 * time.Second) // Give services time to initialize

	// this never connects to talaria, so no events are generated
	_ = setupXmidtAgent(t)

	//time.Sleep(30 * time.Second) // Wait for agent to connect and events to be published
	// 4. Verify messages in Kafka
	records := consumeMessages(t, broker, "device-events", messageConsumeWait)
	require.Len(t, records, 1, "Expected exactly 1 message in Kafka")
	//verifyWRPMessage(t, records[0], msg)
}
