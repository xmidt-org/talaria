// SPDX-FileCopyrightText: 2025 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

//go:build integration

package main

// TODO: When Talaria supports multiple JWT validators (separate for device vs API endpoints):
// 1. Add API_THEMIS_URL placeholder to talaria_template.yaml
// 2. Search and replace: grep -r 'WithThemisInstance("api".*DEVICE_THEMIS_URL' to update all api instances
// 3. Change DEVICE_THEMIS_URL -> API_THEMIS_URL for api instances

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

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

// TestGetDevices_Auth tests authentication behavior for GET /api/v2/devices
func TestGetDevices_Auth(t *testing.T) {
	// Start both device and api Themis instances
	// Note: xmidt-agent NOT needed - this endpoint works without connected devices
	fixture := setupIntegrationTest(t, "talaria_template.yaml",
		WithThemisInstance("device", "themis.yaml", "DEVICE_THEMIS_URL"),
		WithThemisInstance("api", "themis.yaml"),
		WithCaduceus(),
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
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "no_credentials",
			username:       "",
			password:       "",
			description:    "No credentials should fail",
			expectedStatus: http.StatusForbidden,
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

	// Test with API JWT
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
		// this currently fails because Talaria only supports one JWT validator globally (which is the device one)
		require.Equal(t, http.StatusOK, statusCode, "API JWT should be accepted on API endpoint")
	})

	// Test with Device JWT
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

		// this should be 403 because the devices should not accept device JWTs
		// it currently returns 200 because Talaria only supports one JWT validator globally, which is set to device
		require.Equal(t, http.StatusForbidden, statusCode, "Device JWT should not be accepted")
	})

	// Test with invalid JWT token
	t.Run("invalid_jwt_token", func(t *testing.T) {
		invalidToken := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.invalid.signature"

		req, err := fixture.NewRequest("GET", "/api/v2/devices", nil)
		require.NoError(t, err)
		fixture.WithBearerToken(req, invalidToken)

		body, statusCode, err := fixture.DoAndReadBody(req)
		require.NoError(t, err)
		t.Logf("Invalid JWT - Status: %d, Body: %s", statusCode, body)
		require.Equal(t, http.StatusForbidden, statusCode, "Invalid JWT should be rejected")
	})

	// Test with malformed Bearer token
	t.Run("malformed_bearer_token", func(t *testing.T) {
		req, err := fixture.NewRequest("GET", "/api/v2/devices", nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer") // Missing token

		body, statusCode, err := fixture.DoAndReadBody(req)
		require.NoError(t, err)
		t.Logf("Malformed Bearer (no token) - Status: %d, Body: %s", statusCode, body)
		require.Equal(t, http.StatusBadRequest, statusCode, "Malformed Bearer header should be rejected")
	})

	// Test with empty Bearer token
	t.Run("empty_bearer_token", func(t *testing.T) {
		req, err := fixture.NewRequest("GET", "/api/v2/devices", nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer ")

		body, statusCode, err := fixture.DoAndReadBody(req)
		require.NoError(t, err)
		t.Logf("Empty Bearer token - Status: %d, Body: %s", statusCode, body)
		require.Equal(t, http.StatusBadRequest, statusCode, "Empty Bearer token should be rejected")
	})

	// omitted test: send both Basic Auth and Bearer token (test precedence).  Go returns first header from map with name.
}

// TestGetDeviceStat_Auth tests authentication behavior for GET /api/v2/device/:deviceID/stat
// This is an API endpoint - it will use the API JWT validator (not device validator).
func TestGetDeviceStat_Auth(t *testing.T) {
	// Start both device and api Themis instances
	fixture := setupIntegrationTest(t, "talaria_template.yaml",
		WithThemisInstance("device", "themis.yaml", "DEVICE_THEMIS_URL"),
		WithThemisInstance("api", "themis.yaml"),
		WithCaduceus(),
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
			body, statusCode, err := fixture.GET("/api/v2/device/mac:4ca161000109/stat", scenario.username, scenario.password)
			require.NoError(t, err)

			t.Logf("%s - Status: %d, Body: %s", scenario.description, statusCode, body)
			require.Equal(t, scenario.expectedStatus, statusCode, scenario.description)
		})
	}

	// Test with API JWT
	t.Run("valid_api_jwt", func(t *testing.T) {
		// Get JWT from API Themis instance
		apiToken, err := fixture.GetJWTFromThemisInstance("api")
		require.NoError(t, err)
		t.Logf("Using API JWT from 'api' Themis instance")

		req, err := fixture.NewRequest("GET", "/api/v2/device/mac:4ca161000109/stat", nil)
		require.NoError(t, err)
		fixture.WithBearerToken(req, apiToken)

		body, statusCode, err := fixture.DoAndReadBody(req)
		require.NoError(t, err)
		t.Logf("API JWT - Status: %d, Body: %s", statusCode, body)
		// this currently fails because Talaria only supports one JWT validator globally (which is the device one)
		require.Equal(t, http.StatusOK, statusCode, "API JWT should be accepted on API endpoint")
	})

	// Test with Device JWT
	t.Run("device_jwt_on_api_endpoint", func(t *testing.T) {
		// Get JWT from Device Themis instance
		deviceToken, err := fixture.GetJWTFromThemisInstance("device")
		require.NoError(t, err)
		t.Logf("Using Device JWT from 'device' Themis instance")

		req, err := fixture.NewRequest("GET", "/api/v2/device/mac:4ca161000109/stat", nil)
		require.NoError(t, err)
		fixture.WithBearerToken(req, deviceToken)

		body, statusCode, err := fixture.DoAndReadBody(req)
		require.NoError(t, err)
		t.Logf("Device JWT on API endpoint - Status: %d, Body: %s", statusCode, body)

		// this should be 403 because the API endpoints should not accept device JWTs
		// it currently returns 200 because Talaria only supports one JWT validator globally, which is set to device
		require.Equal(t, http.StatusForbidden, statusCode, "Device JWT should not be accepted")
	})

	// Test with invalid JWT token
	t.Run("invalid_jwt_token", func(t *testing.T) {
		invalidToken := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.invalid.signature"

		req, err := fixture.NewRequest("GET", "/api/v2/device/mac:4ca161000109/stat", nil)
		require.NoError(t, err)
		fixture.WithBearerToken(req, invalidToken)

		body, statusCode, err := fixture.DoAndReadBody(req)
		require.NoError(t, err)
		t.Logf("Invalid JWT - Status: %d, Body: %s", statusCode, body)
		require.Equal(t, http.StatusForbidden, statusCode, "Invalid JWT should be rejected")
	})

	// Test with malformed Bearer token
	t.Run("malformed_bearer_token", func(t *testing.T) {
		req, err := fixture.NewRequest("GET", "/api/v2/device/mac:4ca161000109/stat", nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer") // Missing token

		body, statusCode, err := fixture.DoAndReadBody(req)
		require.NoError(t, err)
		t.Logf("Malformed Bearer (no token) - Status: %d, Body: %s", statusCode, body)
		require.Equal(t, http.StatusBadRequest, statusCode, "Malformed Bearer header should be rejected")
	})

	// Test with empty Bearer token
	t.Run("empty_bearer_token", func(t *testing.T) {
		req, err := fixture.NewRequest("GET", "/api/v2/device/mac:4ca161000109/stat", nil)
		require.NoError(t, err)
		req.Header.Set("Authorization", "Bearer ")

		body, statusCode, err := fixture.DoAndReadBody(req)
		require.NoError(t, err)
		t.Logf("Empty Bearer token - Status: %d, Body: %s", statusCode, body)
		require.Equal(t, http.StatusBadRequest, statusCode, "Empty Bearer token should be rejected")
	})
}

// TestPostDeviceSend_Auth tests authentication behavior for POST /api/v2/device/send
// This is an API endpoint - it will use the API JWT validator (not device validator).
func TestPostDeviceSend_Auth(t *testing.T) {
	// Start both device and api Themis instances
	fixture := setupIntegrationTest(t, "talaria_template.yaml",
		WithThemisInstance("device", "themis.yaml", "DEVICE_THEMIS_URL"),
		WithThemisInstance("api", "themis.yaml"),
		WithCaduceus(),
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
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "no_credentials",
			username:       "",
			password:       "",
			description:    "No credentials should fail",
			expectedStatus: http.StatusForbidden,
		},
	}

	for _, scenario := range authScenarios {
		t.Run(scenario.name, func(t *testing.T) {
			// Use valid WRP (Web Router Protocol) message format
			payload := `{
				"msg_type":3,
				"content_type":"application/json",
				"source":"dns:test-sender",
				"dest":"mac:4ca161000109/config",
				"transaction_uuid":"test-transaction-uuid",
				"payload":"eyJjb21tYW5kIjoiR0VUIiwibmFtZXMiOlsiRGV2aWNlLkluZm8uUHJvcGVydHkxIl19",
				"partner_ids":["testpartner"]
			}`
			body, statusCode, err := fixture.POST("/api/v2/device/send", strings.NewReader(payload), "application/json", scenario.username, scenario.password)
			require.NoError(t, err)

			t.Logf("%s - Status: %d, Body: %s", scenario.description, statusCode, body)
			require.Equal(t, scenario.expectedStatus, statusCode, scenario.description)
		})
	}

	// Test with API JWT
	t.Run("valid_api_jwt", func(t *testing.T) {
		// Get JWT from API Themis instance
		apiToken, err := fixture.GetJWTFromThemisInstance("api")
		require.NoError(t, err)
		t.Logf("Using API JWT from 'api' Themis instance")

		// Use valid WRP message format
		payload := `{
			"msg_type":3,
			"content_type":"application/json",
			"source":"dns:test-sender",
			"dest":"mac:4ca161000109/config",
			"transaction_uuid":"test-transaction-uuid",
			"payload":"eyJjb21tYW5kIjoiR0VUIiwibmFtZXMiOlsiRGV2aWNlLkluZm8uUHJvcGVydHkxIl19",
			"partner_ids":["testpartner"]
		}`
		req, err := fixture.NewRequest("POST", "/api/v2/device/send", strings.NewReader(payload))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		fixture.WithBearerToken(req, apiToken)

		body, statusCode, err := fixture.DoAndReadBody(req)
		require.NoError(t, err)
		t.Logf("API JWT - Status: %d, Body: %s", statusCode, body)
		// this currently fails because Talaria only supports one JWT validator globally (which is the device one)
		require.Equal(t, http.StatusOK, statusCode, "API JWT should be accepted on API endpoint")
	})

	// Test with Device JWT
	t.Run("device_jwt_on_api_endpoint", func(t *testing.T) {
		// Get JWT from Device Themis instance
		deviceToken, err := fixture.GetJWTFromThemisInstance("device")
		require.NoError(t, err)
		t.Logf("Using Device JWT from 'device' Themis instance")

		// Use valid WRP message format
		payload := `{
			"msg_type":3,
			"content_type":"application/json",
			"source":"dns:test-sender",
			"dest":"mac:4ca161000109/config",
			"transaction_uuid":"test-transaction-uuid",
			"payload":"eyJjb21tYW5kIjoiR0VUIiwibmFtZXMiOlsiRGV2aWNlLkluZm8uUHJvcGVydHkxIl19",
			"partner_ids":["testpartner"]
		}`
		req, err := fixture.NewRequest("POST", "/api/v2/device/send", strings.NewReader(payload))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		fixture.WithBearerToken(req, deviceToken)

		body, statusCode, err := fixture.DoAndReadBody(req)
		require.NoError(t, err)
		t.Logf("Device JWT on API endpoint - Status: %d, Body: %s", statusCode, body)

		// this should be 403 because the API endpoints should not accept device JWTs
		// it currently returns 200 because Talaria only supports one JWT validator globally, which is set to device
		require.Equal(t, http.StatusForbidden, statusCode, "Device JWT should not be accepted")
	})

	// Test with invalid JWT token
	t.Run("invalid_jwt_token", func(t *testing.T) {
		invalidToken := "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.invalid.signature"

		payload := `{
			"msg_type":3,
			"content_type":"application/json",
			"source":"dns:test-sender",
			"dest":"mac:4ca161000109/config",
			"transaction_uuid":"test-transaction-uuid",
			"payload":"eyJjb21tYW5kIjoiR0VUIiwibmFtZXMiOlsiRGV2aWNlLkluZm8uUHJvcGVydHkxIl19",
			"partner_ids":["testpartner"]
		}`
		req, err := fixture.NewRequest("POST", "/api/v2/device/send", strings.NewReader(payload))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		fixture.WithBearerToken(req, invalidToken)

		body, statusCode, err := fixture.DoAndReadBody(req)
		require.NoError(t, err)
		t.Logf("Invalid JWT - Status: %d, Body: %s", statusCode, body)
		require.Equal(t, http.StatusForbidden, statusCode, "Invalid JWT should be rejected")
	})

	// Test with malformed Bearer token
	t.Run("malformed_bearer_token", func(t *testing.T) {
		payload := `{
			"msg_type":3,
			"content_type":"application/json",
			"source":"dns:test-sender",
			"dest":"mac:4ca161000109/config",
			"transaction_uuid":"test-transaction-uuid",
			"payload":"eyJjb21tYW5kIjoiR0VUIiwibmFtZXMiOlsiRGV2aWNlLkluZm8uUHJvcGVydHkxIl19",
			"partner_ids":["testpartner"]
		}`
		req, err := fixture.NewRequest("POST", "/api/v2/device/send", strings.NewReader(payload))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer") // Missing token

		body, statusCode, err := fixture.DoAndReadBody(req)
		require.NoError(t, err)
		t.Logf("Malformed Bearer (no token) - Status: %d, Body: %s", statusCode, body)
		require.Equal(t, http.StatusBadRequest, statusCode, "Malformed Bearer header should be rejected")
	})

	// Test with empty Bearer token
	t.Run("empty_bearer_token", func(t *testing.T) {
		payload := `{
			"msg_type":3,
			"content_type":"application/json",
			"source":"dns:test-sender",
			"dest":"mac:4ca161000109/config",
			"transaction_uuid":"test-transaction-uuid",
			"payload":"eyJjb21tYW5kIjoiR0VUIiwibmFtZXMiOlsiRGV2aWNlLkluZm8uUHJvcGVydHkxIl19",
			"partner_ids":["testpartner"]
		}`
		req, err := fixture.NewRequest("POST", "/api/v2/device/send", strings.NewReader(payload))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer ")

		body, statusCode, err := fixture.DoAndReadBody(req)
		require.NoError(t, err)
		t.Logf("Empty Bearer token - Status: %d, Body: %s", statusCode, body)
		require.Equal(t, http.StatusBadRequest, statusCode, "Empty Bearer token should be rejected")
	})
}

// TestDeviceConnect_Auth tests authentication behavior for the WebSocket device connect endpoint
// at /api/v2/device. This endpoint requires special headers for WebSocket upgrade.
// 2-THEMIS-AWARE: This is the DEVICE endpoint - it will use the DEVICE JWT validator (not API validator).
func TestDeviceConnect_Auth(t *testing.T) {
	fixture := setupIntegrationTest(t, "talaria_template.yaml",
		WithThemisInstance("device", "themis.yaml", "DEVICE_THEMIS_URL"),
		WithCaduceus(),
		WithXmidtAgent(),
	)

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

		// Setup: Start 2 Themis instances, trusted is mapped to config
		fixture := setupIntegrationTest(t, "talaria_template.yaml",
			WithKafka(),
			WithThemisInstance("trusted", "themis.yaml", "DEVICE_THEMIS_URL"), // Mapped to config
			WithThemisInstance("untrusted", "themis.yaml"),                    // Standalone (not in config)
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
			WithThemisInstance("trusted", "themis.yaml", "DEVICE_THEMIS_URL"),
			WithThemisInstance("untrusted", "themis.yaml"),
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
			WithThemisInstance("trusted", "themis.yaml", "DEVICE_THEMIS_URL"),
			WithThemisInstance("untrusted", "themis.yaml"),
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
			WithThemisInstance("trusted", "themis.yaml", "DEVICE_THEMIS_URL"),
			WithThemisInstance("untrusted", "themis.yaml"),
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

// TestExpiredJWT tests that Talaria properly rejects expired JWT tokens.
// Uses a Themis instance configured to issue JWTs with very short expiration (2 seconds).
func TestExpiredJWT(t *testing.T) {
	t.Run("GET_Devices_With_Expired_JWT", func(t *testing.T) {
		// Start Themis with short-lived JWTs (2 second expiration)
		fixture := setupIntegrationTest(t, "talaria_template.yaml",
			WithThemisInstance("api", "themis_short_expiration.yaml", "DEVICE_THEMIS_URL"),
			WithCaduceus(),
		)

		// Get a JWT token (valid for 2 seconds)
		token, err := fixture.GetJWTFromThemisInstance("api")
		require.NoError(t, err)
		require.NotEmpty(t, token)
		t.Logf("Obtained short-lived JWT (expires in 2s): %s...", token[:50])

		// TIMING FIX: Wait 1 second to ensure token's 'iat' (issued at) time has definitely passed
		t.Log("Waiting 1 second to ensure token iat time has passed...")
		time.Sleep(1 * time.Second)

		// Test with valid token - should succeed (token is still valid)
		req1, _ := fixture.NewRequest("GET", "/api/v2/devices", nil)
		fixture.WithBearerToken(req1, token)
		body1, statusCode1, err1 := fixture.DoAndReadBody(req1)
		require.NoError(t, err1)
		t.Logf("Fresh JWT - Status: %d, Body: %s", statusCode1, body1)
		require.Equal(t, http.StatusOK, statusCode1, "Fresh JWT should be accepted")

		// Wait for token to expire (was issued 1s ago, expires in 2s total = 1s remaining + buffer)
		// Adding extra buffer to account for clock skew and validation precision
		t.Log("Waiting 3 seconds for JWT to expire (2s + buffer)...")
		time.Sleep(3 * time.Second)

		// Test again - should fail (token is now expired)
		req2, _ := fixture.NewRequest("GET", "/api/v2/devices", nil)
		fixture.WithBearerToken(req2, token)
		body2, statusCode2, err2 := fixture.DoAndReadBody(req2)
		require.NoError(t, err2)
		t.Logf("Expired JWT - Status: %d, Body: %s", statusCode2, body2)
		require.Equal(t, http.StatusUnauthorized, statusCode2, "Expired JWT should be rejected")
	})

	t.Run("POST_DeviceSend_With_Expired_JWT", func(t *testing.T) {
		// Start Themis with short-lived JWTs (2 second expiration)
		fixture := setupIntegrationTest(t, "talaria_template.yaml",
			WithThemisInstance("api", "themis_short_expiration.yaml", "DEVICE_THEMIS_URL"),
			WithCaduceus(),
		)

		// Get a JWT token (valid for 2 seconds)
		token, err := fixture.GetJWTFromThemisInstance("api")
		require.NoError(t, err)
		t.Logf("Obtained short-lived JWT (expires in 2s)")

		// TIMING FIX: Wait 1 second to ensure token's 'iat' (issued at) time has definitely passed
		// This prevents "Token used before issued" errors due to clock skew or timing races
		t.Log("Waiting 1 second to ensure token iat time has passed...")
		time.Sleep(1 * time.Second)

		// Use valid WRP (Web Router Protocol) message format
		payload := `{
			"msg_type":3,
			"content_type":"application/json",
			"source":"dns:test-sender",
			"dest":"mac:4ca161000109/config",
			"transaction_uuid":"test-transaction-uuid",
			"payload":"eyJjb21tYW5kIjoiR0VUIiwibmFtZXMiOlsiRGV2aWNlLkluZm8uUHJvcGVydHkxIl09",
			"partner_ids":["foobar"]
		}`

		// Test with valid token (should succeed)
		req1, _ := fixture.NewRequest("POST", "/api/v2/device/send", strings.NewReader(payload))
		req1.Header.Set("Content-Type", "application/json")
		fixture.WithBearerToken(req1, token)
		body1, statusCode1, err1 := fixture.DoAndReadBody(req1)
		require.NoError(t, err1)
		t.Logf("Fresh JWT - Status: %d, Body: %s", statusCode1, body1)
		require.Equal(t, http.StatusOK, statusCode1, "Fresh JWT should be accepted")

		// Wait for token to expire (was issued 1s ago, expires in 2s total = 1s remaining + buffer)
		// Adding extra buffer to account for clock skew and validation precision
		t.Log("Waiting 3 seconds for JWT to expire (2s + buffer)...")
		time.Sleep(3 * time.Second)

		// Test again - should fail
		req2, _ := fixture.NewRequest("POST", "/api/v2/device/send", strings.NewReader(payload))
		req2.Header.Set("Content-Type", "application/json")
		fixture.WithBearerToken(req2, token)
		body2, statusCode2, err2 := fixture.DoAndReadBody(req2)
		require.NoError(t, err2)
		t.Logf("Expired JWT - Status: %d, Body: %s", statusCode2, body2)
		require.Equal(t, http.StatusUnauthorized, statusCode2, "Expired JWT should be rejected")
	})

	t.Run("WebSocket_Device_Connect_With_Expired_JWT", func(t *testing.T) {
		// Start Themis with short-lived JWTs (2 second expiration)
		fixture := setupIntegrationTest(t, "talaria_template.yaml",
			WithThemisInstance("device", "themis_short_expiration.yaml", "DEVICE_THEMIS_URL"),
			WithCaduceus(),
		)

		// Get a JWT token (valid for 2 seconds)
		token, err := fixture.GetJWTFromThemisInstance("device")
		require.NoError(t, err)
		t.Logf("Obtained short-lived JWT (expires in 2s)")

		// TIMING FIX: Wait 1 second to ensure token's 'iat' (issued at) time has definitely passed
		t.Log("Waiting 1 second to ensure token iat time has passed...")
		time.Sleep(1 * time.Second)

		wsURL := strings.Replace(fixture.TalariaURL, "http://", "ws://", 1) + "/api/v2/device"

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

		// Test immediately - should succeed
		dialer := websocket.Dialer{}
		conn1, resp1, err1 := dialer.Dial(wsURL, prepareHeaders(token))
		if conn1 != nil {
			defer conn1.Close()
		}
		require.NoError(t, err1, "Fresh JWT should allow WebSocket connection")
		require.Equal(t, http.StatusSwitchingProtocols, resp1.StatusCode)
		t.Logf("Fresh JWT - WebSocket connection succeeded")
		if conn1 != nil {
			conn1.Close() // Close before waiting
		}

		// Wait for token to expire (was issued 1s ago, expires in 2s total = 1s remaining + buffer)
		// Adding extra buffer to account for clock skew and validation precision
		t.Log("Waiting 3 seconds for JWT to expire (2s + buffer)...")
		time.Sleep(3 * time.Second)

		// Test again - should fail
		conn2, resp2, err2 := dialer.Dial(wsURL, prepareHeaders(token))
		if conn2 != nil {
			conn2.Close()
		}
		require.Error(t, err2, "Expired JWT should fail WebSocket connection")
		if resp2 != nil {
			t.Logf("Expired JWT - Status: %d", resp2.StatusCode)
			require.Equal(t, http.StatusUnauthorized, resp2.StatusCode, "Expired JWT should be rejected")
		}
	})
}
