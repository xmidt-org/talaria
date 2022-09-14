package main

import (
	"context"
	"net/http"

	"github.com/fatih/structs"
	// nolint:staticcheck
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/metrics"
	"github.com/thedevsaddam/gojsonq/v2"
	"github.com/xmidt-org/webpa-common/v2/device"

	// nolint:staticcheck
	"github.com/xmidt-org/webpa-common/v2/logging"
	"github.com/xmidt-org/webpa-common/v2/xhttp"
	"github.com/xmidt-org/wrp-go/v3"
)

// HTTP response aware errors
var (
	errWRPCredentialsMissing   = &xhttp.Error{Code: http.StatusForbidden, Text: "Missing WRP credential"}
	errDeviceCredentialMissing = &xhttp.Error{Code: http.StatusForbidden, Text: "Missing device metadata credential"}
	errInvalidWRPDestination   = &xhttp.Error{Code: http.StatusBadRequest, Text: "Invalid WRP Destination"}
	errDeviceNotFound          = &xhttp.Error{Code: http.StatusNotFound, Text: "Device not found"}
	errIncompleteCheck         = &xhttp.Error{Code: http.StatusForbidden, Text: "Check incomplete"}
	errDeniedDeviceAccess      = &xhttp.Error{Code: http.StatusForbidden, Text: "Denied Access to Device"}
)

// deviceAccessCheck describes a single unit of assertion check against a
// device's credentials.
type deviceAccessCheck struct {
	// Name provides a short description of the check.
	Name string

	// DeviceCredentialPath is the Sep-delimited path to the credential value
	// associated with the device.
	DeviceCredentialPath string

	// WRPCredentialPath is the Sep-delimited path to the credential value
	// presented by API users attempting to contact a device.
	// (Optional when RawValue is specified. If both present, DeviceCredentialExpected is preferred).
	WRPCredentialPath string

	// InputValue provides a way to assert on the specific values pointed by DeviceCredentialPath.
	// (Optional when WRPCredential is specified).
	InputValue interface{}

	// Op is the string describing the operation that should be run for this
	// check (i.e. contains).
	Op string

	// Inversed should be set to true if Op should be applied from
	// valueAt(DeviceCredentialPath) to (either DeviceCredentialExpected or valueAt(WRPCredentialPath))
	// (Optional).
	Inversed bool
}

// deviceAccessCheckConfig drives the device access business logic.
type deviceAccessCheckConfig struct {
	// Type can either be "enforce" or "monitor" and refers to the
	// whether or not this check is in strict mode.
	Type string

	// Sep is the separator to be used to split the keys from the given paths.
	// (Optional. Defaults to '.').
	Sep string

	// Checks is the list of checks that will be run against inbound WRP messages.
	Checks []deviceAccessCheck
}

type deviceAccess interface {
	authorizeWRP(context.Context, *wrp.Message) error
}

type talariaDeviceAccess struct {
	strict             bool
	wrpMessagesCounter metrics.Counter
	deviceRegistry     device.Registry
	checks             []*parsedCheck
	sep                string
	debugLogger        log.Logger
}

func (t *talariaDeviceAccess) withFailure(labelValues ...string) metrics.Counter {
	if !t.strict {
		return t.withSuccess(labelValues...)
	}
	return t.wrpMessagesCounter.With(append(labelValues, outcomeLabel, rejected)...)
}

func (t *talariaDeviceAccess) withFatal(labelValues ...string) metrics.Counter {
	return t.wrpMessagesCounter.With(append(labelValues, outcomeLabel, rejected)...)
}

func (t *talariaDeviceAccess) withSuccess(labelValues ...string) metrics.Counter {
	return t.wrpMessagesCounter.With(append(labelValues, outcomeLabel, accepted)...)
}

func getRight(check *parsedCheck, wrpCredentials *gojsonq.JSONQ) interface{} {
	if check.inputValue != nil {
		return check.inputValue
	}

	return wrpCredentials.Reset().Find(check.wrpCredentialPath)
}

// authorizeWRP returns true if the talaria partners access policy checks succeed. Otherwise, false
// alongside an appropriate error that's friendly to go-kit's HTTP error response encoder.
func (t *talariaDeviceAccess) authorizeWRP(_ context.Context, message *wrp.Message) error {
	ID, err := device.ParseID(message.Destination)
	if err != nil {
		t.withFatal(reasonLabel, invalidWRPDest).Add(1)
		return errInvalidWRPDestination
	}

	d, ok := t.deviceRegistry.Get(ID)
	if !ok {
		t.withFatal(reasonLabel, deviceNotFound).Add(1)
		return errDeviceNotFound
	}
	deviceCredentials := gojsonq.New(gojsonq.WithSeparator(t.sep)).FromInterface(d.Metadata().Claims())
	wrpCredentials := gojsonq.New(gojsonq.WithSeparator(t.sep)).FromInterface(structs.Map(message))

	for _, c := range t.checks {
		left := deviceCredentials.Reset().Find(c.deviceCredentialPath)

		if left == nil {
			t.withFailure(reasonLabel, missingDeviceCredential).Add(1)
			if t.strict {
				return errDeviceCredentialMissing
			}
			return nil
		}

		right := getRight(c, wrpCredentials)
		if right == nil {
			t.withFailure(reasonLabel, missingWRPCredential).Add(1)
			if t.strict {
				return errWRPCredentialsMissing
			}
			return nil
		}

		if c.inversed {
			left, right = right, left
		}

		t.debugLogger.Log(
			logging.MessageKey(), "Performing check with operation applied from left to right",
			"check", c.name,
			"left", left,
			"operation", c.assertion.name(),
			"right", right)

		ok, err := c.assertion.evaluate(left, right)
		if err != nil {
			t.debugLogger.Log(logging.MessageKey(), "Check failed to complete", "check", c.name, logging.ErrorKey(), err)
			t.withFailure(reasonLabel, incompleteCheck).Add(1)

			if t.strict {
				return errIncompleteCheck
			}
			return nil
		}

		if !ok {
			t.debugLogger.Log(logging.MessageKey(), "WRP is unauthorized to reach device", "check", c.name)
			t.withFailure(reasonLabel, denied).Add(1)

			if t.strict {
				return errDeniedDeviceAccess
			}

			return nil
		}
	}

	t.withSuccess(reasonLabel, authorized).Add(1)
	return nil
}
