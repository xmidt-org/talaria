package main

import (
	"encoding/json"
	"testing"
)

func TestSimpleMapLoader(t *testing.T) {
	testCases := []struct {
		name     string
		json     string
		path     []string
		expected interface{}
	}{
		{
			name:     "Third level key value found",
			json:     `{"rgb":{"red": {"hex": "#ff0000"} }}`,
			path:     []string{"rgb", "red", "hex"},
			expected: "#ff0000",
		},

		{
			name:     "Second level key value not found",
			json:     `{"rgb":{"green": {"hex": "#00ff00"} }}`,
			path:     []string{"rgb", "red", "hex"},
			expected: nil,
		},

		{
			name:     "Empty path",
			json:     `{"rgb":{"green": {"hex": "#00ff00"} }}`,
			path:     []string{"rhex"},
			expected: nil,
		},
	}
	var mapLoader MapLoader = &simpleMapLoader{}

	for _, testCase := range testCases {
		var m map[string]interface{}
		e := json.Unmarshal([]byte(testCase.json), &m)
		if e != nil {
			t.Fatal(e)
		}

		actual := mapLoader.Load(m, testCase.path)
		if actual != testCase.expected {
			t.Errorf("Test case: '%s'. Expected '%v' but got '%v'", testCase.name, testCase.expected, actual)
		}
	}
}
