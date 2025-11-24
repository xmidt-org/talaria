// SPDX-FileCopyrightText: 2025 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

//go:build integration

package wrpkafka_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xmidt-org/wrpkafka"
)

// TestIntegration_ReceiveEvent tests basic event publishing to Kafka.
//
// Verifies:
// -  device connect and disconnect messages are published to kafka
// - Verification of message content in Kafka
func TestIntegration_ReceiveEvent(t *testing.T) {
	t.Parallel()
	_, broker := setupKafka(t)	

	_, _ := setupXmidtAgent(t)

	time.Sleep(5 * time.Second) // wait for Talaria to start and connect to Kafka
	
	// Verify message in Kafka
	records := consumeMessages(t, broker, "device-events", messageConsumeWait)
	require.Len(t, records, 1, "Expected exactly 1 message in Kafka")
	verifyWRPMessage(t, records[0], msg)
}
