// SPDX-FileCopyrightText: 2025 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

//go:build integration

package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/xmidt-org/wrp-go/v5"
)

// TODO - get environment variables to work - prefixed by applicationName
// TODO - refactor common code with other integration tests
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

	// 2. Start supporting services for themis and "caduceus"
	_, themisIssuerUrl, themisKeysUrl := setupThemis(t)
	t.Logf("Themis issuer started at: %s", themisIssuerUrl)
	t.Logf("Themis keys started at: %s", themisKeysUrl)

	// Channel for "caduceus" to receive wrp message
	receivedBodyChan := make(chan string, 1)

	// 1. Create a mock caduceus server using httptest.Server
	testServer := setupCaduceusMockServer(t, receivedBodyChan)
	defer testServer.Close() // Clean up the server after the test

	// TODO - pass in config file and try to use env variables
	// 3. Start Talaria with the dynamic Kafka broker and Themis keys URL
	cleanupTalaria := setupTalaria(t, broker, themisKeysUrl, testServer.URL)
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

	// TODO - put in test struct
	msg := &wrp.Message{
		Type:        wrp.SimpleEventMessageType,
		Source:      "dns:integration-test.talaria.com",
		Destination: "event:device-status/mac:4ca161000109/online",
	}
	verifyWRPMessage(t, records[0].Value, msg)
	require.Equal(t, msg.Source, string(records[0].Key), "Partition key should match Source")

	// 7. Verify that the mock server received the expected WRP message
	select {
	case receivedBody := <-receivedBodyChan:
		fmt.Println(receivedBody)
		verifyWRPMessage(t, []byte(receivedBody), msg)
		t.Log("✓ Mock server received expected WRP message")
	case <-time.After(5 * time.Second):
		t.Fatal("Timed out waiting for WRP message to be received by mock server")
	}
}
