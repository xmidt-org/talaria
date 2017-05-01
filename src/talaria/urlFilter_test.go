package main

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

func testURLFilterInvalidAssumeScheme(t *testing.T) {
	assert := assert.New(t)
	assert.Panics(func() {
		// assume scheme must appear in the allowed schemes
		newURLFilter("http", []string{"https"})
	})
}

func testURLFilterFilter(t *testing.T) {
	var (
		assert   = assert.New(t)
		testData = []struct {
			urlFilter        *urlFilter
			input            string
			expectedFiltered string
			expectsError     bool
		}{
			{newURLFilter("", nil), "foobar.com", "https://foobar.com", false},
			{newURLFilter("", nil), "foobar.com?test=1&a=2", "https://foobar.com?test=1&a=2", false},
			{newURLFilter("", nil), "foobar.com:8080", "https://foobar.com:8080", false},
			{newURLFilter("", nil), "xxx://foobar.com", "", true},
			{newURLFilter("", nil), "http://foobar.com:1234", "", true},

			{newURLFilter("xyz", []string{"xyz", "pdq"}), "foobar.com", "xyz://foobar.com", false},
			{newURLFilter("xyz", []string{"xyz", "pdq"}), "foobar.com:9000", "xyz://foobar.com:9000", false},
			{newURLFilter("xyz", []string{"xyz", "pdq"}), "xyz://foobar.com", "xyz://foobar.com", false},
			{newURLFilter("xyz", []string{"xyz", "pdq"}), "pdq://foobar.com:443", "pdq://foobar.com:443", false},
			{newURLFilter("xyz", []string{"xyz", "pdq"}), "xxx://foobar.com:443", "", true},
		}
	)

	for _, record := range testData {
		t.Logf("%#v", record)
		actualFiltered, err := record.urlFilter.filter(record.input)
		assert.Equal(record.expectedFiltered, actualFiltered)
		assert.Equal(record.expectsError, err != nil)
	}
}

func TestURLFilter(t *testing.T) {
	t.Run("InvalidAssumeScheme", testURLFilterInvalidAssumeScheme)
	t.Run("Filter", testURLFilterFilter)
}
