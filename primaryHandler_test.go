// SPDX-FileCopyrightText: 2017 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xmidt-org/bascule"
	"github.com/xmidt-org/sallust"
	"github.com/xmidt-org/touchstone"
	"github.com/xmidt-org/webpa-common/v2/device"
	"go.uber.org/zap"
)

func TestAuthStatusCoder(t *testing.T) {
	tests := []struct {
		description string
		req         *http.Request
		err         error
		expected    int
	}{
		{
			description: "Missing Credentials",
			err:         bascule.ErrMissingCredentials,
			expected:    http.StatusUnauthorized,
		},
		{
			description: "Bad Credentials",
			err:         bascule.ErrBadCredentials,
			expected:    http.StatusUnauthorized,
		},
		{
			description: "Invalid Credentials",
			err:         bascule.ErrInvalidCredentials,
			expected:    http.StatusBadRequest,
		},
		{
			description: "Unauthorized",
			err:         bascule.ErrUnauthorized,
			expected:    http.StatusForbidden,
		},
		{
			description: "Unknown Error",
			err:         errors.New("some error"),
			expected:    http.StatusForbidden,
		},
		{
			description: "V2 Invalid Credentials",
			req:         mux.SetURLVars(httptest.NewRequest(http.MethodGet, "/api/v2/device/send", nil), map[string]string{"version": v2}),
			err:         bascule.ErrInvalidCredentials,
			expected:    http.StatusBadRequest,
		},
		{
			description: "V2 Bad Credentials",
			req:         mux.SetURLVars(httptest.NewRequest(http.MethodGet, "/api/v2/device/send", nil), map[string]string{"version": v2}),
			err:         bascule.ErrBadCredentials,
			expected:    http.StatusForbidden,
		},
		{
			description: "V3 Invalid Credentials",
			req:         mux.SetURLVars(httptest.NewRequest(http.MethodGet, "/api/v3/device/send", nil), map[string]string{"version": version}),
			err:         bascule.ErrInvalidCredentials,
			expected:    http.StatusBadRequest,
		},
		{
			description: "Nil Error",
			err:         nil,
			expected:    http.StatusForbidden,
		},
	}

	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {
			assert.Equal(t, tc.expected, authStatusCoder(tc.req, tc.err))
		})
	}
}

func TestGetInboundTimeout(t *testing.T) {
	tests := []struct {
		description string
		value       string
		expected    time.Duration
	}{
		{
			description: "Valid Duration",
			value:       "45s",
			expected:    45 * time.Second,
		},
		{
			description: "Invalid Duration Uses Default",
			value:       "not-a-duration",
			expected:    DefaultInboundTimeout,
		},
		{
			description: "Empty Duration Uses Default",
			value:       "",
			expected:    DefaultInboundTimeout,
		},
	}

	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {
			v := viper.New()
			v.Set(InboundTimeoutConfigKey, tc.value)

			assert.Equal(t, tc.expected, getInboundTimeout(v))
		})
	}
}

func TestBuildUserPassMap(t *testing.T) {
	logger := zap.NewNop()

	validA := base64.StdEncoding.EncodeToString([]byte("userA:passA"))
	validB := base64.StdEncoding.EncodeToString([]byte("userB:passB"))
	invalidBase64 := "%%%"
	missingColon := base64.StdEncoding.EncodeToString([]byte("userCpassC"))
	emptyUser := base64.StdEncoding.EncodeToString([]byte(":passD"))

	decoded := buildUserPassMap(logger, []string{validA, invalidBase64, missingColon, emptyUser, validB})

	assert.Len(t, decoded, 2)
	assert.Equal(t, "passA", decoded["userA"])
	assert.Equal(t, "passB", decoded["userB"])
	_, hasMissingColonUser := decoded["userCpassC"]
	assert.False(t, hasMissingColonUser)
	_, hasEmptyUser := decoded[""]
	assert.False(t, hasEmptyUser)
}

// newTestTouchstoneFactory builds a touchstone.Factory with an isolated prometheus registry
// suitable for use inside unit tests.
func newTestTouchstoneFactory(t *testing.T) *touchstone.Factory {
	t.Helper()
	cfg := touchstone.Config{DefaultNamespace: "test", DefaultSubsystem: "primary"}
	_, pr, err := touchstone.New(cfg)
	require.NoError(t, err)
	return touchstone.NewFactory(cfg, sallust.Default(), pr)
}

// newTestDeviceManager builds a real but unconfigured device.Manager for unit test fixtures.
func newTestDeviceManager() device.Manager {
	return device.NewManager(&device.Options{})
}

func TestBuildDeviceAccessCheck(t *testing.T) {
	logger := zap.NewNop()
	counter := prometheus.NewCounterVec(
		prometheus.CounterOpts{Name: "test_wrp_counter", Help: "test"},
		[]string{reasonLabel, outcomeLabel},
	)

	validCheck := deviceAccessCheck{
		Name:                 "partner-check",
		DeviceCredentialPath: "partner-id",
		WRPCredentialPath:    "partner-id",
		Op:                   "contains",
	}

	tests := []struct {
		description string
		config      *deviceAccessCheckConfig
		expectErr   bool
	}{
		{
			description: "No Checks",
			config:      &deviceAccessCheckConfig{Type: "enforce"},
			expectErr:   true,
		},
		{
			description: "Invalid Type",
			config: &deviceAccessCheckConfig{
				Type:   "unknown",
				Checks: []deviceAccessCheck{validCheck},
			},
			expectErr: true,
		},
		{
			description: "Invalid Check - Missing Name",
			config: &deviceAccessCheckConfig{
				Type: "enforce",
				Checks: []deviceAccessCheck{
					{DeviceCredentialPath: "path", WRPCredentialPath: "path", Op: "contains"},
				},
			},
			expectErr: true,
		},
		{
			description: "Valid Enforce Type",
			config: &deviceAccessCheckConfig{
				Type:   "enforce",
				Checks: []deviceAccessCheck{validCheck},
			},
		},
		{
			description: "Valid Monitor Type",
			config: &deviceAccessCheckConfig{
				Type:   "monitor",
				Checks: []deviceAccessCheck{validCheck},
			},
		},
		{
			description: "Default Sep Applied",
			config: &deviceAccessCheckConfig{
				Type:   "monitor",
				Sep:    "",
				Checks: []deviceAccessCheck{validCheck},
			},
		},
		{
			description: "Custom Sep Applied",
			config: &deviceAccessCheckConfig{
				Type:   "monitor",
				Sep:    "/",
				Checks: []deviceAccessCheck{validCheck},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {
			result, err := buildDeviceAccessCheck(tc.config, logger, counter, newTestDeviceManager())
			if tc.expectErr {
				assert.Error(t, err)
				assert.Nil(t, result)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, result)
			}
		})
	}
}

func TestNewPrimaryHandler(t *testing.T) {
	newViper := func() *viper.Viper { return viper.New() }

	tests := []struct {
		description string
		setup       func(*viper.Viper)
		expectErr   bool
	}{
		{
			description: "Minimal Config",
			setup:       func(v *viper.Viper) {},
		},
		{
			description: "With Basic Auth",
			setup: func(v *viper.Viper) {
				v.Set(ServiceBasicAuthConfigKey, []string{
					base64.StdEncoding.EncodeToString([]byte("user:pass")),
				})
			},
		},
		{
			description: "With Basic Auth But Empty Entries",
			setup: func(v *viper.Viper) {
				v.Set(ServiceBasicAuthConfigKey, []string{"%%%"})
			},
		},
		{
			description: "Fail Open False",
			setup: func(v *viper.Viper) {
				v.Set(FailOpenConfigKey, false)
			},
		},
		{
			description: "DeviceAccessCheck Bad Config",
			setup: func(v *viper.Viper) {
				v.Set(DeviceAccessCheckConfigKey+".type", "enforce")
				// no checks → buildDeviceAccessCheck returns error
			},
			expectErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {
			v := newViper()
			tc.setup(v)

			tf := newTestTouchstoneFactory(t)
			logger := zap.NewNop()
			manager := newTestDeviceManager()
			router := mux.NewRouter()

			handler, err := NewPrimaryHandler(logger, manager, v, nil, nil, NoOpConstructor, tf, router)
			if tc.expectErr {
				assert.Error(t, err)
				assert.Nil(t, handler)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, handler)
			}
		})
	}
}
