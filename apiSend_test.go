// SPDX-FileCopyrightText: 2025 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0

//go:build integration

package main

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

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

// TestGetDevices_WithAuth tests authentication behavior for the devices endpoint.
func TestGetDevices_WithAuth(t *testing.T) {
	fixture := setupIntegrationTest(t, "talaria_template.yaml")

	tests := []struct {
		name           string
		username       string
		password       string
		expectedStatus int
		description    string
	}{
		{
			name:           "valid_credentials",
			username:       "user",
			password:       "pass",
			expectedStatus: http.StatusOK,
			description:    "Valid credentials should return 200",
		},
		{
			name:           "invalid_credentials",
			username:       "wrong",
			password:       "wrong",
			expectedStatus: http.StatusUnauthorized,
			description:    "Invalid credentials should return 401",
		},
		{
			name:           "no_credentials",
			username:       "",
			password:       "",
			expectedStatus: http.StatusUnauthorized,
			description:    "No credentials should return 401",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body, statusCode, err := fixture.GetDevices(tt.username, tt.password)
			require.NoError(t, err)

			t.Logf("%s - Status: %d, Body: %s", tt.description, statusCode, body)
			require.Equal(t, tt.expectedStatus, statusCode, tt.description)
		})
	}
}

// TestCustomAPICall demonstrates using the fixture for custom API calls with different verbs.
func TestCustomAPICall(t *testing.T) {
	fixture := setupIntegrationTest(t, "talaria_template.yaml")

	t.Run("GET_with_bearer_token", func(t *testing.T) {
		// Example: Test with Bearer token instead of Basic auth
		req, err := fixture.NewRequest("GET", "/api/v2/devices", nil)
		require.NoError(t, err)

		fixture.WithBearerToken(req, "some-token")

		resp, err := fixture.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		t.Logf("Bearer token auth status: %d", resp.StatusCode)
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
