package main

import (
	"errors"
	"github.com/spf13/cast"
	"reflect"
	"strings"
)

var (
	ErrIterableTypeOnly = errors.New("Only slices and arrays are currently supportd as iterable")
	ErrIntTypeOnly      = errors.New("Only int valiues are supported")
)

const (
	//Name of supported operations
	IntersectOp = "intersect"

	//To be implemented
	ContainsOp    = "contains"
	EqualsOp      = "=="
	GreaterThanOp = ">"
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

//DefaultPathParser simply splits the source string into sep-delimited
//parts. It never returns an error
var DefaultPathParser = func(source string, sep string) (Path, error) {
	return strings.Split(source, sep), nil
}

//Path is a convenient string list type used as a key path
//to load values from anywhere in a map
type Path []string

//PathParser defines the form
type PathParser func(source, sep string) (Path, error)

type parsedCheck struct {
	userCredentialPath   Path
	deviceCredentialPath Path
	assertion            Check
	inversed             bool
}

type checkParser struct {
	pathParser PathParser
}

func (c checkParser) parse(configs []DeviceAccessCheck) ([]parsedCheck, error) {
	parsedChecks := make([]parsedCheck, len(configs))
	for i, config := range configs {

		if config.Sep == "" {
			config.Sep = "."
		}

		userCredentialPath, err := c.pathParser(config.UserCredentialPath, config.Sep)
		if err != nil {
			return nil, err
		}

		deviceCredentialPath, err := c.pathParser(config.DeviceCredentialPath, config.Sep)
		if err != nil {
			return nil, err
		}

		parsedCheck := parsedCheck{
			userCredentialPath:   userCredentialPath,
			deviceCredentialPath: deviceCredentialPath,
		}

		check, err := newCheck(config.Op)
		if err != nil {
			return nil, err
		}

		parsedCheck.assertion = check
		parsedChecks[i] = parsedCheck
	}

	return parsedChecks, nil
}

func newCheck(operation string) (Check, error) {
	switch operation {
	case IntersectOp:
		return Intersection, nil
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

	// a, ok := iterable(this)
	// if !ok {
	// 	return false, ErrIterableTypeOnly
	// }

	a := cast.ToSlice(this)

	if a == nil {
		return false, ErrIterableTypeOnly
	}

	// b, ok := iterable(that)

	// if !ok {
	// 	return false, ErrIterableTypeOnly
	// }
	b := cast.ToSlice(that)
	if b == nil {
		return false, ErrIntTypeOnly
	}

	m := make(map[interface{}]struct{})

	for _, e := range a {
		m[e] = struct{}{}
	}

	for _, e := range b {
		if _, ok := m[e]; ok {
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

// var GreaterThan CheckFunc = func(this interface{}, that interface{}) (bool, error) {
// 	thisInt, ok := this.(int)
// 	thatInt, ok := that.(int)
// 	thatInt := int(that)

// }
