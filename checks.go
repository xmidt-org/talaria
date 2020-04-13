package main

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/spf13/cast"
)

var (
	ErrIterableTypeOnly = errors.New("Only slices and arrays are currently supportd as iterable")
	ErrIntTypeOnly      = errors.New("Only int valiues are supported")
)

// Supported check operations
const (
	IntersectOp = "intersect"
	ContainsOp  = "contains"
	EqualsOp    = "=="
)

//Check describes the behavior of an assertion operation (i.e. contains) that
//can be executed. The operation is applied from left to right (i.e. does 'this' contain 'that')
type Check interface {
	Run(this, that interface{}) (bool, error)
}

//CheckFunc allows functions to satisfy the Check interface
type CheckFunc func(interface{}, interface{}) (bool, error)

func (c CheckFunc) Run(this, that interface{}) (bool, error) {
	return c(this, that)
}

type parsedCheck struct {
	name                 string
	wrpCredentialPath    string
	deviceCredentialPath string
	assertion            Check
	inversed             bool
}

func parseDeviceAccessChecks(configs []deviceAccessCheck) ([]parsedCheck, []error) {
	var (
		parsedChecks = make([]parsedCheck, len(configs))
		errs         []error
	)
	for i, config := range configs {
		parsedCheck := parsedCheck{
			name:                 strings.Trim(config.Name, " "),
			wrpCredentialPath:    strings.Trim(config.WRPCredentialPath, " "),
			deviceCredentialPath: strings.Trim(config.DeviceCredentialPath, " "),
			inversed:             config.Inversed,
		}

		errMsg := ""
		if parsedCheck.name == "" || parsedCheck.wrpCredentialPath == "" || parsedCheck.deviceCredentialPath == "" {
			errMsg += "No fields should be empty."
		}

		check, err := newCheck(config.Operation)
		if err != nil {
			errMsg += " " + err.Error()
		}

		if len(errMsg) > 0 {
			errs = append(errs, fmt.Errorf("Check %d: %s", i, errMsg))
		}

		if len(errs) > 0 {
			continue
		}

		parsedCheck.assertion = check
		parsedChecks[i] = parsedCheck
	}

	if len(errs) > 0 {
		return nil, errs

	}

	return parsedChecks, nil
}

func newCheck(operation string) (Check, error) {
	switch operation {
	case IntersectOp:
		return Intersection, nil
	case ContainsOp:
		return Contains, nil
	default:
		return nil, errors.New("Operation not supported")
	}
}

//Intersection returns true if this and that contain some shared member.
//False otherwise.
//Note: only slices are currently supported
var Intersection CheckFunc = func(this interface{}, that interface{}) (bool, error) {
	if this == nil || that == nil {
		return false, nil
	}

	a := cast.ToSlice(this)

	if a == nil {
		return false, ErrIterableTypeOnly
	}

	b := cast.ToSlice(that)
	if b == nil {
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

//Contains returns true if e is a member of the iterable object list.
//ErrIterableTypeOnly is returned if list is not a supported iterable object (Slices and Arrays).
var Contains CheckFunc = func(list interface{}, element interface{}) (bool, error) {
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

//Equal delegates its arguments to reflect.DeepEqual()
var Equal CheckFunc = func(this interface{}, that interface{}) (bool, error) {
	return reflect.DeepEqual(this, that), nil
}
