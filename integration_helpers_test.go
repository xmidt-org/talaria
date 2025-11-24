// SPDX-FileCopyrightText: 2025 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/kafka"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/xmidt-org/wrp-go/v5"
)

const (
	messageConsumeWait = 10 * time.Second
)

// configureTestContainersForPodman is a no-op since the Makefile sets the required
// environment variables (DOCKER_HOST, TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE).
// We keep this function for backwards compatibility but don't set anything to avoid
// race conditions with testcontainers' internal caching.
func configureTestContainersForPodman(t *testing.T) {
	t.Helper()
	// Environment variables are set by the Makefile before running tests.
	// Nothing to do here.
}

func setupDevice(t *testing.T) {
	t.Helper()

	// Skip if running in short mode
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Environment variables are set by the Makefile before running tests.
}

// setupKafka starts Kafka using testcontainers and returns the container and broker address.
// Automatically registers cleanup to stop Kafka when test completes.
func setupKafka(t *testing.T) (*kafka.KafkaContainer, string) {
	t.Helper()

	// Skip if running in short mode
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Configure testcontainers to use Podman if DOCKER_HOST is set
	configureTestContainersForPodman(t)

	// Start Kafka container
	// Use confluent-local image which is designed for testcontainers
	// Using specific version tag since testcontainers validates version for KRaft mode
	kafkaContainer, err := kafka.Run(ctx,
		"confluentinc/confluent-local:7.8.0",
		kafka.WithClusterID("test-cluster"),
	)
	require.NoError(t, err, "Failed to start Kafka container")

	t.Cleanup(func() {
		t.Log("Stopping Kafka container...")
		if err := kafkaContainer.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate Kafka container: %v", err)
		}
	})

	// Get broker address
	brokers, err := kafkaContainer.Brokers(ctx)
	require.NoError(t, err, "Failed to get Kafka brokers")
	require.NotEmpty(t, brokers, "No Kafka brokers available")

	broker := brokers[0]
	t.Logf("Kafka broker available at: %s", broker)

	// Verify Kafka is accepting connections
	require.NoError(t, waitForKafka(ctx, t, broker))

	return kafkaContainer, broker
}

// waitForKafka attempts to connect to Kafka broker until it responds or timeout.
func waitForKafka(ctx context.Context, t *testing.T, broker string) error {
	t.Helper()

	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		client, err := kgo.NewClient(
			kgo.SeedBrokers(broker),
			kgo.RequestTimeoutOverhead(5*time.Second),
		)
		if err == nil {
			// Try to ping broker to verify it's responsive
			pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := client.Ping(pingCtx)
			cancel()
			client.Close()

			if err == nil {
				t.Log("Kafka is ready!")
				return nil
			}
			t.Logf("Kafka not ready yet: %v", err)
		}

		time.Sleep(1 * time.Second)
	}

	return context.DeadlineExceeded
}

// consumeMessages consumes messages from a Kafka topic with a timeout.
// Returns all messages received before timeout.
func consumeMessages(t *testing.T, broker string, topic string, timeout time.Duration) []*kgo.Record {
	t.Helper()

	client, err := kgo.NewClient(
		kgo.SeedBrokers(broker),
		kgo.ConsumeTopics(topic),
		kgo.ConsumeResetOffset(kgo.NewOffset().AtStart()),
	)
	require.NoError(t, err, "Failed to create Kafka consumer")
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	var records []*kgo.Record
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		fetches := client.PollFetches(ctx)
		if fetches.IsClientClosed() {
			break
		}

		fetches.EachError(func(topic string, partition int32, err error) {
			t.Logf("Fetch error on %s[%d]: %v", topic, partition, err)
		})

		fetches.EachRecord(func(r *kgo.Record) {
			records = append(records, r)
		})

		// If we got records, give a bit more time for any additional ones
		if len(records) > 0 {
			time.Sleep(500 * time.Millisecond)
			// Try one more fetch
			fetches = client.PollFetches(ctx)
			fetches.EachRecord(func(r *kgo.Record) {
				records = append(records, r)
			})
			break
		}

		time.Sleep(100 * time.Millisecond)
	}

	return records
}

// decodeWRPMessage decodes a msgpack-encoded WRP message from a Kafka record.
func decodeWRPMessage(t *testing.T, record *kgo.Record) *wrp.Message {
	t.Helper()

	var msg wrp.Message
	decoder := wrp.NewDecoderBytes(record.Value, wrp.Msgpack)
	err := decoder.Decode(&msg)
	require.NoError(t, err, "Failed to decode WRP message")

	return &msg
}

// verifyWRPMessage verifies that a Kafka record contains the expected WRP message.
func verifyWRPMessage(t *testing.T, record *kgo.Record, expected *wrp.Message) {
	t.Helper()

	actual := decodeWRPMessage(t, record)

	// Verify key fields
	require.Equal(t, expected.Type, actual.Type, "Message type mismatch")
	require.Equal(t, expected.Source, actual.Source, "Source mismatch")
	require.Equal(t, expected.Destination, actual.Destination, "Destination mismatch")
	require.Equal(t, string(expected.Payload), string(actual.Payload), "Payload mismatch")

	// Verify partition key matches device ID
	require.Equal(t, expected.Source, string(record.Key), "Partition key should match Source")
}

// createTestMessage creates a WRP message for testing.
func createTestMessage(eventType string, deviceID string, qos int64) *wrp.Message {
	return &wrp.Message{
		Type:             wrp.SimpleEventMessageType,
		Source:           deviceID,
		Destination:      "event:" + eventType + "/" + deviceID,
		Payload:          []byte(`{"status":"online"}`),
		QualityOfService: wrp.QOSValue(qos),
	}
}

// setupKafka starts Kafka using testcontainers and returns the container and broker address.
// Automatically registers cleanup to stop Kafka when test completes.
func setupXmidtAgent(t *testing.T) (*testcontainers.Container, string) {
	t.Helper()

	// Skip if running in short mode
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Configure testcontainers to use Podman if DOCKER_HOST is set
	configureTestContainersForPodman(t)

	req := testcontainers.ContainerRequest{
		Image:        "ghcr.io/xmidt-org/xmidt-agent:v0.9.0",
		ExposedPorts: []string{"9999/tcp"},
		//Cmd:          []string{"sh", "-c", "cat /app/config.txt && sleep 60"}, // Command to read the file and keep container alive
		WaitingFor:   wait.ForLog("This is a configuration file."),
		Files: []testcontainers.ContainerFile{
			{
				HostFilePath:      "./xmidt-agent.yaml",
				ContainerFilePath: "/etc/xmidt-agent/xmidt-agent.yaml",
				FileMode:          0644, // Optional: specify file permissions in the container
			},
		},
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "Failed to start Xmidt Agent container")


	t.Cleanup(func() {
		t.Log("Stopping Xmidt-Agent container...")
		if err := container.Terminate(ctx); err != nil {
			fmt.Printf("failed to terminate container: %v", err)
		}
	})

	return &container, "9999"
}
