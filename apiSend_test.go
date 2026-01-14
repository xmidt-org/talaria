//go:build integration

package main

import (
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// simply starts up the cluster.
func TestHelloWorld(t *testing.T) {
	// this looks to be broken, with talaria picking up talaria.yaml instead
	talariaTestConfigFile := "talaria_template.yaml"
	// this sets up a talaria cluster with kafka, themis, and a mock caduceus server.
	// Disable Ryuk (testcontainers reaper) to avoid port mapping issues
	t.Setenv("TESTCONTAINERS_RYUK_DISABLED", "true")

	// 1. Start Kafka with dynamic port
	_, broker := setupKafka(t)
	t.Logf("Kafka broker started at: %s", broker)

	// 2a. Start supporting services for themis
	_, themisIssuerUrl, themisKeysUrl := setupThemis(t)
	t.Logf("Themis issuer started at: %s", themisIssuerUrl)
	t.Logf("Themis keys started at: %s", themisKeysUrl)

	// Channel for "caduceus" to receive wrp message
	receivedBodyChan := make(chan string, 1)

	// 2b. Create a mock caduceus server using httptest.Server
	testServer := setupCaduceusMockServer(t, receivedBodyChan)

	// 3. Start Talaria with the dynamic Kafka broker and Themis keys URL
	cleanupTalaria := setupTalaria(t, broker, themisKeysUrl, testServer.URL, talariaTestConfigFile)
	defer cleanupTalaria()

	// 4. Build and start device-simulator
	simCmd := setupXmidtAgent(t, themisIssuerUrl)
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

	//TODO: execute test here
	t.Logf("THIS IS A PLACEHOLDER TEST - NO ASSERTIONS")

	req, err := http.NewRequest("GET", "http://localhost:6200/api/v2/devices", nil)
	require.NoError(t, err)

	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")

	resp, err := http.DefaultClient.Do(req)
	defer resp.Body.Close()

	require.NoError(t, err)

	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	t.Logf("***************************")
	t.Logf("Status: %s", resp.Status)
	t.Logf("Response: %s", string(body))
	t.Logf("***************************")

	//runIt(t, testConfig{
	//	configFile:   "talaria_template.yaml",
	//	writeToKafka: false,
	//})
}
