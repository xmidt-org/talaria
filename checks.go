package main

import (
	"errors"
	"reflect"
	"strings"
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

	if reflect.TypeOf(this).Kind() != reflect.Slice ||
		reflect.TypeOf(that).Kind() != reflect.Slice {
		return false, errors.New("Only slices are supported as values")
	}

	m := make(map[interface{}]struct{})

	a := reflect.ValueOf(this)
	for i := 0; i < a.Len(); i++ {
		m[a.Index(i).Interface()] = struct{}{}
	}

	b := reflect.ValueOf(that)
	for i := 0; i < b.Len(); i++ {
		if _, ok := m[b.Index(i).Interface()]; ok {
			return true, nil
		}
	}

	return false, nil
}
