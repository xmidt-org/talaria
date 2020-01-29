package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIntersect(t *testing.T) {
	testCases := []struct {
		name       string
		list       interface{}
		element    interface{}
		shouldPass bool
		shouldErr  bool
	}{
		{
			name:       "Slices",
			list:       []string{"red", "green", "blue"},
			element:    []string{"red", "green", "blue"},
			shouldPass: true,
		},
		{
			name:       "Arrays",
			list:       [2]string{"red", "green"},
			element:    [3]string{"red", "green", "blue"},
			shouldPass: true,
		},
		{
			name:       "Mixed types. Partial intersection",
			list:       []interface{}{1, "one", 1.0, true},
			element:    []int{1},
			shouldPass: true,
		},
		{
			name:    "Mixed types. No intersection",
			list:    []interface{}{1, "one", 1.0},
			element: []string{"orange", "apple"},
		},
		{
			name:    "Empty slice",
			list:    []interface{}{},
			element: []string{"orange", "apple"},
		},
		{
			name:      "Unsupported type for first argument",
			list:      true,
			element:   []string{"orange", "apple"},
			shouldErr: true,
		},

		{
			name:      "Unsupported type for second argument",
			list:      []interface{}{3},
			element:   3,
			shouldErr: true,
		},

		{
			name:    "Nil values",
			list:    nil,
			element: 3,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			assert := assert.New(t)
			actual, err := Intersection(testCase.list, testCase.element)

			assert.Equal(testCase.shouldPass, actual)

			if testCase.shouldErr {
				assert.NotNil(err)
			} else {
				assert.Nil(err)
			}
		})
	}
}

func TestContains(t *testing.T) {
	testCases := []struct {
		name       string
		list       interface{}
		element    interface{}
		shouldErr  bool
		shouldPass bool
	}{
		{
			name:       "A member",
			list:       []interface{}{"two", 2},
			element:    2,
			shouldPass: true,
		},
		{
			name:    "Not a member",
			list:    []string{"svalinn", "gungnir"},
			element: "talaria",
		},
		{
			name:      "Type error",
			list:      3, //not an iterable object
			element:   "three",
			shouldErr: true,
		},

		{
			name:    "Undefined list argument",
			list:    nil,
			element: "three",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			assert := assert.New(t)
			ok, err := Contains(testCase.list, testCase.element)

			assert.Equal(testCase.shouldPass, ok)

			if testCase.shouldErr {
				assert.NotNil(err)
			} else {
				assert.Nil(err)
			}
		})
	}
}
