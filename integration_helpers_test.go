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

	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/kafka"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/twmb/franz-go/pkg/kgo"
	"github.com/xmidt-org/wrp-go/v5"
)

// ThemisInstance represents a single Themis JWT issuer instance.
// Allows testing with multiple JWT issuers for different endpoints or scenarios.
type ThemisInstance struct {
	Name      string // Instance name (e.g., "device", "api", "admin")
	IssuerURL string // JWT issue endpoint
	KeysURL   string // Public keys endpoint
}

// TalariaTestFixture holds all the test infrastructure for integration tests.
// It provides service URLs, channels for monitoring events, and helper methods
// for making API calls and assertions.
type TalariaTestFixture struct {
	t *testing.T

	// Service URLs
	KafkaBroker string

	// Multiple Themis instances for different JWT scenarios
	// Map key is the instance name (e.g., "device", "api", "admin")
	ThemisInstances map[string]*ThemisInstance

	// Backwards compatibility - points to primary/default Themis instance
	ThemisIssuerURL string
	ThemisKeysURL   string

	CaduceusURL string
	TalariaURL  string

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

// GetJWTFromThemis requests a JWT token from the default Themis issuer service.
// Returns the raw JWT token string.
// For multiple Themis instances, use GetJWTFromThemisInstance(name) instead.
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

// GetThemisInstance returns the Themis instance with the given name.
// Returns nil if the instance doesn't exist.
func (f *TalariaTestFixture) GetThemisInstance(name string) *ThemisInstance {
	return f.ThemisInstances[name]
}

// GetJWTFromThemisInstance requests a JWT token from a specific Themis instance.
// Use this when testing with multiple JWT issuers for different endpoints.
// Returns an error if the instance doesn't exist.
func (f *TalariaTestFixture) GetJWTFromThemisInstance(name string) (string, error) {
	instance := f.GetThemisInstance(name)
	if instance == nil {
		return "", fmt.Errorf("themis instance '%s' not found", name)
	}

	resp, err := http.Get(instance.IssuerURL)
	if err != nil {
		return "", fmt.Errorf("failed to get JWT from Themis instance '%s': %w", name, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("Themis instance '%s' returned status %d: %s", name, resp.StatusCode, string(body))
	}

	tokenBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read JWT response from Themis instance '%s': %w", name, err)
	}

	token := string(tokenBytes)
	return strings.TrimSpace(token), nil
}

// ListThemisInstances returns a slice of all Themis instance names.
func (f *TalariaTestFixture) ListThemisInstances() []string {
	names := make([]string, 0, len(f.ThemisInstances))
	for name := range f.ThemisInstances {
		names = append(names, name)
	}
	return names
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
//
// MULTI-THEMIS SUPPORT: Currently accepts a single themisKeysUrl that's used for both
// device and API endpoints. When Talaria supports separate validators, this can be
// extended to accept deviceThemisUrl and apiThemisUrl separately.
//
// Future signature when Talaria supports 2 JWT validators:
//
//	func setupTalaria(t *testing.T, kafkaBroker, deviceThemisUrl, apiThemisUrl, caduceusUrl, configFile string) func()
//
// Or use a config struct for even more flexibility:
//
//	type TalariaConfig struct {
//	    KafkaBroker      string
//	    DeviceThemisURL  string  // For /api/v2/device WebSocket endpoint
//	    APIThemisURL     string  // For REST API endpoints
//	    CaduceusURL      string
//	    ConfigFile       string
//	}
//	func setupTalaria(t *testing.T, config TalariaConfig) func()
func setupTalaria(t *testing.T, kafkaBroker string, themisURLs map[string]string, caduceusUrl string, configFile string) func() {
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

	// Replace Themis URL placeholders
	// themisURLs is a map of placeholder -> Themis keys URL
	// Example: {"DEVICE_THEMIS_URL": "http://localhost:6500/keys", "API_THEMIS_URL": "http://localhost:6501/keys"}
	for placeholder, keysUrl := range themisURLs {
		themisKeysTemplate := fmt.Sprintf("%s/{keyID}", keysUrl)
		configContent = strings.ReplaceAll(configContent, placeholder, themisKeysTemplate)
		t.Logf("Replacing %s with %s", placeholder, themisKeysTemplate)
	}

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

	// TODO: MULTI-THEMIS CONFIGURATION (2 JWT Validators)
	// Talaria will support 2 separate JWT validators:
	//   1. DEVICE endpoint (WebSocket /api/v2/device)
	//   2. API endpoints (REST GET/POST)
	//
	// Current state: Both endpoints use the same validator (DEVICE_THEMIS_URL placeholder).
	// The config template already has placeholders for both:
	//   - DEVICE_THEMIS_URL (currently used)
	//   - API_THEMIS_URL (ready for when Talaria supports it)
	//
	// When Talaria's config structure is finalized:
	// 1. Update setupTalaria signature to accept deviceThemisUrl and apiThemisUrl separately:
	//    func setupTalaria(t, kafkaBroker, deviceThemisUrl, apiThemisUrl, caduceusUrl, configFile)
	//
	// 2. Update the replacement logic above to use different URLs:
	//    configContent = strings.Replace(configContent, \"DEVICE_THEMIS_URL\", deviceThemisTemplate, -1)
	//    configContent = strings.Replace(configContent, \"API_THEMIS_URL\", apiThemisTemplate, -1)
	//
	// 3. Update setupIntegrationTestWithCapabilities to pass separate URLs based on
	//    config.trustedThemisInstances (e.g., \"device\" instance for device endpoint,
	//    \"api\" instance for API endpoints)
	//
	// This approach allows:
	//   - Testing device endpoint with device-specific Themis
	//   - Testing API endpoints with API-specific Themis
	//   - Testing trusted vs untrusted JWT validation per endpoint category
	//        Resolve:
	//          Templates:
	//            - http://themis-trusted:6500/keys/{keyID}
	//            # Omit untrusted Themis URLs
	//
	// This enables testing:
	//   - Start 2+ Themis instances (all on different dynamic ports)
	//   - Configure Talaria to trust only specific instance(s)
	//   - Test: endpoint accepts JWT from trusted instance
	//   - Test: endpoint rejects JWT from untrusted instance
	//
	// Example test pattern when implemented:
	//   fixture := setupIntegrationTest(t, \"talaria.yaml\",
	//       WithThemisInstance(\"trusted\", \"themis.yaml\"),
	//       WithThemisInstance(\"untrusted\", \"themis.yaml\"),
	//       WithTrustedThemis(\"trusted\"),
	//   )
	//   trustedJWT := fixture.GetJWTFromThemisInstance(\"trusted\")
	//   untrustedJWT := fixture.GetJWTFromThemisInstance(\"untrusted\")
	//   // GET /api/v2/devices with trustedJWT → 200 OK
	//   // GET /api/v2/devices with untrustedJWT → 401 Unauthorized

	// write the test config file
	if err := os.WriteFile(testConfigFile, []byte(configContent), 0600); err != nil {
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

//nolint:unused
func setupXmidtAgent(t *testing.T, themisURL string, debug bool) *exec.Cmd {
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

	// DYNAMIC PORT ALLOCATION: Update Themis URL with the actual dynamic port
	// The xmidt-agent needs to get its JWT from Themis, which now runs on a random port.
	// Replace the hardcoded "http://localhost:6501/issue" with the actual Themis issuer URL.
	configStr = strings.Replace(configStr, "http://localhost:6501/issue", themisURL, 1)
	t.Logf("✓ Updated xmidt-agent config to use Themis at: %s", themisURL)

	// Write test-specific config
	testConfigFile := filepath.Join(testTempDir, "xmidt-agent.yaml")
	if err := os.WriteFile(testConfigFile, []byte(configStr), 0600); err != nil {
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

	// Build command with optional debug flag
	args := []string{}
	if debug {
		args = append(args, "-d")
		t.Log("Starting xmidt-agent with DEBUG mode enabled (-d flag)")
	}
	args = append(args, "-f", testConfigFile)

	simCmd := exec.Command(XmidtAgentBinary, args...)
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
		// DYNAMIC PORT ALLOCATION: No HostConfigModifier - Docker will assign random available ports.
		// This enables multiple Themis instances to run simultaneously for testing trusted vs untrusted JWT validation.
		// We retrieve the actual assigned ports using container.MappedPort() after the container starts.
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
	startKafka           bool
	startThemis          bool
	startCaduceus        bool
	startXmidtAgent      bool
	startDeviceSimulator bool

	// xmidtAgentDebug enables debug logging (-d flag) for xmidt-agent.
	// When false (default), xmidt-agent runs without -d flag (less verbose).
	// When true, enables debug mode which can be very spammy.
	xmidtAgentDebug bool

	// Multiple Themis configurations: map of instance name -> config file
	// Key is the instance name (e.g., "device", "api", "admin")
	// Value is the config file path relative to test_config/ (e.g., "themis_device.yaml")
	themisConfigs map[string]string

	// themisPlaceholders maps instance names to config placeholders they should replace
	// Key is the instance name (e.g., "device"), Value is placeholder (e.g., "DEVICE_THEMIS_URL")
	// If an instance is not in this map, it's a standalone instance (not mapped to config)
	themisPlaceholders map[string]string

	// Default Themis config file (used when startThemis=true but no themisConfigs specified)
	defaultThemisConfig string

	// MULTI-THEMIS CONFIGURATION: Names of Themis instances that Talaria should trust.
	// When empty (default), Talaria trusts all started Themis instances (backwards compatible).
	// When specified, Talaria will only trust JWTs from these named instances.
	// This enables testing: endpoint accepts trusted JWT, rejects untrusted JWT.
	// TODO: Uncomment and use when Talaria supports multiple JWT validators per endpoint.
	trustedThemisInstances []string
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

// WithDeviceSimulator enables xmidt-agent as a docker cotainer in the test setup
func WithDeviceSimulator() setupOption {
	return func(c *setupConfig) { c.startDeviceSimulator = true }
}

// WithXmidtAgentDebug enables xmidt-agent with debug logging (-d flag).
// Debug mode is very verbose and can be spammy.
// Use this when you need detailed logs from xmidt-agent for troubleshooting.
func WithXmidtAgentDebug() setupOption {
	return func(c *setupConfig) {
		c.startXmidtAgent = true
		c.xmidtAgentDebug = true
	}
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

// WithThemisInstance enables a specific Themis instance with the given name and config file.
// This allows testing with multiple JWT issuers for different endpoints.
// The optional placeholder parameter specifies which config placeholder this instance replaces.
// If no placeholder is provided, the instance is standalone (for testing but not mapped to config).
//
// Examples:
//
//	WithThemisInstance("device", "themis_device.yaml", "DEVICE_THEMIS_URL")
//	WithThemisInstance("api", "themis_api.yaml", "API_THEMIS_URL")
//	WithThemisInstance("untrusted", "themis.yaml")  // Standalone, no placeholder
func WithThemisInstance(name, configFile string, placeholder ...string) setupOption {
	return func(c *setupConfig) {
		c.startThemis = true
		if c.themisConfigs == nil {
			c.themisConfigs = make(map[string]string)
		}
		c.themisConfigs[name] = configFile

		// If placeholder provided, map it
		if len(placeholder) > 0 && placeholder[0] != "" {
			if c.themisPlaceholders == nil {
				c.themisPlaceholders = make(map[string]string)
			}
			c.themisPlaceholders[name] = placeholder[0]
		}
	}
}

// WithDeviceThemis enables a Themis instance for device authentication.
// Uses themis.yaml by default for full capabilities.
// Maps to DEVICE_THEMIS_URL placeholder in config.
func WithDeviceThemis() setupOption {
	return WithThemisInstance("device", "themis.yaml", "DEVICE_THEMIS_URL")
}

// WithAPIThemis enables a Themis instance for API authentication.
// Uses themis.yaml by default. Override with WithThemisInstance("api", "custom.yaml", "API_THEMIS_URL").
// Maps to API_THEMIS_URL placeholder in config.
func WithAPIThemis() setupOption {
	return WithThemisInstance("api", "themis.yaml", "API_THEMIS_URL")
}

// WithAdminThemis enables a Themis instance for admin authentication.
// Uses themis.yaml by default. Override with WithThemisInstance("admin", "custom.yaml", "ADMIN_THEMIS_URL").
// Maps to ADMIN_THEMIS_URL placeholder in config.
func WithAdminThemis() setupOption {
	return WithThemisInstance("admin", "themis.yaml", "ADMIN_THEMIS_URL")
}

// WithMultipleThemis enables multiple Themis instances at once.
// Example: WithMultipleThemis(map[string]string{"device": "themis_device.yaml", "api": "themis_api.yaml"})
func WithMultipleThemis(configs map[string]string) setupOption {
	return func(c *setupConfig) {
		c.startThemis = true
		if c.themisConfigs == nil {
			c.themisConfigs = make(map[string]string)
		}
		for name, configFile := range configs {
			c.themisConfigs[name] = configFile
		}
	}
}

// WithTrustedThemis configures Talaria to ONLY trust JWTs from the specified Themis instance(s).
// This enables testing: endpoint accepts trusted JWT, rejects untrusted JWT.
//
// Example usage:
//
//	fixture := setupIntegrationTest(t, "talaria.yaml",
//	    WithThemisInstance("trusted", "themis.yaml"),
//	    WithThemisInstance("untrusted", "themis.yaml"),
//	    WithTrustedThemis("trusted"),  // Only trust the "trusted" instance
//	)
//
//	trustedJWT, _ := fixture.GetJWTFromThemisInstance("trusted")
//	untrustedJWT, _ := fixture.GetJWTFromThemisInstance("untrusted")
//
//	// Test endpoint with trusted JWT - should succeed
//	// Test endpoint with untrusted JWT - should fail
//
// TODO: Currently a no-op. Uncomment the implementation when Talaria supports
// multiple JWT validator configurations per endpoint. For now, Talaria trusts
// ALL Themis instances (uses the first available keys URL).
func WithTrustedThemis(instanceNames ...string) setupOption {
	return func(c *setupConfig) {
		c.trustedThemisInstances = append(c.trustedThemisInstances, instanceNames...)
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

// WithoutDeviceSimulator disables the device simulator
func WithoutDeviceSimulator() setupOption {
	return func(c *setupConfig) { c.startDeviceSimulator = false }
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
		startKafka:          true,
		startThemis:         true,
		startCaduceus:       true,
		startXmidtAgent:     true,
		defaultThemisConfig: themisConfigFile,
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
	themisInstances := make(map[string]*ThemisInstance)

	// 1. Start Kafka (if enabled)
	if config.startKafka {
		_, broker = setupKafka(t)
		t.Logf("✓ Kafka broker started at: %s", broker)
	} else {
		broker = "localhost:9092" // Placeholder if not started
		t.Log("⊘ Kafka disabled")
	}

	// 2. Start Themis instance(s) (if enabled)
	if config.startThemis {
		// If no specific instances configured, start a single default instance
		if len(config.themisConfigs) == 0 {
			_, issuerURL, keysURL := setupThemis(t, config.defaultThemisConfig)
			t.Logf("✓ Themis (default) started at: %s (using %s)", issuerURL, config.defaultThemisConfig)

			// Store as default instance
			themisInstances["default"] = &ThemisInstance{
				Name:      "default",
				IssuerURL: issuerURL,
				KeysURL:   keysURL,
			}

			// Backwards compatibility
			themisIssuerURL = issuerURL
			themisKeysURL = keysURL
		} else {
			// Start multiple Themis instances (each on dynamic ports)
			for name, configFile := range config.themisConfigs {
				_, issuerURL, keysURL := setupThemis(t, configFile)
				t.Logf("✓ Themis instance '%s' started at: %s (using %s)", name, issuerURL, configFile)

				themisInstances[name] = &ThemisInstance{
					Name:      name,
					IssuerURL: issuerURL,
					KeysURL:   keysURL,
				}
			}

			// TODO: MULTI-THEMIS CONFIGURATION
			// When Talaria supports multiple Themis validators, filter instances based on
			// config.trustedThemisInstances. Currently, Talaria will trust ALL instances
			// because it only supports one global JWT validator URL.
			//
			// Future implementation:
			//   var trustedKeysURLs []string
			//   if len(config.trustedThemisInstances) > 0 {
			//       // Only include trusted instances
			//       for _, name := range config.trustedThemisInstances {
			//           if instance, ok := themisInstances[name]; ok {
			//               trustedKeysURLs = append(trustedKeysURLs, instance.KeysURL)
			//           }
			//       }
			//   } else {
			//       // Trust all instances (backwards compatible)
			//       for _, instance := range themisInstances {
			//           trustedKeysURLs = append(trustedKeysURLs, instance.KeysURL)
			//       }
			//   }
			//   // Pass trustedKeysURLs to setupTalaria instead of single themisKeysURL

			// Set default/primary instance for backwards compatibility
			// Prefer "device" instance, fall back to first available
			if instance, ok := themisInstances["device"]; ok {
				themisIssuerURL = instance.IssuerURL
				themisKeysURL = instance.KeysURL
			} else if instance, ok := themisInstances["api"]; ok {
				themisIssuerURL = instance.IssuerURL
				themisKeysURL = instance.KeysURL
			} else {
				// Use first available instance
				for _, instance := range themisInstances {
					themisIssuerURL = instance.IssuerURL
					themisKeysURL = instance.KeysURL
					break
				}
			}
		}
	} else {
		themisIssuerURL = "http://localhost:6501/issue"
		themisKeysURL = "http://localhost:6500/keys"
		t.Log("⊘ Themis disabled")
	}

	// 3. Build Themis URL map for config placeholders
	// Only include instances that have explicit placeholder mappings
	themisURLs := make(map[string]string)
	for name, instance := range themisInstances {
		if placeholder, ok := config.themisPlaceholders[name]; ok {
			themisURLs[placeholder] = instance.KeysURL
			t.Logf("Mapped Themis instance '%s' to placeholder '%s'", name, placeholder)
		} else {
			t.Logf("Themis instance '%s' is standalone (no placeholder mapping)", name)
		}
	}

	// If no explicit mappings but we have a default instance, use backwards-compatible single mapping
	if len(themisURLs) == 0 && themisKeysURL != "" {
		themisURLs["DEVICE_THEMIS_URL"] = themisKeysURL
		t.Logf("Using backwards-compatible single Themis URL for DEVICE_THEMIS_URL")
	}

	// 4. Create channel for Caduceus (if enabled)
	if config.startCaduceus {
		receivedBodyChan = make(chan string, 10) // Buffered to avoid blocking
	}

	// 5. Create mock Caduceus server (if enabled)
	if config.startCaduceus {
		caduceusServer = setupCaduceusMockServer(t, receivedBodyChan)
		t.Logf("✓ Caduceus mock server started at: %s", caduceusServer.URL)
	} else {
		caduceusServer = &httptest.Server{URL: "http://localhost:6000"}
		t.Log("⊘ Caduceus disabled")
	}

	// 6. Start Talaria
	cleanupTalaria := setupTalaria(t, broker, themisURLs, caduceusServer.URL, talariaConfigFile)
	t.Cleanup(cleanupTalaria)
	talariaURL := "http://localhost:6200"
	t.Logf("✓ Talaria started at: %s", talariaURL)

	// 7. Build and start xmidt-agent (if enabled)
	if config.startXmidtAgent {
		simCmd := setupXmidtAgent(t, themisIssuerURL, config.xmidtAgentDebug)
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

	// start local device simulator (instead of xmidt-agent)
	if config.startDeviceSimulator {
		// 4. Build and start device-simulator
		simCmd := setupDeviceSimulator(t, themisIssuerURL)
		if err := simCmd.Start(); err != nil {
			t.Fatalf("Failed to start device-simulator: %v", err)
		}
		t.Logf("✓ Device-simulator started with PID %d", simCmd.Process.Pid)
		t.Cleanup(func() {
			if simCmd.Process != nil {
				t.Log("Stopping device-simulator...")
				simCmd.Process.Kill()
				simCmd.Wait()
				t.Log("✓ device-simulator stopped")
			}
		})
	}

	// 8. Create and return fixture
	fixture := &TalariaTestFixture{
		t:                t,
		KafkaBroker:      broker,
		ThemisInstances:  themisInstances,
		ThemisIssuerURL:  themisIssuerURL, // Backwards compatibility
		ThemisKeysURL:    themisKeysURL,   // Backwards compatibility
		CaduceusURL:      caduceusServer.URL,
		TalariaURL:       talariaURL,
		ReceivedBodyChan: receivedBodyChan,
		httpClient:       http.DefaultClient,
	}

	return fixture
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
