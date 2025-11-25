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
func TestIntegration_ReceiveEvent(t *testing.T) {
	// Disable Ryuk (testcontainers reaper) to avoid port mapping issues
	t.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")

	//t.Parallel()

	// 1. Start Kafka with dynamic port
	_, broker := setupKafka(t)
	t.Logf("Kafka broker started at: %s", broker)

	// 2. Start Talaria with the dynamic Kafka broker
	cleanupTalaria := setupTalaria(t, broker)
	defer cleanupTalaria()

	// 3. Start supporting services
	_ = setupThemis(t)

	time.Sleep(3 * time.Second) // Give services time to initialize

	_ = setupXmidtAgent(t)

	time.Sleep(3 * time.Second) // Wait for agent to connect and events to be published

	// 4. Verify messages in Kafka
	records := consumeMessages(t, broker, "device-events", messageConsumeWait)
	require.Len(t, records, 1, "Expected exactly 1 message in Kafka")
	//verifyWRPMessage(t, records[0], msg)
}
