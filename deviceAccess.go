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
	ErrTokenMissing           = &xhttp.Error{Code: http.StatusInternalServerError, Text: "No JWT Token was found in context"}
	ErrTokenTypeMismatch      = &xhttp.Error{Code: http.StatusInternalServerError, Text: "Token must be a JWT"}
	ErrPIDMissing             = &xhttp.Error{Code: http.StatusBadRequest, Text: "WRP PartnerIDs field must not be empty"}
	ErrInvalidAllowedPartners = &xhttp.Error{Code: http.StatusForbidden, Text: "AllowedPartners JWT claim must be a non-empty list of strings"}
	ErrPIDMismatch            = &xhttp.Error{Code: http.StatusForbidden, Text: "Unauthorized partner credentials in WRP message"}

	ErrCredentialsMissing          = &xhttp.Error{Code: http.StatusForbidden, Text: "Could not find credentials to compare"}
	ErrInvalidWRPDestination       = &xhttp.Error{Code: http.StatusBadRequest, Text: "Invalid WRP Destination"}
	ErrDeviceNotFound              = &xhttp.Error{Code: http.StatusNotFound, Text: "Device not found"}
	ErrDeviceAccessUnauthorized    = &xhttp.Error{Code: http.StatusForbidden, Text: "Unauthorized device access"}
	ErrDeviceAccessIncompleteCheck = &xhttp.Error{Code: http.StatusForbidden, Text: "Device access check could not complete"}
)

// deviceAccessCheck describes a single unit of assertion check between
// values presented by API users against those of the device.
type deviceAccessCheck struct {
	Name string
	//UserCredentialPath is the Sep-delimited path to the credential value
	//presented by API users attempting to contact a device.
	WRPCredentialPath string

	//Op is the string describing the operation that should be run for this
	//check (i.e. contains).
	Operation string

	//DeviceCredentialPath is the Sep-delimited path to the credential value
	//associated with the device.
	DeviceCredentialPath string

	//Inversed should be set to true if Op should be applied from
	//valueAt(DeviceCredentialPath) to valueAt(UserCredentialPath)
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
		this, thisOk := deviceCredentials.Get(c.deviceCredentialPath)
		that, thatOk := wrpCredentials.Get(c.wrpCredentialPath)
		if !thisOk || !thatOk {
			return ErrCredentialsMissing
		}

		if c.inversed {
			this, that = that, this
		}

		t.debugLogger.Log(logging.MessageKey(), "Performing operation applied from this to that", "this", this, "that", that)

		ok, err := c.assertion.Run(this, that)
		if err != nil {
			t.debugLogger.Log(logging.MessageKey(), "Check failed to complete", logging.ErrorKey(), err)
			return ErrDeviceAccessIncompleteCheck
		}
		if !ok {
			return ErrDeviceAccessUnauthorized
		}
	}

	return nil
}
