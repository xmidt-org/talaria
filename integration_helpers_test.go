// SPDX-FileCopyrightText: 2025 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/kafka"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/xmidt-org/wrp-go/v5"
)

const (
	messageConsumeWait = 10 * time.Second
)

// setupTalariaConfig creates a temporary config file for Talaria with the given Kafka broker.
// Returns the path to the config file and a cleanup function.
func setupTalariaConfig(t *testing.T, kafkaBroker string) (string, func()) {
	t.Helper()

	// Create a temporary config file based on talaria.yaml
	// but with the dynamic Kafka broker address
	configContent := fmt.Sprintf(`---
server: "talaria-test"
build: "test"
region: "test"
flavor: "test"

primary:
  address: ":6200"

health:
  address: ":6201"

pprof:
  address: ":6202"

control:
  address: ":6203"

metric:
  address: ":6204"
  metricsOptions:
    namespace: "xmidt"
    subsystem: "talaria"

log:
  file: "stdout"
  level: "DEBUG"
  json: true

device:
  manager:
    wrpSourceCheck:
      type: monitor
    upgrader:
      handshakeTimeout: "10s"
    maxDevices: 100
    deviceMessageQueueSize: 1000
    pingPeriod: "1m"
    writeTimeout: "2m"
    idlePeriod: "2m"

  outbound:
    method: "POST"
    retries: 3
    eventEndpoints:
      default: http://localhost:6000/api/v4/notify
    requestTimeout: "2m"
    defaultScheme: "http"
    allowedSchemes:
      - "http"
      - "https"
    outboundQueueSize: 2000
    workerPoolSize: 50
    transport:
      maxIdleConns: 0
      maxIdleConnsPerHost: 100
      idleConnTimeout: "120s"
    clientTimeout: "2m"

service:
  defaultScheme: http
  vnodeCount: 211
  fixed:
    - http://localhost:6200

kafka:
  enabled: true
  topic: "device-events"
  brokers:
    - "%s"
  maxBufferedRecords: 10000
  maxBufferedBytes: 104857600
  maxRetries: 3
  requestTimeout: "30s"
  tls:
    enabled: false
  sasl:
    mechanism: ""
  initialDynamicConfig:
    topicMap:
      - pattern: "*"
        topic: "device-events"
    compression: "snappy"

zap:
  outputPaths:
    - stdout
  level: debug
  errorOutputPaths:
    - stderr
  disableCaller: true
  encoderConfig:
    messageKey: message
    levelKey: key
    callerKey: caller
    levelEncoder: lowercase
  encoding: json

failOpen: true
`, kafkaBroker)

	// Write to temporary file
	tmpFile, err := os.CreateTemp("", "talaria-test-*.yaml")
	require.NoError(t, err, "Failed to create temp config file")

	_, err = tmpFile.WriteString(configContent)
	require.NoError(t, err, "Failed to write config file")

	err = tmpFile.Close()
	require.NoError(t, err, "Failed to close config file")

	cleanup := func() {
		os.Remove(tmpFile.Name())
	}

	t.Logf("Created Talaria config file: %s", tmpFile.Name())
	return tmpFile.Name(), cleanup
}

// setupTalariaEnv configures environment variables for Talaria to use the given Kafka broker.
// Viper will automatically read these environment variables.
// Returns a cleanup function to restore the original environment.
func setupTalariaEnv(t *testing.T, kafkaBroker string) func() {
	t.Helper()

	// Viper uses environment variables with prefix matching the key path
	// For nested config like kafka.brokers, we can set KAFKA_BROKERS
	originalBrokers := os.Getenv("KAFKA_BROKERS")
	originalEnabled := os.Getenv("KAFKA_ENABLED")
	originalTopic := os.Getenv("KAFKA_TOPIC")

	os.Setenv("KAFKA_ENABLED", "true")
	os.Setenv("KAFKA_TOPIC", "device-events")
	os.Setenv("KAFKA_BROKERS", kafkaBroker)

	t.Logf("Set Kafka environment: KAFKA_BROKERS=%s", kafkaBroker)

	cleanup := func() {
		// Restore original values
		if originalBrokers != "" {
			os.Setenv("KAFKA_BROKERS", originalBrokers)
		} else {
			os.Unsetenv("KAFKA_BROKERS")
		}
		if originalEnabled != "" {
			os.Setenv("KAFKA_ENABLED", originalEnabled)
		} else {
			os.Unsetenv("KAFKA_ENABLED")
		}
		if originalTopic != "" {
			os.Setenv("KAFKA_TOPIC", originalTopic)
		} else {
			os.Unsetenv("KAFKA_TOPIC")
		}
	}

	return cleanup
}

// setupTalaria builds and starts Talaria as a subprocess with the given Kafka broker.
// Returns a cleanup function to stop Talaria.
func setupTalaria(t *testing.T, kafkaBroker string) func() {
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

	// 2. Create a config file with dynamic Kafka broker
	//configFile, cleanupConfig := setupTalariaConfig(t, kafkaBroker)

	// 3. Start Talaria as a subprocess
	//talariaCmd := exec.Command("./talaria-test", "--file", "./talaria-test.yaml")
	talariaCmd := exec.Command("./talaria-test", )
	talariaCmd.Dir = "."

	// Set environment variables
	talariaCmd.Env = append(os.Environ(),
		"KAFKA_ENABLED=true",
		"KAFKA_TOPIC=device-events",
		"KAFKA_BROKERS="+kafkaBroker,
	)

	// Capture stdout/stderr for debugging
	stdout, err := talariaCmd.StdoutPipe()
	if err != nil {
		t.Fatalf("Failed to get stdout pipe: %v", err)
	}
	stderr, err := talariaCmd.StderrPipe()
	if err != nil {
		t.Fatalf("Failed to get stderr pipe: %v", err)
	}

	// Start the process
	if err := talariaCmd.Start(); err != nil {
		//cleanupConfig()
		t.Fatalf("Failed to start Talaria: %v", err)
	}

	// Log output in background
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			t.Logf("[Talaria] %s", scanner.Text())
		}
	}()
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			t.Logf("[Talaria ERROR] %s", scanner.Text())
		}
	}()

	t.Logf("✓ Talaria started with PID %d", talariaCmd.Process.Pid)

	// Wait for Talaria to be ready (check if port 6200 is open)
	if err := waitForTalariaReady(t, 30*time.Second); err != nil {
		talariaCmd.Process.Kill()
		talariaCmd.Wait()
		//cleanupConfig()
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

		// Clean up config file
		//cleanupConfig()

		// Remove test binary
		os.Remove("talaria-test")

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
		"confluentinc/confluent-local:7.5.0",
		kafka.WithClusterID("test-cluster"),
		// Wait for Kafka to be ready by checking the broker logs
		// testcontainers.WithWaitStrategy(
		// 	wait.ForLog(".*started \\(kafka.server.KafkaRaftServer\\).*").
		// 		WithStartupTimeout(60*time.Second),
		// ),
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
	fmt.Println("Brokers:", brokers)
	require.NoError(t, err, "Failed to get Kafka brokers")
	require.NotEmpty(t, brokers, "No Kafka brokers available")

	broker := brokers[0]
	t.Logf("Kafka broker available at: %s", broker)

	// Verify Kafka is accepting connections
	//require.NoError(t, waitForKafka(ctx, t, broker))

	return kafkaContainer, broker
}

// waitForKafka attempts to connect to Kafka broker until it responds or timeout.
func waitForKafka(ctx context.Context, t *testing.T, broker string) error {
	t.Helper()

	t.Logf("Waiting for Kafka broker at %s to be ready...", broker)
	deadline := time.Now().Add(60 * time.Second) // Increased timeout
	var lastErr error

	for time.Now().Before(deadline) {
		client, err := kgo.NewClient(
			kgo.SeedBrokers(broker),
			kgo.RequestTimeoutOverhead(10*time.Second),
		)
		if err != nil {
			lastErr = fmt.Errorf("failed to create client: %w", err)
			t.Logf("Kafka not ready yet (client creation failed): %v", err)
			time.Sleep(2 * time.Second)
			continue
		}

		// Try to ping broker to verify it's responsive
		pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err = client.Ping(pingCtx)
		cancel()
		client.Close()

		if err == nil {
			t.Log("✓ Kafka is ready and responsive!")
			return nil
		}

		lastErr = fmt.Errorf("ping failed: %w", err)
		t.Logf("Kafka not ready yet (ping failed): %v", err)
		time.Sleep(2 * time.Second)
	}

	return fmt.Errorf("kafka not ready after timeout: %w", lastErr)
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
func setupXmidtAgent(t *testing.T) *testcontainers.Container {
	t.Helper()

	// Skip if running in short mode
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Configure testcontainers to use Podman if DOCKER_HOST is set
	configureTestContainersForPodman(t)

	req := testcontainers.ContainerRequest{
		Image: "ghcr.io/xmidt-org/xmidt-agent:v0.9.5-amd64",
		//ExposedPorts: []string{"9999/tcp"},
		//Cmd:          []string{"sh", "-c", "cat /app/config.txt && sleep 60"}, // Command to read the file and keep container alive
		//WaitingFor:   wait.ForLog("This is a configuration file."),
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
			fmt.Printf("failed to terminate xmidt-agent: %v", err)
		}
	})

	return &container
}

func setupThemis(t *testing.T) *testcontainers.Container {
	t.Helper()

	// Skip if running in short mode
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Configure testcontainers to use Podman if DOCKER_HOST is set
	configureTestContainersForPodman(t)

	req := testcontainers.ContainerRequest{
		Image:        "ghcr.io/xmidt-org/themis",
		ExposedPorts: []string{"6501/tcp"},
		//WaitingFor:   wait.ForLog("Listening on :6501"),
	}
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	require.NoError(t, err, "Failed to start Themis container")

	t.Cleanup(func() {
		t.Log("Stopping Themis container...")
		if err := container.Terminate(ctx); err != nil {
			fmt.Printf("failed to terminate container: %v", err)
		}
	})

	return &container
}
