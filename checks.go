package main

import (
	"errors"
	"fmt"
	"strings"
)

type namedMultiError struct {
	name   string
	errors []error
}

func newNamedMultiError(name, defaultName string) namedMultiError {
	if name == "" {
		name = defaultName
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

type parsedCheck struct {
	name                 string
	deviceCredentialPath string
	wrpCredentialPath    string
	inputValue           interface{}
	assertion            binOp
	inversed             bool
}

func anyEmpty(values ...string) bool {
	for _, value := range values {
		if value == "" {
			return true
		}
	}
	return false
}

func parseDeviceAccessCheck(config deviceAccessCheck) (*parsedCheck, error) {
	parsedCheck := &parsedCheck{
		name:                 strings.Trim(config.Name, " "),
		wrpCredentialPath:    strings.Trim(config.WRPCredentialPath, " "),
		deviceCredentialPath: strings.Trim(config.DeviceCredentialPath, " "),
		inputValue:           config.InputValue,
		inversed:             config.Inversed,
	}

	errs := newNamedMultiError(config.Name, "DeviceAccessCheck")

	if anyEmpty(parsedCheck.name, parsedCheck.deviceCredentialPath) {
		errs.Append(errors.New("Name and DeviceCredentialPath are required"))
	}

	if parsedCheck.inputValue == nil && parsedCheck.wrpCredentialPath == "" {
		errs.Append(errors.New("Either inputValue or wrpCredentialPath must be provided"))
	}

	check, err := newBinOp(config.Op)
	if err != nil {
		errs.Append(err)
	}

	parsedCheck.assertion = check

	if len(errs.errors) > 0 {
		return nil, err
	}

	return parsedCheck, errs
}
