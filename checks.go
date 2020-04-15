package main

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/xmidt-org/bascule"
)

var (
	ErrIterableTypeOnly = errors.New("Only slices and arrays are currently supported as iterable")
	ErrIntTypeOnly      = errors.New("Only int values are supported")
)

// Supported operations
const (
	IntersectOp = "intersect"
	ContainsOp  = "contains"
	EqualsOp    = "=="
)

// binOp encapsulates the execution of a generic binary operator.
type binOp interface {
	//Evaluate applies the operation from left to right
	Evaluate(left, right interface{}) (bool, error)

	// Name is the name of the operation.
	Name() string
}

type namedMultiError struct {
	name   string
	errors []error
}

func newNamedMultiError(name string, index int) namedMultiError {
	if name == "" {
		name = fmt.Sprintf("checks[%d]", index)
	}
	return namedMultiError{
		name: name,
	}
}

func (e *namedMultiError) Append(err error) {
	e.errors = append(e.errors, fmt.Errorf("'%s': %s", e.name, err.Error()))
}

func (e namedMultiError) Error() string {
	var errors []string
	for _, err := range e.errors {
		errors = append(errors, err.Error())
	}
	return strings.Join(errors, "\n")
}

func (e namedMultiError) Errors() []error {
	return e.errors
}

type parsedCheck struct {
	name                     string
	wrpCredentialPath        string
	deviceCredentialPath     string
	deviceCredentialExpected interface{}
	assertion                binOp
	inversed                 bool
}

func anyEmpty(values ...string) bool {
	for _, value := range values {
		if value == "" {
			return true
		}
	}
	return false
}

func parseDeviceAccessChecks(configs []deviceAccessCheck) ([]parsedCheck, bascule.MultiError) {
	var (
		parsedChecks = make([]parsedCheck, len(configs))
		errs         bascule.Errors
	)
	for i, config := range configs {
		parsedCheck := parsedCheck{
			name:                     strings.Trim(config.Name, " "),
			wrpCredentialPath:        strings.Trim(config.WRPCredentialPath, " "),
			deviceCredentialPath:     strings.Trim(config.DeviceCredentialPath, " "),
			deviceCredentialExpected: config.DeviceCredentialExpected,
			inversed:                 config.Inversed,
		}

		errsForCheck := newNamedMultiError(config.Name, i)

		if anyEmpty(parsedCheck.name, parsedCheck.deviceCredentialPath) {
			errsForCheck.Append(errors.New("Name and DeviceCredentialPath are required"))
		}

		if parsedCheck.deviceCredentialExpected == nil && parsedCheck.deviceCredentialPath == "" {
			errsForCheck.Append(errors.New("Either deviceCredentialExpected or deviceCredentialPath must be provided"))
		}

		check, err := newBinOp(config.Op)
		if err != nil {
			errsForCheck.Append(err)
		}

		if len(errsForCheck.errors) > 0 {
			errs = append(errs, errsForCheck)
			continue
		}

		parsedCheck.assertion = check
		parsedChecks[i] = parsedCheck
	}

	return parsedChecks, errs
}

func newBinOp(operation string) (binOp, error) {
	switch operation {
	case IntersectOp:
		return new(intersects), nil
	case ContainsOp:
		return new(contains), nil
	case EqualsOp:
		return new(equals), nil
	default:
		return nil, errors.New("Operation not supported")
	}
}

// intersects returns true if left and right contain
// some shared member. False otherwise.
// Note:
// - only elements which can be casted to slices are currently supported.
type intersects struct{}

func (i intersects) Evaluate(left, right interface{}) (bool, error) {
	if left == nil || right == nil {
		return false, nil
	}

	a, ok := iterable(left)
	if !ok {
		return false, ErrIterableTypeOnly
	}

	b, ok := iterable(right)
	if !ok {
		return false, ErrIterableTypeOnly
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

func (i intersects) Name() string {
	return IntersectOp
}

// contains returns true if right is a member of left.
// Note: only slices are supported.
type contains struct{}

func (c contains) Evaluate(list interface{}, element interface{}) (bool, error) {
	if list == nil {
		return false, nil
	}

	l, ok := iterable(list)
	if !ok {
		return false, ErrIterableTypeOnly
	}

	for _, e := range l {
		if reflect.DeepEqual(e, element) {
			return true, nil
		}
	}

	return false, nil
}

func (c contains) Name() string {
	return ContainsOp
}

// equals returns true if left and right at equas as defined by reflect.DeepEqual()
type equals struct{}

func (e equals) Evaluate(left interface{}, right interface{}) (bool, error) {
	return reflect.DeepEqual(left, right), nil
}

func (e equals) Name() string {
	return EqualsOp
}

//iterable checks that the given interface is of a
//supported iterable reflect.Kind and if so,
//returns a slice of its elements
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
