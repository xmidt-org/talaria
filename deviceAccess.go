package main

import (
	"context"
	"net/http"

	"github.com/fatih/structs"
	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/metrics"
	"github.com/xmidt-org/bascule"
	"github.com/xmidt-org/webpa-common/device"
	"github.com/xmidt-org/webpa-common/logging"
	"github.com/xmidt-org/webpa-common/xhttp"
	"github.com/xmidt-org/wrp-go/v3"
)

// HTTP response aware errors
var (
	ErrWRPCredentialsMissing   = &xhttp.Error{Code: http.StatusForbidden, Text: "Missing WRP credential"}
	ErrDeviceCredentialMissing = &xhttp.Error{Code: http.StatusForbidden, Text: "Missing device metadata credential"}
	ErrInvalidWRPDestination   = &xhttp.Error{Code: http.StatusBadRequest, Text: "Invalid WRP Destination"}
	ErrDeviceNotFound          = &xhttp.Error{Code: http.StatusNotFound, Text: "Device not found"}
	ErrIncompleteCheck         = &xhttp.Error{Code: http.StatusForbidden, Text: "Check incomplete"}
	ErrDeniedDeviceAccess      = &xhttp.Error{Code: http.StatusForbidden, Text: "Denied Access to Device"}
)
var (
	missingDeviceCredentialsLabelPair = []string{ReasonLabel, MissingDeviceCredential}
	missingWRPCredentialsLabelPair    = []string{ReasonLabel, MissingWRPCredential}
)

// deviceAccessCheck describes a single unit of assertion check against a
// device's credentials.
type deviceAccessCheck struct {
	// Name provides a short description of the check
	Name string

	// WRPCredentialPath is the Sep-delimited path to the credential value
	// presented by API users attempting to contact a device.
	// (Optional when RawValue is specified. If both present, DeviceCredentialExpected is preferred).
	WRPCredentialPath string

	// InputValue provides a way to assert on the specific values pointed by DeviceCredentialPath.
	// (Optional when WRPCredential is specified).
	InputValue interface{}

	//Op is the string describing the operation that should be run for this
	//check (i.e. contains)
	Op string

	//DeviceCredentialPath is the Sep-delimited path to the credential value
	//associated with the device.
	DeviceCredentialPath string

	//Inversed should be set to true if Op should be applied from
	//valueAt(DeviceCredentialPath) to (either DeviceCredentialExpected or valueAt(WRPCredentialPath))
	//(Optional)
	Inversed bool
}

//deviceAccessCheckConfig drives the device access business logic.
type deviceAccessCheckConfig struct {
	Type string

	//Sep is the separator to be used to split the keys from the given paths.
	//(Optional. Defaults to '.')
	Sep string

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
	return t.wrpMessagesCounter.With(append(labelValues, OutcomeLabel, Rejected)...)
}

func (t *talariaDeviceAccess) withFatal(labelValues ...string) metrics.Counter {
	return t.wrpMessagesCounter.With(append(labelValues, OutcomeLabel, Rejected)...)
}

func (t *talariaDeviceAccess) withSuccess(labelValues ...string) metrics.Counter {
	return t.wrpMessagesCounter.With(append(labelValues, OutcomeLabel, Accepted)...)
}

func getRight(check *parsedCheck, wrpCredentials bascule.Attributes) (interface{}, bool) {
	if check.inputValue != nil {
		return check.inputValue, true
	}
	return wrpCredentials.Get(check.wrpCredentialPath)
}

// authorizeWRP returns true if the talaria partners access policy checks succeed. Otherwise, false
// alongside an appropiate error that's friendly to go-kit's HTTP error response encoder.
func (t *talariaDeviceAccess) authorizeWRP(_ context.Context, message *wrp.Message) error {
	ID, err := device.ParseID(message.Destination)
	if err != nil {
		t.withFatal(ReasonLabel, InvalidWRPDest).Add(1)
		return ErrInvalidWRPDestination
	}

	d, ok := t.deviceRegistry.Get(ID)
	if !ok {
		t.withFatal(ReasonLabel, DeviceNotFound).Add(1)
		return ErrDeviceNotFound
	}

	deviceCredentials := bascule.NewAttributesWithOptions(
		bascule.AttributesOptions{
			KeyDelimiter:  t.sep,
			AttributesMap: d.Metadata().JWTClaims().Data(),
		})

	wrpCredentials := bascule.NewAttributesWithOptions(
		bascule.AttributesOptions{
			KeyDelimiter:  t.sep,
			AttributesMap: structs.Map(message),
		})

	for _, c := range t.checks {
		left, ok := deviceCredentials.Get(c.deviceCredentialPath)
		if !ok {
			t.withFailure(missingDeviceCredentialsLabelPair...).Add(1)
			if t.strict {
				return ErrDeviceCredentialMissing
			}
			return nil
		}

		right, ok := getRight(c, wrpCredentials)
		if !ok {
			t.withFailure(missingWRPCredentialsLabelPair...).Add(1)
			if t.strict {
				return ErrWRPCredentialsMissing
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
			"operation", c.assertion.Name(),
			"right", right)

		ok, err := c.assertion.Evaluate(left, right)
		if err != nil {
			t.debugLogger.Log(logging.MessageKey(), "Check failed to complete", "check", c.name, logging.ErrorKey(), err)
			t.withFailure(ReasonLabel, IncompleteCheck).Add(1)

			if t.strict {
				return ErrIncompleteCheck
			}
			return nil
		}

		if !ok {
			t.debugLogger.Log(logging.MessageKey(), "WRP is unauthorized to reach device", "check", c.name)
			t.withFailure(ReasonLabel, Denied).Add(1)

			if t.strict {
				return ErrDeniedDeviceAccess
			}

			return nil
		}
	}

	t.withSuccess(ReasonLabel, Authorized).Add(1)
	return nil
}
