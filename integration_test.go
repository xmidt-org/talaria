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
// - Device connect messages are published to Kafka
// - Message content in Kafka is correct
//
// This test uses the device-simulator to connect to Talaria and generate events.
func TestIntegration_ReceiveOnlineEvent(t *testing.T) {
	// Disable Ryuk (testcontainers reaper) to avoid port mapping issues
	t.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")

	// 1. Start Kafka with dynamic port
	_, broker := setupKafka(t)
	t.Logf("Kafka broker started at: %s", broker)

	// 2. Start supporting services
	_, themisIssuerUrl, themisKeysUrl := setupThemis(t)
	t.Logf("Themis issuer started at: %s", themisIssuerUrl)
	t.Logf("Themis keys started at: %s", themisKeysUrl)

	// 3. Start Talaria with the dynamic Kafka broker and Themis keys URL
	cleanupTalaria := setupTalaria(t, broker, themisKeysUrl)
	defer cleanupTalaria()

	// 4. Build and start device-simulator
	simCmd := setupDeviceSimulator(t, themisIssuerUrl)
	if err := simCmd.Start(); err != nil {
		t.Fatalf("Failed to start device-simulator: %v", err)
	}
	t.Logf("✓ Device-simulator started with PID %d", simCmd.Process.Pid)

	// Cleanup: Kill the simulator when test ends
	defer func() {
		if simCmd.Process != nil {
			t.Log("Stopping device-simulator...")
			simCmd.Process.Kill()
			simCmd.Wait()
		}
	}()

	// Wait for device to connect and events to be published
	time.Sleep(10 * time.Second)

	// 6. Verify messages in Kafka
	records := consumeMessages(t, broker, "device-events", messageConsumeWait)
	require.Len(t, records, 1, "Expected exactly 1 message in Kafka")
	//verifyWRPMessage(t, records[0], msg)
}

// TODO - problems
// unless wrp QOS is set to CriticalValue, the messages do not flush to kafka, even with a short linger
// we don't want to modify talaria production code for conditional test stuff - how can we inject this?
// we need allowAutoTopicCreation set to true, it's not coming through in the config
