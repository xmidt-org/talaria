// SPDX-FileCopyrightText: 2017 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testCase struct {
	name        string
	left        interface{}
	right       interface{}
	expected    bool
	expectedErr error
}

func testBinOp(testCases []testCase, operation string, t *testing.T) {
	require := require.New(t)
	op, err := newBinOp(operation)
	require.Nil(err)
	require.Equal(op.name(), operation)

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			assert := assert.New(t)
			actual, err := op.evaluate(testCase.left, testCase.right)

			assert.Equal(testCase.expected, actual)
			assert.Equal(testCase.expectedErr, err)
		})
	}
}

func TestIntersects(t *testing.T) {
	testCases := []testCase{
		{
			name:     "Slices",
			left:     []string{"red", "green", "blue"},
			right:    []string{"red", "green", "blue"},
			expected: true,
		},
		{
			name:     "Arrays",
			left:     [2]string{"red", "green"},
			right:    [3]string{"red", "green", "blue"},
			expected: true,
		},
		{
			name:     "Mixed types. Partial intersection",
			left:     []interface{}{1, "one", 1.0, true},
			right:    []int{1},
			expected: true,
		},
		{
			name:  "Mixed types. No intersection",
			left:  []interface{}{1, "one", 1.0},
			right: []string{"orange", "apple"},
		},
		{
			name:  "Empty slice",
			left:  []interface{}{},
			right: []string{"orange", "apple"},
		},
		{
			name:        "Unsupported type for first argument",
			left:        true,
			right:       []string{"orange", "apple"},
			expectedErr: errIterableTypeOnly,
		},

		{
			name:        "Unsupported type for second argument",
			left:        []interface{}{3},
			right:       3,
			expectedErr: errIterableTypeOnly,
		},

		{
			name:  "Nil values",
			left:  nil,
			right: 3,
		},
	}
	testBinOp(testCases, IntersectsOp, t)
}

func TestContains(t *testing.T) {
	testCases := []testCase{
		{
			name:     "A member",
			left:     []interface{}{"two", 2},
			right:    2,
			expected: true,
		},
		{
			name:  "Not a member",
			left:  []string{"svalinn", "gungnir"},
			right: "talaria",
		},
		{
			name:        "Type error",
			left:        3, //not an iterable object
			right:       "three",
			expectedErr: errIterableTypeOnly,
		},
		{
			name:  "Undefined left argument",
			left:  nil,
			right: "three",
		},
	}
	testBinOp(testCases, ContainsOp, t)
}

func TestEquals(t *testing.T) {
	// no need for thorough tests here as implementation
	// uses reflect.DeepEqual
	testCases := []testCase{
		{
			name:     "Equals",
			left:     "theSame",
			right:    "theSame",
			expected: true,
		},

		{
			name:  "Not Equals",
			left:  34,
			right: 43,
		},
	}
	testBinOp(testCases, EqualsOp, t)
}

func TestGreater(t *testing.T) {
	testCases := []testCase{
		{
			name:        "Not a number",
			left:        "NaNaNaNaN Batman",
			right:       0,
			expectedErr: errNumericalTypeOnly,
		},

		{
			name:     "Max int64",
			left:     math.MaxInt64,
			right:    math.MaxInt8,
			expected: true,
		},
	}
	testBinOp(testCases, GreaterThanOp, t)
}

func TestUnsupportedOp(t *testing.T) {
	assert := assert.New(t)

	op, err := newBinOp("modulo")
	assert.Nil(op)
	assert.Equal(errOpNotSupported, err)
}
