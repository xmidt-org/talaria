package main

import (
	"encoding/json"
	"testing"
)

func TestGet(t *testing.T) {
	testCases := []struct {
		name     string
		json     string
		path     string
		expected interface{}
	}{
		{
			name:     "Third level key value found",
			json:     `{"rgb":{"red": {"hex": "#ff0000"} }}`,
			path:     "rgb.red.hex",
			expected: "#ff0000",
		},

		{
			name:     "Second level key value not found",
			json:     `{"rgb":{"green": {"hex": "#00ff00"} }}`,
			path:     "rgb.red.hex",
			expected: nil,
		},

		{
			name:     "Empty path",
			json:     `{"rgb":{"green": {"hex": "#00ff00"} }}`,
			path:     "rhex",
			expected: nil,
		},
	}

	for _, testCase := range testCases {
		var m map[string]interface{}
		e := json.Unmarshal([]byte(testCase.json), &m)
		if e != nil {
			t.Fatal(e)
		}

		actual := getValueAtPath(m, testCase.path)
		if actual != testCase.expected {
			t.Errorf("Test case: '%s'. Expected '%v' but got '%v'", testCase.name, testCase.expected, actual)
		}
	}
}

func TestIntersect(t *testing.T) {
	testCases := []struct {
		name      string
		jsonA     string
		jsonB     string
		pathA     string
		pathB     string
		expected  bool
		shouldErr bool
	}{
		{
			name:     "Same slices. Full intersection",
			jsonA:    `{"rgb":["red", "green", "blue"]}`,
			jsonB:    `{"colors":["red", "green", "blue"]}`,
			pathA:    "rgb",
			pathB:    "colors",
			expected: true,
		},

		{
			name:     "Mixed types. Partial intersection",
			jsonA:    `{"types":[1, "one", 1.0]}`,
			jsonB:    `{"supported":[1]}`,
			pathA:    "types",
			pathB:    "supported",
			expected: true,
		},

		{
			name:     "Mixed types. No intersection",
			jsonA:    `{"types":[1, "one", 1.0]}`,
			jsonB:    `{"fruits":["orange", "apple"]}`,
			pathA:    "types",
			pathB:    "fruits",
			expected: false,
		},

		{
			name:     "Empty slice",
			jsonA:    `{"types":[1, "one", 1.0]}`,
			jsonB:    `{"fruits":["orange", "apple"]}`,
			pathA:    "types.wrongpath",
			pathB:    "fruits",
			expected: false,
		},

		{
			name:      "Value at pathA is not a slice",
			jsonA:     `{"map": {"key":"value"}}`,
			jsonB:     `{"fruits":["orange", "apple"]}`,
			pathA:     "map.key",
			pathB:     "fruits",
			expected:  false,
			shouldErr: true,
		},
	}

	for _, testCase := range testCases {
		var aMap map[string]interface{}
		e := json.Unmarshal([]byte(testCase.jsonA), &aMap)
		if e != nil {
			t.Fatal(e)
		}

		var bMap map[string]interface{}
		e = json.Unmarshal([]byte(testCase.jsonB), &bMap)
		if e != nil {
			t.Fatal(e)
		}

		actual, err := Intersect(aMap, bMap, testCase.pathA, testCase.pathB)
		if err != nil && !testCase.shouldErr {
			t.Errorf("Test case: '%s'. Error was not expected: %v", testCase.name, err)
		}

		if actual != testCase.expected {
			t.Errorf("Test case: '%s'. Expected '%v' but got '%v'", testCase.name, testCase.expected, actual)
		}
	}
}
