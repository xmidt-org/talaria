// SPDX-FileCopyrightText: 2017 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
)

const testAuthHeader = "Authorization"

func TestSanitizeHeaders(t *testing.T) {
	testCases := []struct {
		Description string
		Input       http.Header
		Expected    http.Header
	}{
		{
			Description: "Filtered",
			Input:       http.Header{testAuthHeader: []string{"Basic xyz"}, "HeaderA": []string{"x"}},
			Expected:    http.Header{"HeaderA": []string{"x"}, "Authorization-Type": []string{"Basic"}},
		},
		{
			Description: "Handled human error",
			Input:       http.Header{testAuthHeader: []string{"BasicXYZ"}, "HeaderB": []string{"y"}},
			Expected:    http.Header{"HeaderB": []string{"y"}},
		},
		{
			Description: "Not a perfect system",
			Input:       http.Header{testAuthHeader: []string{"MySecret IWantToLeakIt"}},
			Expected:    http.Header{"Authorization-Type": []string{"MySecret"}},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.Description, func(t *testing.T) {
			assert := assert.New(t)
			actual := sanitizeHeaders(tc.Input)
			assert.Equal(tc.Expected, actual)
		})

	}
}
