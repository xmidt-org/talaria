// SPDX-FileCopyrightText: 2017 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testURLFilterInvalidDefaultScheme(t *testing.T) {
	assert := assert.New(t)
	urlFilter, err := NewURLFilter(&Outbounder{
		DefaultScheme:  "http",
		AllowedSchemes: []string{"https"},
	})

	assert.Nil(urlFilter)
	assert.Error(err)
}

func testURLFilterFilter(t *testing.T) {
	var (
		assert  = assert.New(t)
		require = require.New(t)

		testData = []struct {
			outbounder       *Outbounder
			input            string
			expectedFiltered string
			expectsError     bool
		}{
			{nil, "foobar.com", "https://foobar.com", false},
			{nil, "foobar.com?test=1&a=2", "https://foobar.com?test=1&a=2", false},
			{nil, "foobar.com:8080", "https://foobar.com:8080", false},
			{nil, "xxx://foobar.com", "", true},
			{nil, "http://foobar.com:1234", "", true},

			{&Outbounder{DefaultScheme: "ftp", AllowedSchemes: []string{"ftp", "https"}}, "foobar.com", "ftp://foobar.com", false},
			{&Outbounder{DefaultScheme: "ftp", AllowedSchemes: []string{"ftp", "https"}}, "foobar.com?test=1", "ftp://foobar.com?test=1", false},
			{&Outbounder{DefaultScheme: "ftp", AllowedSchemes: []string{"ftp", "https"}}, "https://foobar.com", "https://foobar.com", false},
			{&Outbounder{DefaultScheme: "ftp", AllowedSchemes: []string{"ftp", "https"}}, "https://foobar.com?test=1", "https://foobar.com?test=1", false},
			{&Outbounder{DefaultScheme: "ftp", AllowedSchemes: []string{"ftp", "https"}}, "http://foobar.com", "", true},
		}
	)

	for _, record := range testData {
		t.Logf("%#v", record)
		urlFilter, err := NewURLFilter(record.outbounder)
		require.NotNil(urlFilter)
		require.NoError(err)

		actualFiltered, err := urlFilter.Filter(record.input)
		assert.Equal(record.expectedFiltered, actualFiltered)
		assert.Equal(record.expectsError, err != nil)
	}
}

func TestURLFilter(t *testing.T) {
	t.Run("InvalidDefaultScheme", testURLFilterInvalidDefaultScheme)
	t.Run("Filter", testURLFilterFilter)
}
