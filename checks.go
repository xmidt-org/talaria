package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/xmidt-org/bascule"
)

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

func parseDeviceAccessChecks(configs []deviceAccessCheck) ([]parsedCheck, bascule.MultiError) {
	var (
		parsedChecks = make([]parsedCheck, len(configs))
		errs         bascule.Errors
	)
	for i, config := range configs {
		parsedCheck := parsedCheck{
			name:                 strings.Trim(config.Name, " "),
			wrpCredentialPath:    strings.Trim(config.WRPCredentialPath, " "),
			deviceCredentialPath: strings.Trim(config.DeviceCredentialPath, " "),
			inputValue:           config.InputValue,
			inversed:             config.Inversed,
		}

		errsForCheck := newNamedMultiError(config.Name, i)

		if anyEmpty(parsedCheck.name, parsedCheck.deviceCredentialPath) {
			errsForCheck.Append(errors.New("Name and DeviceCredentialPath are required"))
		}

		if parsedCheck.inputValue == nil && parsedCheck.wrpCredentialPath == "" {
			errsForCheck.Append(errors.New("Either inputValue or wrpCredentialPath must be provided"))
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
