// SPDX-FileCopyrightText: 2025 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

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

	"github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
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

// TalariaTestFixture holds all the test infrastructure for integration tests.
// It provides service URLs, channels for monitoring events, and helper methods
// for making API calls and assertions.
type TalariaTestFixture struct {
	t *testing.T

	// Service URLs
	KafkaBroker     string
	ThemisIssuerURL string
	ThemisKeysURL   string
	CaduceusURL     string
	TalariaURL      string

	// Channels for monitoring
	ReceivedBodyChan chan string

	// HTTP client
	httpClient *http.Client
}

// NewRequest creates an HTTP request to Talaria without any authentication.
// Use WithBasicAuth(), WithBearerToken(), etc. to add authentication.
func (f *TalariaTestFixture) NewRequest(method, path string, body io.Reader) (*http.Request, error) {
	url := f.TalariaURL + path
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	return req, nil
}

// WithBasicAuth adds Basic authentication to a request.
// Pass empty username/password to test with invalid credentials.
func (f *TalariaTestFixture) WithBasicAuth(req *http.Request, username, password string) *http.Request {
	req.SetBasicAuth(username, password)
	return req
}

// WithBearerToken adds Bearer token authentication to a request.
func (f *TalariaTestFixture) WithBearerToken(req *http.Request, token string) *http.Request {
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

// WithContentType sets the Content-Type header on a request.
func (f *TalariaTestFixture) WithContentType(req *http.Request, contentType string) *http.Request {
	req.Header.Set("Content-Type", contentType)
	return req
}

// Do executes an HTTP request and returns the response.
func (f *TalariaTestFixture) Do(req *http.Request) (*http.Response, error) {
	return f.httpClient.Do(req)
}

// DoAndReadBody executes an HTTP request and returns the response body and status code.
// Automatically closes the response body.
func (f *TalariaTestFixture) DoAndReadBody(req *http.Request) (body string, statusCode int, err error) {
	resp, err := f.Do(req)
	if err != nil {
		return "", 0, err
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", resp.StatusCode, err
	}

	return string(bodyBytes), resp.StatusCode, nil
}

// GET makes a GET request to the specified path with optional auth.
// If username is empty, no auth is added.
func (f *TalariaTestFixture) GET(path string, username, password string) (body string, statusCode int, err error) {
	req, err := f.NewRequest("GET", path, nil)
	if err != nil {
		return "", 0, err
	}

	if username != "" {
		f.WithBasicAuth(req, username, password)
	}

	return f.DoAndReadBody(req)
}

// POST makes a POST request with the specified body and optional auth.
// If username is empty, no auth is added.
func (f *TalariaTestFixture) POST(path string, body io.Reader, contentType string, username, password string) (respBody string, statusCode int, err error) {
	req, err := f.NewRequest("POST", path, body)
	if err != nil {
		return "", 0, err
	}

	if contentType != "" {
		f.WithContentType(req, contentType)
	}

	if username != "" {
		f.WithBasicAuth(req, username, password)
	}

	return f.DoAndReadBody(req)
}

// PUT makes a PUT request with the specified body and optional auth.
// If username is empty, no auth is added.
func (f *TalariaTestFixture) PUT(path string, body io.Reader, contentType string, username, password string) (respBody string, statusCode int, err error) {
	req, err := f.NewRequest("PUT", path, body)
	if err != nil {
		return "", 0, err
	}

	if contentType != "" {
		f.WithContentType(req, contentType)
	}

	if username != "" {
		f.WithBasicAuth(req, username, password)
	}

	return f.DoAndReadBody(req)
}

// DELETE makes a DELETE request with optional auth.
// If username is empty, no auth is added.
func (f *TalariaTestFixture) DELETE(path string, username, password string) (body string, statusCode int, err error) {
	req, err := f.NewRequest("DELETE", path, nil)
	if err != nil {
		return "", 0, err
	}

	if username != "" {
		f.WithBasicAuth(req, username, password)
	}

	return f.DoAndReadBody(req)
}

// GetDevices makes a GET request to /api/v2/devices with basic auth.
func (f *TalariaTestFixture) GetDevices(username, password string) (string, int, error) {
	return f.GET("/api/v2/devices", username, password)
}

// GetJWTFromThemis requests a JWT token from the Themis issuer service.
// Returns the raw JWT token string.
func (f *TalariaTestFixture) GetJWTFromThemis() (string, error) {
	resp, err := http.Get(f.ThemisIssuerURL)
	if err != nil {
		return "", fmt.Errorf("failed to get JWT from Themis: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Themis returned status %d: %s", resp.StatusCode, string(body))
	}

	tokenBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read JWT response: %w", err)
	}

	token := string(tokenBytes)
	// Trim any whitespace/newlines
	token = strings.TrimSpace(token)

	return token, nil
}

// WaitForCaduceusMessage waits for a message to arrive at the mock Caduceus server.
func (f *TalariaTestFixture) WaitForCaduceusMessage(timeout time.Duration) (string, error) {
	select {
	case msg := <-f.ReceivedBodyChan:
		return msg, nil
	case <-time.After(timeout):
		return "", fmt.Errorf("timeout waiting for Caduceus message")
	}
}

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
	// Generate unique output filename based on the template name
	// Place it in workspace root where Viper will find it
	configTemplateFile := filepath.Join(workspaceRoot, "test_config", configFile)
	// Strip .yaml extension and add _test.yaml suffix for output file
	baseConfigName := strings.TrimSuffix(configFile, ".yaml")
	testConfigFile := filepath.Join(workspaceRoot, baseConfigName+"_test.yaml")

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
	if err := os.WriteFile(testConfigFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}
	t.Logf("Created test config file: %s", testConfigFile)

	// 3. Start Talaria as a subprocess. Pass just the base config name without .yaml extension
	// Viper will look for <name>.yaml in its configured directories (including workspaceRoot)
	talariaCmd := exec.Command(talariaTestBinary, "--file", baseConfigName+"_test")

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

	// 1. Create an unstarted mock HTTP server
	testServer := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

		// Disable keep-alive to ensure clean shutdown
		w.Header().Set("Connection", "close")
		w.WriteHeader(http.StatusOK)
	}))

	// Disable HTTP keep-alive on the server
	testServer.Config.SetKeepAlivesEnabled(false)

	// 2. Create a listener on a specific port (e.g., 7000)
	listener, err := net.Listen("tcp", "127.0.0.1:6000")
	if err != nil {
		t.Fatalf("Failed to create listener on port 6000: %v", err)
	}

	// 3. Set the listener and start the server
	testServer.Listener = listener
	testServer.Start()

	t.Logf("Caduceus mock server started at: %s", testServer.URL)

	// Register cleanup to ensure server closes properly
	t.Cleanup(func() {
		testServer.CloseClientConnections()
		testServer.Close()
	})

	return testServer
}

func setupXmidtAgent(t *testing.T, themisURL string) *exec.Cmd {
	t.Helper()

	// Get the workspace root directory
	_, filename, _, _ := runtime.Caller(0)
	workspaceRoot := filepath.Dir(filename)
	XmidtAgentDir := filepath.Join(workspaceRoot, "cmd", "xmidt-agent", "cmd", "xmidt-agent")
	XmidtAgentBinary := filepath.Join(XmidtAgentDir, "xmidt-agent")
	XmidtAgentConfigTemplate := filepath.Join(workspaceRoot, "test_config", "xmidt-agent.yaml")

	// Create a test-specific temporary directory for token storage (parallel-safe)
	testTempDir := t.TempDir() // Automatically cleaned up after test
	tokenCacheDir := filepath.Join(testTempDir, "temporary")
	durableCacheDir := filepath.Join(testTempDir, "durable")

	// Create directories
	if err := os.MkdirAll(tokenCacheDir, 0755); err != nil {
		t.Fatalf("Failed to create token cache directory: %v", err)
	}
	if err := os.MkdirAll(durableCacheDir, 0755); err != nil {
		t.Fatalf("Failed to create durable cache directory: %v", err)
	}

	// Create a test-specific config file with unique storage paths
	configContent, err := os.ReadFile(XmidtAgentConfigTemplate)
	if err != nil {
		t.Fatalf("Failed to read device simulator config template: %v", err)
	}

	// Replace storage paths with test-specific directories
	// hardcoded paths in the template for now
	configStr := string(configContent)
	configStr = strings.Replace(configStr, "./local-rdk-testing/temporary", tokenCacheDir, 1)
	configStr = strings.Replace(configStr, "./local-rdk-testing/durable", durableCacheDir, 1)

	// Write test-specific config
	testConfigFile := filepath.Join(testTempDir, "xmidt-agent.yaml")
	if err := os.WriteFile(testConfigFile, []byte(configStr), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}
	t.Logf("✓ Created test-specific config with isolated storage")

	t.Log("Building device-simulator...")
	buildCmd := exec.Command("go", "build", "-o", XmidtAgentBinary, ".")
	buildCmd.Dir = XmidtAgentDir
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Logf("Build output: %s", string(output))
		t.Fatalf("Failed to build device-simulator: %v", err)
	}
	t.Log("✓ Device-simulator built successfully")

	simCmd := exec.Command(XmidtAgentBinary,
		"-d",
		"-f", testConfigFile,
	)
	simCmd.Dir = workspaceRoot
	simCmd.Stdout = os.Stdout
	simCmd.Stderr = os.Stderr

	return simCmd

}

func setupThemis(t *testing.T, themisConfigFile string) (*testcontainers.Container, string, string) {
	t.Helper()

	// Skip if running in short mode
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx := context.Background()

	// Get the workspace root directory
	_, filename, _, _ := runtime.Caller(0)
	workspaceRoot := filepath.Dir(filename)
	themisConfigPath := filepath.Join(workspaceRoot, "test_config", themisConfigFile)

	// Configure testcontainers to use Podman if DOCKER_HOST is set
	configureTestContainersForPodman(t)

	req := testcontainers.ContainerRequest{
		Image:        "ghcr.io/xmidt-org/themis:latest-amd64",
		ExposedPorts: []string{"6500/tcp", "6501/tcp", "6502/tcp", "6503/tcp", "6504/tcp"},
		HostConfigModifier: func(hc *container.HostConfig) {
			// Fix ports for predictable access from config files
			hc.PortBindings = nat.PortMap{
				"6500/tcp": []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: "6500"}},
				"6501/tcp": []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: "6501"}},
				"6502/tcp": []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: "6502"}},
				"6503/tcp": []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: "6503"}},
				"6504/tcp": []nat.PortBinding{{HostIP: "0.0.0.0", HostPort: "6504"}},
			}
		},
		Hostname:   "themis",
		WaitingFor: wait.ForHTTP("/health").WithPort("6504/tcp").WithStartupTimeout(60 * time.Second),
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

// setupOption configures which services to start in integration tests
type setupOption func(*setupConfig)

// setupConfig holds configuration for which services to start
type setupConfig struct {
	startKafka      bool
	startThemis     bool
	startCaduceus   bool
	startXmidtAgent bool
}

// WithKafka enables Kafka container in the test setup
func WithKafka() setupOption {
	return func(c *setupConfig) { c.startKafka = true }
}

// WithThemis enables Themis JWT issuer container in the test setup
func WithThemis() setupOption {
	return func(c *setupConfig) { c.startThemis = true }
}

// WithCaduceus enables Caduceus mock server in the test setup
func WithCaduceus() setupOption {
	return func(c *setupConfig) { c.startCaduceus = true }
}

// WithXmidtAgent enables xmidt-agent device simulator in the test setup
func WithXmidtAgent() setupOption {
	return func(c *setupConfig) { c.startXmidtAgent = true }
}

// WithFullStack enables all services (Kafka, Themis, Caduceus, xmidt-agent)
func WithFullStack() setupOption {
	return func(c *setupConfig) {
		c.startKafka = true
		c.startThemis = true
		c.startCaduceus = true
		c.startXmidtAgent = true
	}
}

// WithAPIServices enables services needed for API testing (Themis, Caduceus)
func WithAPIServices() setupOption {
	return func(c *setupConfig) {
		c.startThemis = true
		c.startCaduceus = true
	}
}

// WithoutKafka disables Kafka (useful with WithFullStack to exclude one service)
func WithoutKafka() setupOption {
	return func(c *setupConfig) { c.startKafka = false }
}

// WithoutThemis disables Themis
func WithoutThemis() setupOption {
	return func(c *setupConfig) { c.startThemis = false }
}

// WithoutCaduceus disables Caduceus
func WithoutCaduceus() setupOption {
	return func(c *setupConfig) { c.startCaduceus = false }
}

// WithoutXmidtAgent disables xmidt-agent
func WithoutXmidtAgent() setupOption {
	return func(c *setupConfig) { c.startXmidtAgent = false }
}

// setupIntegrationTest creates and starts all services needed for integration testing.
// Returns a TalariaTestFixture with all service URLs and helper methods.
// All cleanup is automatically registered with t.Cleanup().
// Uses themis.yaml by default. For custom capabilities, use setupIntegrationTestWithCapabilities().
//
// By default, starts all services. Use options to customize which services to start:
//
//	setupIntegrationTest(t, "config.yaml", WithThemis(), WithCaduceus()) // Only Themis and Caduceus
//	setupIntegrationTest(t, "config.yaml", WithFullStack()) // All services
//	setupIntegrationTest(t, "config.yaml", WithAPIServices()) // Themis + Caduceus
func setupIntegrationTest(t *testing.T, configFile string, opts ...setupOption) *TalariaTestFixture {
	return setupIntegrationTestWithCapabilities(t, configFile, "themis.yaml", opts...)
}

// setupIntegrationTestWithCapabilities creates and starts all services needed for integration testing
// with a custom Themis configuration for testing different JWT capabilities.
// Returns a TalariaTestFixture with all service URLs and helper methods.
// All cleanup is automatically registered with t.Cleanup().
//
// By default, starts all services. Use options to customize which services to start:
//
//	setupIntegrationTestWithCapabilities(t, "talaria.yaml", "themis_custom.yaml", WithThemis(), WithCaduceus())
func setupIntegrationTestWithCapabilities(t *testing.T, talariaConfigFile, themisConfigFile string, opts ...setupOption) *TalariaTestFixture {
	t.Helper()

	// Default: start all services (backwards compatible)
	config := setupConfig{
		startKafka:      true,
		startThemis:     true,
		startCaduceus:   true,
		startXmidtAgent: true,
	}

	// Apply options
	for _, opt := range opts {
		opt(&config)
	}

	// Disable Ryuk (testcontainers reaper) to avoid port mapping issues
	t.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")

	var broker string
	var themisIssuerURL, themisKeysURL string
	var caduceusServer *httptest.Server
	var receivedBodyChan chan string

	// 1. Start Kafka (if enabled)
	if config.startKafka {
		_, broker = setupKafka(t)
		t.Logf("✓ Kafka broker started at: %s", broker)
	} else {
		broker = "localhost:9092" // Placeholder if not started
		t.Log("⊘ Kafka disabled")
	}

	// 2. Start Themis (if enabled)
	if config.startThemis {
		_, themisIssuerURL, themisKeysURL = setupThemis(t, themisConfigFile)
		t.Logf("✓ Themis issuer started at: %s (using %s)", themisIssuerURL, themisConfigFile)
		t.Logf("✓ Themis keys started at: %s", themisKeysURL)
	} else {
		themisIssuerURL = "http://localhost:6501/issue"
		themisKeysURL = "http://localhost:6500/keys"
		t.Log("⊘ Themis disabled")
	}

	// 3. Create channel for Caduceus (if enabled)
	if config.startCaduceus {
		receivedBodyChan = make(chan string, 10) // Buffered to avoid blocking
	}

	// 4. Create mock Caduceus server (if enabled)
	if config.startCaduceus {
		caduceusServer = setupCaduceusMockServer(t, receivedBodyChan)
		t.Logf("✓ Caduceus mock server started at: %s", caduceusServer.URL)
	} else {
		caduceusServer = &httptest.Server{URL: "http://localhost:6000"}
		t.Log("⊘ Caduceus disabled")
	}

	// 5. Start Talaria
	cleanupTalaria := setupTalaria(t, broker, themisKeysURL, caduceusServer.URL, talariaConfigFile)
	t.Cleanup(cleanupTalaria)
	talariaURL := "http://localhost:6200"
	t.Logf("✓ Talaria started at: %s", talariaURL)

	// 6. Build and start xmidt-agent (if enabled)
	if config.startXmidtAgent {
		simCmd := setupXmidtAgent(t, themisIssuerURL)
		if err := simCmd.Start(); err != nil {
			t.Fatalf("Failed to start xmidt-agent: %v", err)
		}
		t.Logf("✓ xmidt-agent started with PID %d", simCmd.Process.Pid)

		// Register cleanup for xmidt-agent
		t.Cleanup(func() {
			if simCmd.Process != nil {
				t.Log("Stopping xmidt-agent...")
				simCmd.Process.Kill()
				simCmd.Wait()
				t.Log("✓ xmidt-agent stopped")
			}
		})

		// 7. Wait for device to connect
		t.Log("Waiting for device to connect...")
		time.Sleep(10 * time.Second)
		t.Log("✓ Device connection window complete")
	} else {
		t.Log("⊘ xmidt-agent disabled")
	}

	// 8. Create and return fixture
	fixture := &TalariaTestFixture{
		t:                t,
		KafkaBroker:      broker,
		ThemisIssuerURL:  themisIssuerURL,
		ThemisKeysURL:    themisKeysURL,
		CaduceusURL:      caduceusServer.URL,
		TalariaURL:       talariaURL,
		ReceivedBodyChan: receivedBodyChan,
		httpClient:       http.DefaultClient,
	}

	return fixture
}
