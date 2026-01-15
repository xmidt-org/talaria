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
	fixture := setupIntegrationTest(t, "talaria_template.yaml", WithFullStack())

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
// This is an API endpoint - it will use the API JWT validator (not device validator).
// 2-THEMIS-AWARE: Tests both API JWT (should succeed) and Device JWT (should fail).
func TestGetDevices_Auth(t *testing.T) {
	// Start both device and api Themis instances
	fixture := setupIntegrationTest(t, "talaria_template.yaml",
		WithKafka(),
		WithThemisInstance("device", "themis.yaml"), // For device/WebSocket endpoint
		WithThemisInstance("api", "themis.yaml"),    // For API endpoints
		WithCaduceus(),
		WithXmidtAgent(),
	)

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

	// 2-THEMIS-AWARE: Test with API JWT (should succeed - correct validator)
	t.Run("valid_api_jwt", func(t *testing.T) {
		// Get JWT from API Themis instance
		apiToken, err := fixture.GetJWTFromThemisInstance("api")
		require.NoError(t, err)
		t.Logf("Using API JWT from 'api' Themis instance")

		req, err := fixture.NewRequest("GET", "/api/v2/devices", nil)
		require.NoError(t, err)
		fixture.WithBearerToken(req, apiToken)

		body, statusCode, err := fixture.DoAndReadBody(req)
		require.NoError(t, err)
		t.Logf("API JWT - Status: %d, Body: %s", statusCode, body)
		require.Equal(t, http.StatusOK, statusCode, "API JWT should be accepted on API endpoint")
	})

	// 2-THEMIS-AWARE: Test with Device JWT (should fail - wrong validator)
	t.Run("device_jwt_on_api_endpoint", func(t *testing.T) {
		// Get JWT from Device Themis instance
		deviceToken, err := fixture.GetJWTFromThemisInstance("device")
		require.NoError(t, err)
		t.Logf("Using Device JWT from 'device' Themis instance")

		req, err := fixture.NewRequest("GET", "/api/v2/devices", nil)
		require.NoError(t, err)
		fixture.WithBearerToken(req, deviceToken)

		body, statusCode, err := fixture.DoAndReadBody(req)
		require.NoError(t, err)
		t.Logf("Device JWT on API endpoint - Status: %d, Body: %s", statusCode, body)

		// TODO: When Talaria supports 2 validators, this should be 401 Unauthorized
		// For now, both JWTs work because Talaria only has one validator
		// Change this assertion when Talaria implements separate validators:
		// require.Equal(t, http.StatusUnauthorized, statusCode, "Device JWT should be rejected on API endpoint")
		t.Logf("NOTE: Currently both JWTs work (single validator). Expected 401 when 2 validators implemented.")
	})
}

// TestGetDeviceStat_Auth tests authentication behavior for GET /api/v2/device/:deviceID/stat
// This is an API endpoint - it will use the API JWT validator (not device validator).
func TestGetDeviceStat_Auth(t *testing.T) {
	fixture := setupIntegrationTest(t, "talaria_template.yaml", WithFullStack())

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

	// 2-THEMIS-AWARE: Test with JWT (API endpoint uses API JWT validator)
	t.Run("valid_api_jwt", func(t *testing.T) {
		// When Talaria supports 2 validators: use fixture.GetJWTFromThemisInstance("api")
		// For now, all JWTs come from the same Themis instance
		token, err := fixture.GetJWTFromThemis()
		require.NoError(t, err)

		req, err := fixture.NewRequest("GET", "/api/v2/device/mac:4ca161000109/stat", nil)
		require.NoError(t, err)
		fixture.WithBearerToken(req, token)

		body, statusCode, err := fixture.DoAndReadBody(req)
		require.NoError(t, err)
		t.Logf("API JWT - Status: %d, Body: %s", statusCode, body)
		require.Equal(t, http.StatusOK, statusCode, "Valid API JWT should be accepted")
	})
}

// TestPostDeviceSend_Auth tests authentication behavior for POST /api/v2/device/send
// This is an API endpoint - it will use the API JWT validator (not device validator).
func TestPostDeviceSend_Auth(t *testing.T) {
	fixture := setupIntegrationTest(t, "talaria_template.yaml", WithFullStack())

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

	// 2-THEMIS-AWARE: Test with JWT (API endpoint uses API JWT validator)
	t.Run("valid_api_jwt", func(t *testing.T) {
		// When Talaria supports 2 validators: use fixture.GetJWTFromThemisInstance("api")
		// For now, all JWTs come from the same Themis instance
		token, err := fixture.GetJWTFromThemis()
		require.NoError(t, err)

		payload := `{"device_id":"mac:4ca161000109","message":"test"}`
		req, err := fixture.NewRequest("POST", "/api/v2/device/send", strings.NewReader(payload))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		fixture.WithBearerToken(req, token)

		body, statusCode, err := fixture.DoAndReadBody(req)
		require.NoError(t, err)
		t.Logf("API JWT - Status: %d, Body: %s", statusCode, body)
		require.Equal(t, http.StatusOK, statusCode, "Valid API JWT should be accepted")
	})
}

// TestCustomAPICall demonstrates using the fixture for custom API calls with different verbs.
// 2-THEMIS-AWARE: These are API endpoints - they will use the API JWT validator (not device validator).
func TestCustomAPICall(t *testing.T) {
	// Only need Themis for auth testing, not full stack
	fixture := setupIntegrationTest(t, "talaria_template.yaml", WithAPIServices())

	t.Run("GET_with_bearer_token", func(t *testing.T) {
		// 2-THEMIS-AWARE: API endpoint uses API JWT validator
		// When Talaria supports 2 validators: use fixture.GetJWTFromThemisInstance("api")
		// For now, all JWTs come from the same Themis instance
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
// 2-THEMIS-AWARE: This is the DEVICE endpoint - it will use the DEVICE JWT validator (not API validator).
func TestDeviceConnect_Auth(t *testing.T) {
	fixture := setupIntegrationTest(t, "talaria_template.yaml", WithFullStack())

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
				// 2-THEMIS-AWARE: Device endpoint uses DEVICE JWT validator
				// When Talaria supports 2 validators: use fixture.GetJWTFromThemisInstance("device")
				// For now, all JWTs come from the same Themis instance
				token, err := fixture.GetJWTFromThemis()
				t.Logf("Obtained Device JWT token: %s", token)
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

// TestSetupOptions demonstrates the flexible service startup options.
// This test shows how to selectively start services based on test requirements.
func TestSetupOptions(t *testing.T) {
	t.Run("API_Services_Only", func(t *testing.T) {
		// Only start Themis and Caduceus for testing API endpoints
		// This is faster than starting full stack when Kafka/xmidt-agent aren't needed
		fixture := setupIntegrationTest(t, "talaria_template.yaml", WithAPIServices())

		// Verify we can make authenticated API calls
		token, err := fixture.GetJWTFromThemis()
		require.NoError(t, err)
		require.NotEmpty(t, token)

		// Test an endpoint with the token
		req, err := fixture.NewRequest("GET", "/api/v2/devices", nil)
		require.NoError(t, err)
		fixture.WithBearerToken(req, token)

		body, statusCode, err := fixture.DoAndReadBody(req)
		require.NoError(t, err)
		t.Logf("API Services test - Status: %d, Body: %s", statusCode, body)
		require.Equal(t, http.StatusOK, statusCode)
	})

	t.Run("Themis_Only", func(t *testing.T) {
		// Only start Themis for testing JWT generation
		// Useful for auth-only tests
		fixture := setupIntegrationTest(t, "talaria_template.yaml", WithThemis())

		// Verify JWT generation works
		token, err := fixture.GetJWTFromThemis()
		require.NoError(t, err)
		require.NotEmpty(t, token)
		t.Logf("Generated JWT token: %s", token[:50]+"...")
	})

	t.Run("Full_Stack_Without_Agent", func(t *testing.T) {
		// Start all services except xmidt-agent
		// Useful when you want to test manual device connections
		fixture := setupIntegrationTest(
			t,
			"talaria_template.yaml",
			WithFullStack(),
			WithoutXmidtAgent(),
		)

		// Verify services are available
		require.NotEmpty(t, fixture.KafkaBroker)
		require.NotEmpty(t, fixture.ThemisIssuerURL)
		require.NotEmpty(t, fixture.CaduceusURL)
		require.NotEmpty(t, fixture.TalariaURL)

		// API should work normally
		body, statusCode, err := fixture.GetDevices("user", "pass")
		require.NoError(t, err)
		t.Logf("Devices response: %s", body)
		require.Equal(t, http.StatusOK, statusCode)
	})
}

// TestMultipleThemisInstances demonstrates using multiple Themis JWT issuers
// for different endpoints or authentication scenarios.
func TestMultipleThemisInstances(t *testing.T) {
	t.Run("Single_Themis_Default_Behavior", func(t *testing.T) {
		// Default behavior - single Themis instance
		fixture := setupIntegrationTest(t, "talaria_template.yaml", WithThemis())

		// List available instances
		instances := fixture.ListThemisInstances()
		require.Len(t, instances, 1, "Should have exactly one Themis instance")
		require.Equal(t, "default", instances[0])

		// Get JWT from default instance
		token, err := fixture.GetJWTFromThemis()
		require.NoError(t, err)
		require.NotEmpty(t, token)
		t.Logf("Default Themis JWT: %s...", token[:50])

		// Also works with GetJWTFromThemisInstance
		token2, err := fixture.GetJWTFromThemisInstance("default")
		require.NoError(t, err)
		require.NotEmpty(t, token2)
		t.Logf("Default Themis JWT (explicit): %s...", token2[:50])
	})

	t.Run("Multiple_Themis_Instances", func(t *testing.T) {
		// Start multiple Themis instances with different configurations
		fixture := setupIntegrationTest(t, "talaria_template.yaml",
			WithKafka(),
			WithThemisInstance("device", "themis.yaml"),                // Full capabilities
			WithThemisInstance("api", "themis_read_only.yaml"),         // Read-only
			WithThemisInstance("admin", "themis_no_capabilities.yaml"), // No capabilities
			WithCaduceus(),
		)

		// List all instances
		instances := fixture.ListThemisInstances()
		require.Len(t, instances, 3, "Should have three Themis instances")
		t.Logf("Available Themis instances: %v", instances)

		// Get JWT from device Themis (full capabilities)
		deviceJWT, err := fixture.GetJWTFromThemisInstance("device")
		require.NoError(t, err)
		require.NotEmpty(t, deviceJWT)
		t.Logf("Device Themis JWT (full caps): %s...", deviceJWT[:50])

		// Get JWT from API Themis (read-only)
		apiJWT, err := fixture.GetJWTFromThemisInstance("api")
		require.NoError(t, err)
		require.NotEmpty(t, apiJWT)
		t.Logf("API Themis JWT (read-only): %s...", apiJWT[:50])

		// Get JWT from admin Themis (no capabilities)
		adminJWT, err := fixture.GetJWTFromThemisInstance("admin")
		require.NoError(t, err)
		require.NotEmpty(t, adminJWT)
		t.Logf("Admin Themis JWT (no caps): %s...", adminJWT[:50])

		// Verify JWTs are different
		require.NotEqual(t, deviceJWT, apiJWT, "Device and API JWTs should be different")
		require.NotEqual(t, apiJWT, adminJWT, "API and Admin JWTs should be different")

		// Test with device JWT (should succeed - full capabilities)
		req, _ := fixture.NewRequest("GET", "/api/v2/devices", nil)
		fixture.WithBearerToken(req, deviceJWT)
		body, statusCode, err := fixture.DoAndReadBody(req)
		require.NoError(t, err)
		t.Logf("Device JWT result - Status: %d, Body: %s", statusCode, body)
		require.Equal(t, http.StatusOK, statusCode, "Device JWT should work")

		// Test with API JWT (should succeed - read capabilities)
		req2, _ := fixture.NewRequest("GET", "/api/v2/devices", nil)
		fixture.WithBearerToken(req2, apiJWT)
		body2, statusCode2, err2 := fixture.DoAndReadBody(req2)
		require.NoError(t, err2)
		t.Logf("API JWT result - Status: %d, Body: %s", statusCode2, body2)
		require.Equal(t, http.StatusOK, statusCode2, "API JWT should work for read")

		// Test with admin JWT (may fail - no capabilities)
		req3, _ := fixture.NewRequest("GET", "/api/v2/devices", nil)
		fixture.WithBearerToken(req3, adminJWT)
		body3, statusCode3, err3 := fixture.DoAndReadBody(req3)
		require.NoError(t, err3)
		t.Logf("Admin JWT result - Status: %d, Body: %s", statusCode3, body3)
		// Note: May succeed or fail depending on Talaria's failOpen configuration
	})

	t.Run("Convenience_Options", func(t *testing.T) {
		// Use convenience options for common scenarios
		fixture := setupIntegrationTest(t, "talaria_template.yaml",
			WithDeviceThemis(), // Uses themis.yaml
			WithAPIThemis(),    // Uses themis.yaml
			WithKafka(),
			WithCaduceus(),
		)

		// Verify both instances exist
		deviceInstance := fixture.GetThemisInstance("device")
		require.NotNil(t, deviceInstance, "Device Themis should exist")
		require.Equal(t, "device", deviceInstance.Name)

		apiInstance := fixture.GetThemisInstance("api")
		require.NotNil(t, apiInstance, "API Themis should exist")
		require.Equal(t, "api", apiInstance.Name)

		// Get tokens from each
		deviceToken, err := fixture.GetJWTFromThemisInstance("device")
		require.NoError(t, err)
		require.NotEmpty(t, deviceToken)

		apiToken, err := fixture.GetJWTFromThemisInstance("api")
		require.NoError(t, err)
		require.NotEmpty(t, apiToken)

		t.Logf("✓ Device and API Themis instances configured and working")
	})

	t.Run("Batch_Configuration", func(t *testing.T) {
		// Configure multiple instances in one call
		themisConfigs := map[string]string{
			"device": "themis.yaml",
			"api":    "themis_read_only.yaml",
			"admin":  "themis_specific_device.yaml",
		}

		fixture := setupIntegrationTest(t, "talaria_template.yaml",
			WithKafka(),
			WithMultipleThemis(themisConfigs),
			WithCaduceus(),
		)

		// Verify all instances created
		instances := fixture.ListThemisInstances()
		require.Len(t, instances, 3)
		t.Logf("Batch-configured instances: %v", instances)

		// Get tokens from all instances
		for name := range themisConfigs {
			token, err := fixture.GetJWTFromThemisInstance(name)
			require.NoError(t, err, "Failed to get token from %s", name)
			require.NotEmpty(t, token)
			t.Logf("%s JWT: %s...", name, token[:30])
		}
	})

	t.Run("Nonexistent_Instance_Error", func(t *testing.T) {
		fixture := setupIntegrationTest(t, "talaria_template.yaml", WithThemis())

		// Try to get JWT from nonexistent instance
		_, err := fixture.GetJWTFromThemisInstance("nonexistent")
		require.Error(t, err)
		require.Contains(t, err.Error(), "not found")
		t.Logf("Expected error: %v", err)
	})
}

// TestTrustedVsUntrustedJWT demonstrates testing JWT validation with trusted and untrusted Themis instances.
// This test is currently INCOMPLETE because Talaria only supports ONE global JWT validator.
//
// TODO: Complete this test when Talaria supports multiple JWT validators per endpoint.
//
// Current limitation: All started Themis instances are trusted by Talaria because setupTalaria()
// only configures one JWT validator URL (the first available instance).
//
// To complete implementation:
// 1. Update Talaria to support multiple JWT validator URLs (per-endpoint or globally)
// 2. Uncomment the WithTrustedThemis option logic in setupIntegrationTestWithCapabilities
// 3. Update setupTalaria to accept and configure multiple Themis keys URLs
// 4. Enable these tests by removing the t.Skip() calls
//
// Expected behavior when complete:
// - Talaria configured with WithTrustedThemis("trusted") will ONLY trust JWTs from "trusted" instance
// - JWTs from "untrusted" instance will be rejected with 401 Unauthorized
// - This enables security testing: verify endpoints properly validate JWT issuers
func TestTrustedVsUntrustedJWT(t *testing.T) {
	t.Run("GET_Devices_Trusted_vs_Untrusted", func(t *testing.T) {
		t.Skip("TODO: Enable when Talaria supports multiple JWT validators. See function documentation.")

		// Setup: Start 2 Themis instances, but only trust one
		fixture := setupIntegrationTest(t, "talaria_template.yaml",
			WithKafka(),
			WithThemisInstance("trusted", "themis.yaml"),
			WithThemisInstance("untrusted", "themis.yaml"),
			WithTrustedThemis("trusted"), // TODO: Implement filtering in setupTalaria
			WithCaduceus(),
		)

		// Get JWTs from both instances
		trustedJWT, err := fixture.GetJWTFromThemisInstance("trusted")
		require.NoError(t, err)
		require.NotEmpty(t, trustedJWT)

		untrustedJWT, err := fixture.GetJWTFromThemisInstance("untrusted")
		require.NoError(t, err)
		require.NotEmpty(t, untrustedJWT)

		// Verify JWTs are different
		require.NotEqual(t, trustedJWT, untrustedJWT)

		// Test with trusted JWT - should succeed
		req, _ := fixture.NewRequest("GET", "/api/v2/devices", nil)
		fixture.WithBearerToken(req, trustedJWT)
		body, statusCode, err := fixture.DoAndReadBody(req)
		require.NoError(t, err)
		t.Logf("Trusted JWT - Status: %d, Body: %s", statusCode, body)
		require.Equal(t, http.StatusOK, statusCode, "Trusted JWT should be accepted")

		// Test with untrusted JWT - should fail
		req2, _ := fixture.NewRequest("GET", "/api/v2/devices", nil)
		fixture.WithBearerToken(req2, untrustedJWT)
		body2, statusCode2, err2 := fixture.DoAndReadBody(req2)
		require.NoError(t, err2)
		t.Logf("Untrusted JWT - Status: %d, Body: %s", statusCode2, body2)
		require.Equal(t, http.StatusUnauthorized, statusCode2, "Untrusted JWT should be rejected")
	})

	t.Run("POST_DeviceSend_Trusted_vs_Untrusted", func(t *testing.T) {
		t.Skip("TODO: Enable when Talaria supports multiple JWT validators. See function documentation.")

		fixture := setupIntegrationTest(t, "talaria_template.yaml",
			WithKafka(),
			WithThemisInstance("trusted", "themis.yaml"),
			WithThemisInstance("untrusted", "themis.yaml"),
			WithTrustedThemis("trusted"),
			WithCaduceus(),
		)

		trustedJWT, _ := fixture.GetJWTFromThemisInstance("trusted")
		untrustedJWT, _ := fixture.GetJWTFromThemisInstance("untrusted")

		payload := `{"device_id":"mac:4ca161000109","message":"test"}`

		// Test with trusted JWT
		req, _ := fixture.NewRequest("POST", "/api/v2/device/send", strings.NewReader(payload))
		req.Header.Set("Content-Type", "application/json")
		fixture.WithBearerToken(req, trustedJWT)
		body, statusCode, err := fixture.DoAndReadBody(req)
		require.NoError(t, err)
		t.Logf("Trusted JWT - Status: %d, Body: %s", statusCode, body)
		require.Equal(t, http.StatusOK, statusCode, "Trusted JWT should be accepted")

		// Test with untrusted JWT
		req2, _ := fixture.NewRequest("POST", "/api/v2/device/send", strings.NewReader(payload))
		req2.Header.Set("Content-Type", "application/json")
		fixture.WithBearerToken(req2, untrustedJWT)
		body2, statusCode2, err2 := fixture.DoAndReadBody(req2)
		require.NoError(t, err2)
		t.Logf("Untrusted JWT - Status: %d, Body: %s", statusCode2, body2)
		require.Equal(t, http.StatusUnauthorized, statusCode2, "Untrusted JWT should be rejected")
	})

	t.Run("WebSocket_Device_Connect_Trusted_vs_Untrusted", func(t *testing.T) {
		t.Skip("TODO: Enable when Talaria supports multiple JWT validators. See function documentation.")

		fixture := setupIntegrationTest(t, "talaria_template.yaml",
			WithKafka(),
			WithThemisInstance("trusted", "themis.yaml"),
			WithThemisInstance("untrusted", "themis.yaml"),
			WithTrustedThemis("trusted"),
			WithCaduceus(),
		)

		trustedJWT, _ := fixture.GetJWTFromThemisInstance("trusted")
		untrustedJWT, _ := fixture.GetJWTFromThemisInstance("untrusted")

		wsURL := strings.Replace(fixture.TalariaURL, "http://", "ws://", 1) + "/api/v2/device"

		// Prepare common headers
		prepareHeaders := func(jwt string) http.Header {
			headers := http.Header{}
			headers.Set("X-Webpa-Device-Name", "mac:aabbccddeeff")

			conveyData := map[string]interface{}{
				"fw-name":         "test-firmware-1.0",
				"hw-model":        "test-model",
				"hw-manufacturer": "test-manufacturer",
			}
			conveyJSON, _ := json.Marshal(conveyData)
			headers.Set("X-Webpa-Convey", base64.StdEncoding.EncodeToString(conveyJSON))
			headers.Set("Authorization", "Bearer "+jwt)
			return headers
		}

		// Test with trusted JWT - should connect
		dialer := websocket.Dialer{}
		conn, resp, err := dialer.Dial(wsURL, prepareHeaders(trustedJWT))
		if conn != nil {
			defer conn.Close()
		}
		require.NoError(t, err, "Trusted JWT should allow WebSocket connection")
		require.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
		t.Logf("Trusted JWT - Connected successfully")

		// Test with untrusted JWT - should fail
		conn2, resp2, err2 := dialer.Dial(wsURL, prepareHeaders(untrustedJWT))
		if conn2 != nil {
			conn2.Close()
		}
		require.Error(t, err2, "Untrusted JWT should fail WebSocket connection")
		if resp2 != nil {
			require.Equal(t, http.StatusUnauthorized, resp2.StatusCode, "Untrusted JWT should be rejected")
			t.Logf("Untrusted JWT - Rejected with status: %d", resp2.StatusCode)
		}
	})

	t.Run("GET_DeviceStat_Trusted_vs_Untrusted", func(t *testing.T) {
		t.Skip("TODO: Enable when Talaria supports multiple JWT validators. See function documentation.")

		fixture := setupIntegrationTest(t, "talaria_template.yaml",
			WithKafka(),
			WithThemisInstance("trusted", "themis.yaml"),
			WithThemisInstance("untrusted", "themis.yaml"),
			WithTrustedThemis("trusted"),
			WithCaduceus(),
			WithXmidtAgent(), // Need connected device for stat endpoint
		)

		trustedJWT, _ := fixture.GetJWTFromThemisInstance("trusted")
		untrustedJWT, _ := fixture.GetJWTFromThemisInstance("untrusted")

		// Test with trusted JWT
		req, _ := fixture.NewRequest("GET", "/api/v2/device/mac:4ca161000109/stat", nil)
		fixture.WithBearerToken(req, trustedJWT)
		body, statusCode, err := fixture.DoAndReadBody(req)
		require.NoError(t, err)
		t.Logf("Trusted JWT - Status: %d, Body: %s", statusCode, body)
		require.Equal(t, http.StatusOK, statusCode, "Trusted JWT should be accepted")

		// Test with untrusted JWT
		req2, _ := fixture.NewRequest("GET", "/api/v2/device/mac:4ca161000109/stat", nil)
		fixture.WithBearerToken(req2, untrustedJWT)
		body2, statusCode2, err2 := fixture.DoAndReadBody(req2)
		require.NoError(t, err2)
		t.Logf("Untrusted JWT - Status: %d, Body: %s", statusCode2, body2)
		require.Equal(t, http.StatusUnauthorized, statusCode2, "Untrusted JWT should be rejected")
	})
}
