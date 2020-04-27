package main

import (
	"errors"
	"strings"
)

type parsedCheck struct {
	name                 string
	deviceCredentialPath string
	wrpCredentialPath    string
	inputValue           interface{}
	assertion            binOp
	inversed             bool
}

var (
	errNameRequired                    = errors.New("Name is required")
	errDeviceCredPathRequired          = errors.New("DeviceCredentialPath is required")
	errInputValueOrWRPCredPathRequired = errors.New("Either InputValue or WRPCredentialPath is required")
)

func parseDeviceAccessCheck(config deviceAccessCheck) (*parsedCheck, error) {
	parsedCheck := &parsedCheck{
		name:                 strings.TrimSpace(config.Name),
		wrpCredentialPath:    strings.TrimSpace(config.WRPCredentialPath),
		deviceCredentialPath: strings.TrimSpace(config.DeviceCredentialPath),
		inputValue:           config.InputValue,
		inversed:             config.Inversed,
	}

	if parsedCheck.name == "" {
		return nil, errNameRequired
	}

	if parsedCheck.deviceCredentialPath == "" {
		return nil, errDeviceCredPathRequired
	}

	if parsedCheck.inputValue == nil && parsedCheck.wrpCredentialPath == "" {
		return nil, errInputValueOrWRPCredPathRequired
	}

	check, errBinOp := newBinOp(config.Op)
	if errBinOp != nil {
		return nil, errBinOp
	}

	parsedCheck.assertion = check

	return parsedCheck, nil
}
