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
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/xmidt-org/bascule"
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
