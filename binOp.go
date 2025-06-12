// SPDX-FileCopyrightText: 2017 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"errors"
	"reflect"

	"github.com/spf13/cast"
)

// Errors
var (
	errIterableTypeOnly  = errors.New("only slices and arrays are currently supported as iterable")
	errNumericalTypeOnly = errors.New("only numerical values are supported")
	errOpNotSupported    = errors.New("operation not supported")
)

// Supported operations
const (
	IntersectsOp  = "intersects"
	ContainsOp    = "contains"
	EqualsOp      = "eq"
	GreaterThanOp = "gt"
)

// binOp encapsulates the execution of a generic binary operator.
type binOp interface {
	// evaluate applies the operation from left to right
	evaluate(left, right interface{}) (bool, error)

	// name is the name of the operation.
	name() string
}

func newBinOp(operation string) (binOp, error) {
	switch operation {
	case IntersectsOp:
		return new(intersects), nil
	case ContainsOp:
		return new(contains), nil
	case EqualsOp:
		return new(equals), nil
	case GreaterThanOp:
		return new(greaterThan), nil
	default:
		return nil, errOpNotSupported
	}
}

// intersects returns true if left and right contain
// some shared member. False otherwise.
// Note:
// - only elements which can be casted to slices are currently supported.
type intersects struct{}

func (i intersects) evaluate(left, right interface{}) (bool, error) {
	if left == nil || right == nil {
		return false, nil
	}

	a, ok := iterable(left)
	if !ok {
		return false, errIterableTypeOnly
	}

	b, ok := iterable(right)
	if !ok {
		return false, errIterableTypeOnly
	}

	m := make(map[interface{}]bool)

	for _, e := range a {
		m[e] = true
	}

	for _, e := range b {
		if m[e] {
			return true, nil
		}
	}

	return false, nil
}

func (i intersects) name() string {
	return IntersectsOp
}

// contains returns true if right is a member of left.
// Note: only slices are supported.
type contains struct{}

func (c contains) evaluate(left interface{}, right interface{}) (bool, error) {
	if left == nil {
		return false, nil
	}

	l, ok := iterable(left)
	if !ok {
		return false, errIterableTypeOnly
	}

	for _, e := range l {
		if reflect.DeepEqual(e, right) {
			return true, nil
		}
	}

	return false, nil
}

func (c contains) name() string {
	return ContainsOp
}

// equals returns true if left and right are equal as defined by reflect.DeepEqual()
type equals struct{}

func (e equals) evaluate(left interface{}, right interface{}) (bool, error) {
	return reflect.DeepEqual(left, right), nil
}

func (e equals) name() string {
	return EqualsOp
}

type greaterThan struct{}

func (g greaterThan) evaluate(left interface{}, right interface{}) (bool, error) {
	leftNumber, leftErr := cast.ToInt64E(left)
	rightNumber, rightErr := cast.ToInt64E(right)
	if leftErr != nil || rightErr != nil {
		return false, errNumericalTypeOnly
	}

	return leftNumber > rightNumber, nil
}

func (g greaterThan) name() string {
	return GreaterThanOp
}

// iterable checks that the given interface is of a
// supported iterable reflect.Kind and if so,
// returns a slice of its elements
func iterable(e interface{}) ([]interface{}, bool) {
	switch reflect.TypeOf(e).Kind() {
	case reflect.Slice, reflect.Array:
		v := reflect.ValueOf(e)
		n := v.Len()

		elements := make([]interface{}, n)
		for i := 0; i < n; i++ {
			elements[i] = v.Index(i).Interface()
		}
		return elements, true
	}

	return nil, false
}
