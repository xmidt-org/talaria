// SPDX-FileCopyrightText: 2017 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"net/http"

	"go.uber.org/zap"

	gokithttp "github.com/go-kit/kit/transport/http"

	"github.com/xmidt-org/webpa-common/v2/adapter"
	"github.com/xmidt-org/webpa-common/v2/device"

	// nolint:staticcheck

	"github.com/xmidt-org/webpa-common/v2/xhttp"
	"github.com/xmidt-org/wrp-go/v3/wrphttp"
)

func withDeviceAccessCheck(errorLogger *zap.Logger, wrpRouterHandler wrphttp.HandlerFunc, d deviceAccess) wrphttp.HandlerFunc {
	encodeError := talariaWRPErrorEncoder(errorLogger)

	return func(w wrphttp.ResponseWriter, r *wrphttp.Request) {
		err := d.authorizeWRP(r.Context(), &r.Entity.Message)
		if err != nil {
			encodeError(r.Context(), err, w)
			return
		}
		wrpRouterHandler(w, r)
	}
}

func wrpRouterHandler(logger *zap.Logger, router device.Router, ctxlogger func(ctx context.Context) *zap.Logger) wrphttp.HandlerFunc {
	if logger == nil {
		log := adapter.DefaultLogger()
		logger = log.Logger
	}

	if router == nil {
		panic("router is a required component")
	}

	return func(w wrphttp.ResponseWriter, r *wrphttp.Request) {
		// HOTFIX: only support basic auth for now, so as to avoid allowing themis
		// to issue JWTs for this endpoint.
		if _, _, ok := r.Original.BasicAuth(); !ok {
			logger.Error("only basic auth is supported")
			w.WriteHeader(http.StatusForbidden)
			return
		}

		deviceRequest := &device.Request{
			Message:  &r.Entity.Message,
			Format:   r.Entity.Format,
			Contents: r.Entity.Bytes,
		}
		var errorLogger *zap.Logger
		if ctxlogger != nil && ctxlogger(r.Context()) != nil {
			errorLogger = ctxlogger(r.Context())
		} else {
			errorLogger = logger
		}

		// deviceRequest carries the context through the routing infrastructure
		deviceResponse, err := router.Route(deviceRequest.WithContext(r.Context()))

		if err != nil {
			code := http.StatusGatewayTimeout
			// nolint:errorlint
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

			errorLogger.Error("Could not process device request", zap.Error(err), zap.Int("code", code))
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

			errorLogger.Error("Transaction response was empty", zap.Error(err))

			return
		}

		_, err = w.WriteWRP(&wrphttp.Entity{
			Bytes:   deviceResponse.Contents,
			Message: *deviceResponse.Message,
			Format:  deviceResponse.Format,
		})

		if err != nil {
			errorLogger.Error("Error while writing transaction response", zap.Error(err))
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

func talariaWRPErrorEncoder(errorLogger *zap.Logger) gokithttp.ErrorEncoder {
	return func(_ context.Context, err error, w http.ResponseWriter) {
		code := http.StatusInternalServerError
		// nolint errorlint
		if sc, ok := err.(gokithttp.StatusCoder); ok {
			code = sc.StatusCode()
		}
		if code == http.StatusInternalServerError {
			errorLogger.Error("Possibly false internal server error", zap.Error(err))
		}
		w.WriteHeader(code)
	}
}
