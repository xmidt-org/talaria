// SPDX-FileCopyrightText: 2025 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

//go:build integration

package main

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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

// setupTalaria builds and starts Talaria as a subprocess with the given Kafka broker.
// Returns a cleanup function to stop Talaria.
func setupTalaria(t *testing.T, kafkaBroker string, themisKeysUrl string, caduceusUrl string, configFile string) func() {
	t.Helper()

	ctx := context.Background()

	// Get the workspace root directory (parent of the test file)
	_, filename, _, _ := runtime.Caller(0)
	workspaceRoot := filepath.Dir(filename)

	// 1. Build Talaria binary
	t.Log("Building Talaria...")
	talariaTestBinary := filepath.Join(workspaceRoot, "talaria_test")
	buildCmd := exec.CommandContext(ctx, "go", "build", "-o", talariaTestBinary, ".")
	buildCmd.Dir = workspaceRoot
	buildOutput, err := buildCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Failed to build Talaria: %v\nOutput: %s", err, buildOutput)
	}
	t.Log("✓ Talaria built successfully")

	// 2. Create a test config file with dynamic external service values
	// (Gave up on getting environment variables to work)
	configTemplateFile := filepath.Join(workspaceRoot, "test_config", configFile)
	testConfigFile := filepath.Join(workspaceRoot, "talaria_test.yaml")

	// read the config template
	configTemplate, err := os.ReadFile(configTemplateFile)
	if err != nil {
		t.Fatalf("Failed to read template config: %v", err)
	}

	// for now, just replace the values we need directly since the environment variable replacement is still broken
	configContent := string(configTemplate)
	configContent = strings.Replace(configContent,
		"THEMIS_URL",
		fmt.Sprintf("%s/{keyID}", themisKeysUrl),
		1)
	// // Replace Kafka broker
	configContent = strings.Replace(configContent,
		"KAFKA_BROKER",
		kafkaBroker,
		1)

	// // Replace Caduceus URL
	configContent = strings.Replace(configContent,
		"CADUCEUS_URL",
		caduceusUrl,
		1)

	// write the test config file
	//nolint:gosec
	if err := os.WriteFile(testConfigFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}
	t.Logf("Created test config file: %s", testConfigFile)

	// 3. Start Talaria as a subprocess.  It will use talaria_test for the app name for viper
	talariaCmd := exec.Command(talariaTestBinary, "--file", "talaria_test")

	// something is still not working with environment variables, so stuck doing the substitutions above
	// talariaCmd.Env = os.Environ()
	// talariaCmd.Env = append(talariaCmd.Env, fmt.Sprintf("TALARIA_TEST_JWTVALIDATOR_CONFIG_RESOLVE_TEMPLATE=%s/{keyID}", themisKeysUrl))
	// talariaCmd.Env = append(talariaCmd.Env, fmt.Sprintf("TALARIA_TEST_KAFKA_BROKERS_0=%s", kafkaBroker))
	// talariaCmd.Env = append(talariaCmd.Env, fmt.Sprintf("TALARIA_TEST_DEVICE_OUTBOUND_EVENTENDPOINTS_DEFAULT=%s", caduceusUrl))

	talariaCmd.Dir = workspaceRoot

	t.Logf("Using JWT validator template: %s/{keyID}", themisKeysUrl)
	t.Logf("Using Kafka broker: %s", kafkaBroker)
	t.Logf("Using Caduceus URL: %s", caduceusUrl)

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
		os.Remove(talariaTestBinary)
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
func decodeWRPMessage(t *testing.T, in []byte) *wrp.Message {
	t.Helper()

	var msg wrp.Message
	decoder := wrp.NewDecoderBytes(in, wrp.Msgpack)
	err := decoder.Decode(&msg)
	require.NoError(t, err, "Failed to decode WRP message")

	return &msg
}

// TODO environment variables and other cleanup.  allow for multiple tests in a table
func verifyWRPMessage(t *testing.T, msg []byte, expected *wrp.Message) {
	t.Helper()

	actual := decodeWRPMessage(t, msg)

	// Verify fields
	require.Equal(t, expected.Type, actual.Type, "Message type mismatch")
	require.Equal(t, expected.Source, actual.Source, "Source mismatch")
	require.Equal(t, expected.Destination, actual.Destination, "Destination mismatch")
}

func setupCaduceusMockServer(t *testing.T, receivedBodyChan chan<- string) *httptest.Server {
	t.Helper()

	// 1. Create a mock HTTP server using httptest.Server
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("Expected POST request, got %s", r.Method)
		}

		// Read and capture the request body
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("Failed to read request body: %v", err)
		}
		r.Body.Close()

		receivedBodyChan <- string(body) // Send the body to the channel
		w.WriteHeader(http.StatusOK)
	}))

	t.Logf("Caduceus mock server started at: %s", testServer.URL)

	return testServer
}

func setupDeviceSimulator(t *testing.T, themisURL string) *exec.Cmd {
	t.Helper()

	// Get the workspace root directory
	_, filename, _, _ := runtime.Caller(0)
	workspaceRoot := filepath.Dir(filename)
	deviceSimulatorDir := filepath.Join(workspaceRoot, "cmd", "device-simulator")
	deviceSimulatorBinary := filepath.Join(deviceSimulatorDir, "device-simulator")

	t.Log("Building device-simulator...")
	buildCmd := exec.Command("go", "build", "-o", deviceSimulatorBinary, ".")
	buildCmd.Dir = deviceSimulatorDir
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Logf("Build output: %s", string(output))
		t.Fatalf("Failed to build device-simulator: %v", err)
	}
	t.Log("✓ Device-simulator built successfully")

	simCmd := exec.Command(deviceSimulatorBinary,
		"-themis", themisURL,
		"-talaria", "ws://localhost:6200/api/v2/device",
		"-device-id", "mac:4ca161000109",
		"-serial", "1800deadbeef",
		"-ping-interval", "10s",
	)
	simCmd.Dir = workspaceRoot
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

	// Get the workspace root directory
	_, filename, _, _ := runtime.Caller(0)
	workspaceRoot := filepath.Dir(filename)
	themisConfigPath := filepath.Join(workspaceRoot, "test_config", "themis.yaml")

	// Configure testcontainers to use Podman if DOCKER_HOST is set
	configureTestContainersForPodman(t)

	req := testcontainers.ContainerRequest{
		Image:        "ghcr.io/xmidt-org/themis:latest-amd64",
		ExposedPorts: []string{"6500/tcp", "6501/tcp", "6502/tcp", "6503/tcp", "6504/tcp"},
		Hostname:     "themis",
		WaitingFor:   wait.ForHTTP("/health").WithPort("6504/tcp").WithStartupTimeout(60 * time.Second),
		Files: []testcontainers.ContainerFile{
			{
				HostFilePath:      themisConfigPath,
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
