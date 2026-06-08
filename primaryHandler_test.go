// SPDX-FileCopyrightText: 2017 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/stretchr/testify/assert"
	"github.com/xmidt-org/bascule"
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
	}

	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {
			assert.Equal(t, tc.expected, authStatusCoder(tc.req, tc.err))
		})
	}
}
