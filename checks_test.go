package main

import (
	"testing"
)

func TestIntersect(t *testing.T) {
	testCases := []struct {
		name      string
		this      interface{}
		that      interface{}
		expected  bool
		shouldErr bool
	}{
		{
			name:     "Same slices. Full intersection",
			this:     []string{"red", "green", "blue"},
			that:     []string{"red", "green", "blue"},
			expected: true,
		},

		{
			name:     "Mixed types. Partial intersection",
			this:     []interface{}{1, "one", 1.0},
			that:     []int{1},
			expected: true,
		},

		{
			name:     "Mixed types. No intersection",
			this:     []interface{}{1, "one", 1.0},
			that:     []string{"orange", "apple"},
			expected: false,
		},

		{
			name:     "Empty slice",
			this:     []interface{}{},
			that:     []string{"orange", "apple"},
			expected: false,
		},

		{
			name:      "Value is not a slice",
			this:      "notASlice",
			that:      []string{"orange", "apple"},
			expected:  false,
			shouldErr: true,
		},
	}

	for _, testCase := range testCases {
		actual, err := Intersection(testCase.this, testCase.that)
		if err != nil && !testCase.shouldErr {
			t.Errorf("Test case: '%s'. Error was not expected: %v", testCase.name, err)
		}

		if actual != testCase.expected {
			t.Errorf("Test case: '%s'. Expected '%v' but got '%v'", testCase.name, testCase.expected, actual)
		}
	}
}
