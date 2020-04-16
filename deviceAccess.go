package main

import (
	"context"
	"fmt"
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
	ErrTokenMissing           = &xhttp.Error{Code: http.StatusInternalServerError, Text: "No JWT Token was found in context"}
	ErrTokenTypeMismatch      = &xhttp.Error{Code: http.StatusInternalServerError, Text: "Token must be a JWT"}
	ErrPIDMissing             = &xhttp.Error{Code: http.StatusBadRequest, Text: "WRP PartnerIDs field must not be empty"}
	ErrInvalidAllowedPartners = &xhttp.Error{Code: http.StatusForbidden, Text: "AllowedPartners JWT claim must be a non-empty list of strings"}
	ErrPIDMismatch            = &xhttp.Error{Code: http.StatusForbidden, Text: "Unauthorized partner credentials in WRP message"}

	ErrCredentialsMissing    = &xhttp.Error{Code: http.StatusForbidden, Text: "Could not find credentials to compare"}
	ErrInvalidWRPDestination = &xhttp.Error{Code: http.StatusBadRequest, Text: "Invalid WRP Destination"}
	ErrDeviceNotFound        = &xhttp.Error{Code: http.StatusNotFound, Text: "Device not found"}
)

func errDeviceAccessUnauthorized(checkName string) error {
	return &xhttp.Error{Code: http.StatusForbidden, Text: fmt.Sprintf("Unauthorized device access. Check '%s' failed.", checkName)}
}

func errDeviceAccessIncompleteCheck(checkName string) error {
	return &xhttp.Error{Code: http.StatusForbidden, Text: fmt.Sprintf("Device access check '%s' could not complete", checkName)}
}

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
	strict                  bool
	receivedWRPMessageCount metrics.Counter
	deviceRegistry          device.Registry
	checks                  []parsedCheck
	debugLogger             log.Logger
}

func (t *talariaDeviceAccess) withFailure(labelValues ...string) metrics.Counter {
	if !t.strict {
		return t.withSuccess(labelValues...)
	}
	return t.receivedWRPMessageCount.With(append(labelValues, OutcomeLabel, Rejected)...)
}

func (t *talariaDeviceAccess) withSuccess(labelValues ...string) metrics.Counter {
	return t.receivedWRPMessageCount.With(append(labelValues, OutcomeLabel, Accepted)...)
}

func getRight(check parsedCheck, wrpCredentials bascule.Attributes) (interface{}, bool) {
	if check.inputValue != nil {
		return check.inputValue, true
	}
	return wrpCredentials.Get(check.wrpCredentialPath)
}

// authorizeWRP returns true if the talaria partners access policy checks succeed. Otherwise, false
// alongside an appropiate error that's friendly to go-kit's HTTP error response encoder.
// TODO: modify metrics accordingly
func (t *talariaDeviceAccess) authorizeWRP(ctx context.Context, message *wrp.Message) error {
	ID, err := device.ParseID(message.Destination)
	if err != nil {
		return ErrInvalidWRPDestination
	}

	d, ok := t.deviceRegistry.Get(ID)
	if !ok {
		return ErrDeviceNotFound
	}

	deviceCredentials := bascule.NewAttributesFromMap(d.Metadata().JWTClaims().Data())
	wrpCredentials := bascule.NewAttributesFromMap(structs.Map(message))

	for _, c := range t.checks {
		left, ok := deviceCredentials.Get(c.deviceCredentialPath)

		if !ok {
			return ErrCredentialsMissing
		}

		right, ok := getRight(c, wrpCredentials)
		if !ok {
			return ErrCredentialsMissing
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
			return errDeviceAccessIncompleteCheck(c.name)
		}
		if !ok {
			return errDeviceAccessUnauthorized(c.name)
		}
	}

	return nil
}
