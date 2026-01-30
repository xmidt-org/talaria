// SPDX-FileCopyrightText: 2026 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

//go:build integration

package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/xmidt-org/wrp-go/v5"
)

// TestReceiveOnlineEvent tests that device online events are sent to Caduceus (without Kafka).
func TestReceiveOnlineEvent(t *testing.T) {
	// Setup without Kafka - events only go to Caduceus
	fixture := setupIntegrationTest(t, "talaria_no_kafka_template.yaml",
		WithThemis(),
		WithCaduceus(),
		WithXmidtAgent(),
		WithoutKafka(),
	)

	// Wait for device to connect and events to be published
	time.Sleep(10 * time.Second)

	// Expected online message
	expectedMsg := &wrp.Message{
		Type:        wrp.SimpleEventMessageType,
		Source:      "dns:integration-test.talaria.com",
		Destination: "event:device-status/mac:4ca161000109/online",
	}

	// Verify that Caduceus received the WRP message
	select {
	case receivedBody := <-fixture.ReceivedBodyChan:
		verifyWRPMessage(t, []byte(receivedBody), expectedMsg)
		t.Log("✓ Caduceus received expected WRP message (no Kafka)")
	case <-time.After(5 * time.Second):
		t.Fatal("Timed out waiting for WRP message to be received by Caduceus")
	}
}

// TestReceiveOnlineEventWithKafka tests that device online events are sent to both Kafka and Caduceus.
func TestReceiveOnlineEventWithKafka(t *testing.T) {
	// Setup with Kafka enabled
	fixture := setupIntegrationTest(t, "talaria_integration_template.yaml",
		WithKafka(),
		WithThemis(),
		WithCaduceus(),
		WithXmidtAgent(),
	)

	// Wait for device to connect and events to be published
	time.Sleep(10 * time.Second)

	// Expected online message
	expectedMsg := &wrp.Message{
		Type:        wrp.SimpleEventMessageType,
		Source:      "dns:integration-test.talaria.com",
		Destination: "event:device-status/mac:4ca161000109/online",
	}

	// Verify message in Kafka
	records := consumeMessages(t, fixture.KafkaBroker, "device-events", kafkaMessageConsumeWait)
	require.NotEmpty(t, records, "Expected at least 1 message in Kafka")

	// Find the online event
	foundOnlineEvent := false
	for _, record := range records {
		msg := decodeWRPMessage(t, record.Value)
		if msg.Destination == expectedMsg.Destination {
			foundOnlineEvent = true
			verifyWRPMessage(t, record.Value, expectedMsg)
			require.Equal(t, expectedMsg.Source, string(record.Key), "Partition key should match Source")
			t.Log("✓ Found device-online event in Kafka")
			break
		}
	}
	require.True(t, foundOnlineEvent, "Expected to find device-online event in Kafka")

	// Verify that Caduceus also received the WRP message
	select {
	case receivedBody := <-fixture.ReceivedBodyChan:
		verifyWRPMessage(t, []byte(receivedBody), expectedMsg)
		t.Log("✓ Caduceus received expected WRP message (with Kafka)")
	case <-time.After(5 * time.Second):
		t.Fatal("Timed out waiting for WRP message to be received by Caduceus")
	}
}
