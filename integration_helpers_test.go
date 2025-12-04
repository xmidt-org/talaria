// SPDX-FileCopyrightText: 2025 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"syscall"
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
	messageConsumeWait = 60 * time.Second
)

// testLogConsumer is a custom log consumer that sends container logs to the test logger
type testLogConsumer struct {
	t      *testing.T
	prefix string
}

func (lc *testLogConsumer) Accept(l testcontainers.Log) {
	// Log to test output with prefix
	lc.t.Logf("[%s] %s", lc.prefix, string(l.Content))
}

func newTestLogConsumer(t *testing.T, prefix string) *testLogConsumer {
	return &testLogConsumer{t: t, prefix: prefix}
}

// // setupTalariaEnv configures environment variables for Talaria to use the given Kafka broker.
// // Viper will automatically read these environment variables.
// // Returns a cleanup function to restore the original environment.
// func setupTalariaEnv(t *testing.T, kafkaBroker string) func() {
// 	t.Helper()

// 	// Viper uses environment variables with prefix matching the key path
// 	// For nested config like kafka.brokers, we can set KAFKA_BROKERS
// 	originalBrokers := os.Getenv("KAFKA_BROKERS")
// 	originalEnabled := os.Getenv("KAFKA_ENABLED")
// 	originalTopic := os.Getenv("KAFKA_TOPIC")

// 	os.Setenv("KAFKA_ENABLED", "true")
// 	os.Setenv("KAFKA_TOPIC", "device-events")
// 	os.Setenv("KAFKA_BROKERS", kafkaBroker)

// 	t.Logf("Set Kafka environment: KAFKA_BROKERS=%s", kafkaBroker)

// 	cleanup := func() {
// 		// Restore original values
// 		if originalBrokers != "" {
// 			os.Setenv("KAFKA_BROKERS", originalBrokers)
// 		} else {
// 			os.Unsetenv("KAFKA_BROKERS")
// 		}
// 		if originalEnabled != "" {
// 			os.Setenv("KAFKA_ENABLED", originalEnabled)
// 		} else {
// 			os.Unsetenv("KAFKA_ENABLED")
// 		}
// 		if originalTopic != "" {
// 			os.Setenv("KAFKA_TOPIC", originalTopic)
// 		} else {
// 			os.Unsetenv("KAFKA_TOPIC")
// 		}
// 	}

// 	return cleanup
// }

// setupTalaria builds and starts Talaria as a subprocess with the given Kafka broker.
// Returns a cleanup function to stop Talaria.
func setupTalaria(t *testing.T, kafkaBroker string, themisKeysUrl string) func() {
	t.Helper()

	ctx := context.Background()

	// 1. Build Talaria binary
	t.Log("Building Talaria...")
	buildCmd := exec.CommandContext(ctx, "go", "build", "-o", "talaria-test", ".")
	buildCmd.Dir = "/Users/mpicci200/comcast/talaria"
	buildOutput, err := buildCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to build Talaria: %v\nOutput: %s", err, buildOutput)
	}
	t.Log("✓ Talaria built successfully")

	// 2. Create a test config file with dynamic external service values
	// (Gave up on getting environment variables to work)
	originalConfigFile := "test_config/talaria.yaml"
	testConfigFile := "talaria-test.yaml"

	// read the original config
	originalContent, err := os.ReadFile(originalConfigFile)
	if err != nil {
		t.Fatalf("Failed to read original config: %v", err)
	}

	// replace just the JWT validator template and Kafka settings
	configContent := string(originalContent)
	configContent = strings.Replace(configContent,
		`Template: "http://localhost:6500/keys/{keyID}"`,
		fmt.Sprintf(`Template: "%s/{keyID}"`, themisKeysUrl),
		1)
	// Replace Kafka broker
	configContent = strings.Replace(configContent,
		`- (( grab $KAFKA_BROKERS || "http://localhost:9092" ))`,
		fmt.Sprintf(`- "%s"`, kafkaBroker),
		1)

	// write the test config file
	if err := os.WriteFile(testConfigFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}
	t.Logf("Created test config file: %s", testConfigFile)

	// 3. Start Talaria as a subprocess.  It will use talaria-test for the app name for viper
	talariaCmd := exec.Command("./talaria-test")

	t.Logf("Using JWT validator template: %s/{keyID}", themisKeysUrl)
	t.Logf("Using Kafka broker: %s", kafkaBroker) // Send stdout/stderr directly to console
	talariaCmd.Stdout = os.Stdout
	talariaCmd.Stderr = os.Stderr

	// Start the process
	if err := talariaCmd.Start(); err != nil {
		t.Fatalf("Failed to start Talaria: %v", err)
	}

	t.Logf("✓ Talaria started with PID %d", talariaCmd.Process.Pid)

	// Wait for Talaria to be ready (check if port 6200 is open)
	if err := waitForTalariaReady(t, 30*time.Second); err != nil {
		talariaCmd.Process.Kill()
		talariaCmd.Wait()
		t.Fatalf("Talaria failed to start: %v", err)
	}

	t.Log("✓ Talaria is ready and accepting connections")

	// Return cleanup function
	cleanup := func() {
		t.Log("Stopping Talaria...")
		if talariaCmd.Process != nil {
			// Send SIGTERM for graceful shutdown
			talariaCmd.Process.Signal(syscall.SIGTERM)

			// Wait for graceful shutdown with timeout
			done := make(chan error, 1)
			go func() {
				done <- talariaCmd.Wait()
			}()

			select {
			case <-time.After(10 * time.Second):
				// Force kill if graceful shutdown takes too long
				t.Log("Forcing Talaria shutdown...")
				talariaCmd.Process.Kill()
				talariaCmd.Wait()
			case err := <-done:
				if err != nil && err.Error() != "signal: terminated" {
					t.Logf("Talaria exited with error: %v", err)
				}
			}
		}

		// Remove test binary and config files
		os.Remove("talaria-test")
		os.Remove(testConfigFile)

		t.Log("✓ Talaria stopped")
	}

	return cleanup
}

// waitForTalariaReady waits for Talaria to be ready by checking if the primary port is accepting connections.
func waitForTalariaReady(t *testing.T, timeout time.Duration) error {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", "localhost:6200", 1*time.Second)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for Talaria to be ready")
}

// configureTestContainersForPodman is a no-op since the Makefile sets the required
// environment variables (DOCKER_HOST, TESTCONTAINERS_DOCKER_SOCKET_OVERRIDE).
// We keep this function for backwards compatibility but don't set anything to avoid
// race conditions with testcontainers' internal caching.
func configureTestContainersForPodman(t *testing.T) {
	t.Helper()
	// Environment variables are set by the Makefile before running tests.
	// Nothing to do here.
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
		"confluentinc/confluent-local:7.5.0",
		kafka.WithClusterID("test-cluster"),
	)
	require.NoError(t, err, "Failed to start Kafka container")

	// Create log consumer to stream Kafka logs to test output
	logConsumer := newTestLogConsumer(t, "Kafka")

	// Start streaming logs
	if err := kafkaContainer.StartLogProducer(ctx); err != nil {
		t.Logf("Warning: Failed to start log producer for Kafka: %v", err)
	} else {
		kafkaContainer.FollowOutput(logConsumer)
		t.Log("Kafka log streaming enabled")
	}

	// Give logs a moment to start streaming
	time.Sleep(1 * time.Second)

	t.Cleanup(func() {
		t.Log("Stopping Kafka container...")
		if err := kafkaContainer.StopLogProducer(); err != nil {
			t.Logf("Warning: Failed to stop log producer: %v", err)
		}
		if err := kafkaContainer.Terminate(ctx); err != nil {
			t.Logf("Failed to terminate Kafka container: %v", err)
		}
	})

	// Get broker address
	brokers, err := kafkaContainer.Brokers(ctx)
	fmt.Println("Brokers:", brokers)
	require.NoError(t, err, "Failed to get Kafka brokers")
	require.NotEmpty(t, brokers, "No Kafka brokers available")

	broker := brokers[0]
	t.Logf("Kafka broker available at: %s", broker)

	return kafkaContainer, broker
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

	fmt.Printf("Starting to consume messages from topic %s and broker %s...\n", topic, broker)

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

func setupDeviceSimulator(t *testing.T, themisURL string) *exec.Cmd {
	t.Log("Building device-simulator...")
	buildCmd := exec.Command("go", "build", "-o", "device-simulator", ".")
	buildCmd.Dir = "./cmd/device-simulator"
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Logf("Build output: %s", string(output))
		t.Fatalf("Failed to build device-simulator: %v", err)
	}
	t.Log("✓ Device-simulator built successfully")

	simCmd := exec.Command("./device-simulator",
		"-themis", themisURL,
		"-talaria", "ws://localhost:6200/api/v2/device",
		"-device-id", "mac:4ca161000109",
		"-serial", "1800deadbeef",
		"-ping-interval", "10s",
	)
	simCmd.Dir = "./cmd/device-simulator"
	simCmd.Stdout = os.Stdout
	simCmd.Stderr = os.Stderr

	return simCmd

}

func setupThemis(t *testing.T) (*testcontainers.Container, string, string) {
	t.Helper()

	// Skip if running in short mode
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Configure testcontainers to use Podman if DOCKER_HOST is set
	configureTestContainersForPodman(t)

	req := testcontainers.ContainerRequest{
		Image:        "ghcr.io/xmidt-org/themis:latest-amd64",
		ExposedPorts: []string{"6500/tcp", "6501/tcp", "6502/tcp", "6503/tcp", "6504/tcp"},
		Hostname:     "themis",
		WaitingFor:   wait.ForHTTP("/health").WithPort("6504/tcp").WithStartupTimeout(60 * time.Second),
		Files: []testcontainers.ContainerFile{
			{
				HostFilePath:      "test_config/themis.yaml",
				ContainerFilePath: "/etc/themis/themis.yaml",
				FileMode:          0644, // Optional: specify file permissions in the container
			},
		},
	}

	t.Log("Starting Themis container...")
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "Failed to start Themis container")

	t.Log("Themis container started, setting up log streaming...")

	// Create log consumer and start log producer
	logConsumer := newTestLogConsumer(t, "Themis")
	if err := container.StartLogProducer(ctx); err != nil {
		t.Logf("Warning: Failed to start log producer for Themis: %v", err)
	} else {
		container.FollowOutput(logConsumer)
		t.Log("Themis log streaming enabled")
	}

	// Get the mapped port for Themis
	host, err := container.Host(ctx)
	require.NoError(t, err, "Failed to get Themis host")

	port, err := container.MappedPort(ctx, "6500")
	require.NoError(t, err, "Failed to get Themis key port")

	themisKeysUrl := fmt.Sprintf("http://%s:%s/keys", host, port.Port())
	t.Logf("Themis keys available at: %s", themisKeysUrl)

	port, err = container.MappedPort(ctx, "6501")
	require.NoError(t, err, "Failed to get Themis issuer port")

	themisIssuerUrl := fmt.Sprintf("http://%s:%s/issue", host, port.Port())
	t.Logf("Themis issuer available at: %s", themisIssuerUrl)

	// Give logs a moment to start streaming
	time.Sleep(2 * time.Second)

	t.Cleanup(func() {
		t.Log("Stopping Themis container...")
		if err := container.StopLogProducer(); err != nil {
			t.Logf("Warning: Failed to stop log producer: %v", err)
		}
		if err := container.Terminate(ctx); err != nil {
			fmt.Printf("failed to terminate container: %v", err)
		}
	})

	return &container, themisIssuerUrl, themisKeysUrl
}
