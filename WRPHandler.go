package main

import (
	"context"
	"net/http"

	gokithttp "github.com/go-kit/kit/transport/http"

	"github.com/go-kit/kit/log"
	"github.com/xmidt-org/webpa-common/device"
	"github.com/xmidt-org/webpa-common/logging"
	"github.com/xmidt-org/webpa-common/xhttp"
	"github.com/xmidt-org/wrp-go/v3/wrphttp"
)

func withDeviceAccessCheck(wrpRouterHandler wrphttp.HandlerFunc, d deviceAccess) wrphttp.HandlerFunc {
	encodeError := gokithttp.DefaultErrorEncoder

	return func(w wrphttp.ResponseWriter, r *wrphttp.Request) {
		err := d.authorizeWRP(r.Context(), &r.Entity.Message)
		if err != nil {
			encodeError(r.Context(), err, w)
			return
		}
		wrpRouterHandler(w, r)
	}
}

func wrpRouterHandler(logger log.Logger, router device.Router) wrphttp.HandlerFunc {
	if logger == nil {
		logger = logging.DefaultLogger()
	}

	if router == nil {
		panic("router is a required component")
	}

	errorLogger := logging.Error(logger)

	return func(w wrphttp.ResponseWriter, r *wrphttp.Request) {
		deviceRequest := &device.Request{
			Message:  &r.Entity.Message,
			Format:   r.Entity.Format,
			Contents: r.Entity.Bytes,
		}

		// deviceRequest carries the context through the routing infrastructure
		deviceResponse, err := router.Route(deviceRequest.WithContext(r.Context()))

		if err != nil {
			code := http.StatusGatewayTimeout
			switch err {
			case device.ErrorInvalidDeviceName:
				code = http.StatusBadRequest
			case device.ErrorDeviceNotFound:
				code = http.StatusNotFound
			case device.ErrorNonUniqueID:
				code = http.StatusBadRequest
			case device.ErrorInvalidTransactionKey:
				code = http.StatusBadRequest
			case device.ErrorTransactionAlreadyRegistered:
				code = http.StatusBadRequest
			}

			errorLogger.Log(logging.MessageKey(), "Could not process device request", logging.ErrorKey(), err, "code", code)
			w.Header().Set("X-Xmidt-Message-Error", err.Error())
			xhttp.WriteErrorf(
				w,
				code,
				"Could not process device request: %s",
				err,
			)

			return
		}

		// if deviceReponse == nil, that just means the request was not something that represented
		// the start of a transaction.  For example, events do not carry a transaction key because
		// they do not expect responses.
		if deviceResponse == nil {
			return
		}

		if len(deviceResponse.Contents) < 1 {
			_, err = xhttp.WriteError(
				w,
				http.StatusInternalServerError,
				"Transaction response had no content")

			errorLogger.Log(logging.MessageKey(), "Transaction response was empty", logging.ErrorKey(), err)

			return
		}

		_, err = w.WriteWRP(&wrphttp.Entity{
			Bytes:   deviceResponse.Contents,
			Message: *deviceResponse.Message,
			Format:  deviceResponse.Format,
		})

		if err != nil {
			errorLogger.Log(logging.MessageKey(), "Error while writing transaction response", logging.ErrorKey(), err)
		}
	}
}

func decorateRequestDecoder(decode wrphttp.Decoder) wrphttp.Decoder {
	return func(c context.Context, r *http.Request) (*wrphttp.Entity, error) {
		entity, err := decode(c, r)

		if err != nil {
			return nil, &xhttp.Error{
				Code: http.StatusBadRequest,
				Text: "Unable to decode request: " + err.Error(),
			}
		}

		return entity, err
	}
}
