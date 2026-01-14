// SPDX-FileCopyrightText: 2025 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

//go:build integration

package main

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
)

// authTestCase defines a single authentication scenario
type authTestCase struct {
	name           string
	username       string
	password       string
	description    string
	expectedStatus int
}

// TestHelloWorld is a basic integration test that verifies the test fixture setup
// and makes a simple API call to get the list of connected devices.
func TestHelloWorld(t *testing.T) {
	// Set up the complete integration test environment
	fixture := setupIntegrationTest(t, "talaria_template.yaml")

	// Make a simple API call with valid credentials
	body, statusCode, err := fixture.GetDevices("user", "pass")
	require.NoError(t, err, "Failed to get devices")

	// Log the results
	t.Logf("***************************")
	t.Logf("Status: %d", statusCode)
	t.Logf("Response: %s", body)
	t.Logf("***************************")

	// Basic assertion - should get a successful response
	require.Equal(t, http.StatusOK, statusCode, "Expected 200 OK response")
}

// TestGetDevices_Auth tests authentication behavior for GET /api/v2/devices
func TestGetDevices_Auth(t *testing.T) {
	fixture := setupIntegrationTest(t, "talaria_template.yaml")

	authScenarios := []authTestCase{
		{
			name:           "valid_credentials",
			username:       "user",
			password:       "pass",
			description:    "Valid credentials should succeed",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "invalid_credentials",
			username:       "wrong",
			password:       "wrong",
			description:    "Invalid credentials should fail",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "no_credentials",
			username:       "",
			password:       "",
			description:    "No credentials should fail",
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, scenario := range authScenarios {
		t.Run(scenario.name, func(t *testing.T) {
			body, statusCode, err := fixture.GetDevices(scenario.username, scenario.password)
			require.NoError(t, err)

			t.Logf("%s - Status: %d, Body: %s", scenario.description, statusCode, body)
			require.Equal(t, scenario.expectedStatus, statusCode, scenario.description)
		})
	}
}

// TestGetDeviceStat_Auth tests authentication behavior for GET /api/v2/device/:deviceID/stat
func TestGetDeviceStat_Auth(t *testing.T) {
	fixture := setupIntegrationTest(t, "talaria_template.yaml")

	authScenarios := []authTestCase{
		{
			name:           "valid_credentials",
			username:       "user",
			password:       "pass",
			description:    "Valid credentials should succeed",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "invalid_credentials",
			username:       "wrong",
			password:       "wrong",
			description:    "Invalid credentials should fail",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "no_credentials",
			username:       "",
			password:       "",
			description:    "No credentials should fail",
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, scenario := range authScenarios {
		t.Run(scenario.name, func(t *testing.T) {
			body, statusCode, err := fixture.GET("/api/v2/device/mac:4ca161000109/stat", scenario.username, scenario.password)
			require.NoError(t, err)

			t.Logf("%s - Status: %d, Body: %s", scenario.description, statusCode, body)
			require.Equal(t, scenario.expectedStatus, statusCode, scenario.description)
		})
	}
}

// TestPostDeviceSend_Auth tests authentication behavior for POST /api/v2/device/send
func TestPostDeviceSend_Auth(t *testing.T) {
	fixture := setupIntegrationTest(t, "talaria_template.yaml")

	authScenarios := []authTestCase{
		{
			name:           "valid_credentials",
			username:       "user",
			password:       "pass",
			description:    "Valid credentials should succeed",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "invalid_credentials",
			username:       "wrong",
			password:       "wrong",
			description:    "Invalid credentials should fail",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "no_credentials",
			username:       "",
			password:       "",
			description:    "No credentials should fail",
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, scenario := range authScenarios {
		t.Run(scenario.name, func(t *testing.T) {
			payload := `{"device_id":"mac:4ca161000109","message":"test"}`
			body, statusCode, err := fixture.POST("/api/v2/device/send", strings.NewReader(payload), "application/json", scenario.username, scenario.password)
			require.NoError(t, err)

			t.Logf("%s - Status: %d, Body: %s", scenario.description, statusCode, body)
			require.Equal(t, scenario.expectedStatus, statusCode, scenario.description)
		})
	}
}

// TestCustomAPICall demonstrates using the fixture for custom API calls with different verbs.
func TestCustomAPICall(t *testing.T) {
	fixture := setupIntegrationTest(t, "talaria_template.yaml")

	t.Run("GET_with_bearer_token", func(t *testing.T) {
		// Example: Test with Bearer token instead of Basic auth
		token, err := fixture.GetJWTFromThemis()
		require.NoError(t, err)

		req, err := fixture.NewRequest("GET", "/api/v2/devices", nil)
		require.NoError(t, err)

		fixture.WithBearerToken(req, token)

		resp, err := fixture.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		t.Logf("Bearer token auth status: %d", resp.StatusCode)
		require.Equal(t, http.StatusOK, resp.StatusCode, "Valid JWT should be accepted")
	})

	t.Run("POST_with_payload", func(t *testing.T) {
		// Example: POST request with JSON payload
		jsonPayload := []byte(`{"test": "data"}`)
		body, statusCode, err := fixture.POST(
			"/api/v2/some-endpoint",
			bytes.NewReader(jsonPayload),
			"application/json",
			"user",
			"pass",
		)

		require.NoError(t, err)
		t.Logf("POST request status: %d, body: %s", statusCode, body)
	})

	t.Run("custom_headers", func(t *testing.T) {
		// Example: Request with custom headers
		req, err := fixture.NewRequest("GET", "/api/v2/devices", nil)
		require.NoError(t, err)

		fixture.WithBasicAuth(req, "user", "pass")
		req.Header.Set("X-Custom-Header", "test-value")

		body, statusCode, err := fixture.DoAndReadBody(req)
		require.NoError(t, err)

		t.Logf("Custom header request - Status: %d, Body: %s", statusCode, body)
		require.Equal(t, http.StatusOK, statusCode)
	})
}

// TestDeviceConnect_Auth tests authentication behavior for the WebSocket device connect endpoint
// at /api/v2/device. This endpoint requires special headers for WebSocket upgrade.
func TestDeviceConnect_Auth(t *testing.T) {
	fixture := setupIntegrationTest(t, "talaria_template.yaml")

	authScenarios := []authTestCase{
		{
			name:           "valid_jwt_token",
			username:       "",
			password:       "",
			description:    "Valid JWT token should allow connection",
			expectedStatus: http.StatusSwitchingProtocols, // 101 for WebSocket upgrade
		},
		{
			name:           "invalid_jwt_token",
			username:       "",
			password:       "",
			description:    "Invalid JWT token should fail",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "no_auth",
			username:       "",
			password:       "",
			description:    "No auth credentials (depends on failOpen config)",
			expectedStatus: http.StatusSwitchingProtocols, // May succeed if failOpen=true
		},
	}

	for _, scenario := range authScenarios {
		t.Run(scenario.name, func(t *testing.T) {
			// Build WebSocket URL
			wsURL := strings.Replace(fixture.TalariaURL, "http://", "ws://", 1) + "/api/v2/device"

			// Prepare headers
			headers := http.Header{}
			headers.Set("X-Webpa-Device-Name", "mac:aabbccddeeff")

			// Add convey header with device metadata
			conveyData := map[string]interface{}{
				"fw-name":                  "test-firmware-1.0",
				"hw-model":                 "test-model",
				"hw-manufacturer":          "test-manufacturer",
				"hw-serial-number":         "TEST123456",
				"hw-last-reboot-reason":    "power-cycle",
				"webpa-protocol":           "websocket",
				"boot-time":                "1234567890",
				"webpa-interface-used":     "eth0",
				"boot-time-retry-wait":     "30",
				"hw-last-reconnect-reason": "first-connect",
			}
			conveyJSON, err := json.Marshal(conveyData)
			require.NoError(t, err)
			headers.Set("X-Webpa-Convey", base64.StdEncoding.EncodeToString(conveyJSON))

			// Add authentication based on scenario
			switch scenario.name {
			case "valid_jwt_token":
				token, err := fixture.GetJWTFromThemis()
				t.Logf("Obtained JWT token: %s", token)
				require.NoError(t, err)
				headers.Set("Authorization", "Bearer "+token)

			case "invalid_jwt_token":
				headers.Set("Authorization", "Bearer invalid-token-xyz")

			case "no_auth":
				// No Authorization header
			}

			// Attempt WebSocket connection
			dialer := websocket.Dialer{}
			conn, resp, err := dialer.Dial(wsURL, headers)

			if conn != nil {
				defer conn.Close()
			}

			// Check status code and read response body for error details
			var statusCode int
			var responseBody string
			if resp != nil {
				statusCode = resp.StatusCode
				t.Logf("Response Headers: %v", resp.Header)

				// Read response body to see error message
				bodyBytes, readErr := io.ReadAll(resp.Body)
				resp.Body.Close()
				if readErr == nil {
					responseBody = string(bodyBytes)
					t.Logf("Response Body: %s", responseBody)
				}
			} else if err != nil {
				// If dial failed without a response, it's likely a connection error
				statusCode = 0
			}

			t.Logf("%s - Status: %d, Error: %v", scenario.description, statusCode, err)

			// For now, just log the result. We'll refine expected status codes
			// once we know the actual auth requirements for each scenario
			if statusCode > 0 {
				t.Logf("Got status code: %d (expected: %d)", statusCode, scenario.expectedStatus)
			}
		})
	}
}

// Example: Add new endpoints to TestEndpoints_WithAuth by uncommenting and filling in:
/*
{
	name: "GET /api/v2/device/stat/:deviceID/stat",
	makeRequest: func(f *TalariaTestFixture, u, p string) (string, int, error) {
		return f.GET("/api/v2/device/stat/mac:4ca161000109/stat", u, p)
	},
	validStatus:   http.StatusOK,
	invalidStatus: http.StatusUnauthorized,
	noAuthStatus:  http.StatusUnauthorized,
},
{
	name: "POST /api/v2/device/send",
	makeRequest: func(f *TalariaTestFixture, u, p string) (string, int, error) {
		payload := []byte(`{"command": "test"}`)
		return f.POST("/api/v2/device/send", bytes.NewReader(payload), "application/json", u, p)
	},
	validStatus:   http.StatusAccepted,
	invalidStatus: http.StatusUnauthorized,
	noAuthStatus:  http.StatusUnauthorized,
},

		require.Equal(t, http.StatusOK, statusCode)
	})
}
*/
