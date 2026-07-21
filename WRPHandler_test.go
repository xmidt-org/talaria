// SPDX-FileCopyrightText: 2017 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/xmidt-org/webpa-common/v2/device"
	"go.uber.org/zap/zaptest"

	// nolint:staticcheck
	"github.com/xmidt-org/webpa-common/v2/xhttp"
	"github.com/xmidt-org/wrp-go/v3"
	"github.com/xmidt-org/wrp-go/v3/wrphttp"
)

const (
	testHeader1        = "Header-1"
	testHeader2        = "Header-2"
	testMetadataBar    = "bar"
	testMAC            = "mac:123412341234"
	testMetadataFoo    = "foo"
	testWRPSource      = "test.com"
	testWRPContentType = "text/plain"
)

func testWRPHandlerNilRouter(t *testing.T) {
	assert := assert.New(t)
	assert.Panics(func() {
		wrpRouterHandler(nil, nil, nil, InboundMeasures{})
	})
}

func testWithDeviceAccessCheck(t *testing.T, authorized bool) {
	var (
		assert   = assert.New(t)
		d        = new(mockDeviceAccess)
		recorder = httptest.NewRecorder()
		w        = newTestWRPResponseWriter(recorder)
		r        = &wrphttp.Request{
			Entity: &wrphttp.Entity{
				Message: wrp.Message{},
			},
		}
		wrpRouterHandler = func(w wrphttp.ResponseWriter, _ *wrphttp.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("success"))
		}
	)

	if authorized {
		d.On("authorizeWRP", r.Context(), &r.Entity.Message).Return(error(nil))
	} else {
		d.On("authorizeWRP", r.Context(), &r.Entity.Message).Return(&xhttp.Error{Code: http.StatusForbidden, Text: "Error body"})
	}

	wrpRouterHandler = withDeviceAccessCheck(zaptest.NewLogger(t), wrpRouterHandler, d)
	wrpRouterHandler(w, r)

	if authorized {
		assert.NotEmpty(recorder.Body.Bytes())
		assert.Equal(http.StatusOK, recorder.Code)
	} else {
		assert.Empty(recorder.Body.Bytes())
		assert.Equal(http.StatusForbidden, recorder.Code)
	}
}

func testMessageHandlerServeHTTPEncodeError(t *testing.T) {
	const transactionKey = "transaction-key"

	var (
		assert  = assert.New(t)
		require = require.New(t)

		requestMessage = &wrp.Message{
			Type:            wrp.SimpleRequestResponseMessageType,
			Source:          testWRPSource,
			Destination:     testMAC,
			TransactionUUID: transactionKey,
			ContentType:     testWRPContentType,
			Payload:         []byte("eyJjb21tYW5kIjoiR0VUIiwibmFtZXMiOlsiU29tZXRoaW5nIl19"),
			Headers:         []string{testHeader1, testHeader2},
			Metadata:        map[string]string{testMetadataFoo: testMetadataBar},
		}

		responseMessage = &wrp.Message{
			Type:            wrp.SimpleRequestResponseMessageType,
			Destination:     testWRPSource,
			Source:          testMAC,
			TransactionUUID: transactionKey,
		}

		requestContents []byte
	)

	require.NoError(wrp.NewEncoderBytes(&requestContents, wrp.Msgpack).Encode(requestMessage))

	var (
		response = httptest.NewRecorder()
		request  = httptest.NewRequest("POST", "/foo", bytes.NewReader(requestContents))

		router  = new(mockRouter)
		d       = new(device.MockDevice)
		handler = wrphttp.NewHTTPHandler(wrpRouterHandler(nil, router, nil, InboundMeasures{}), wrphttp.WithDecoder(decorateRequestDecoder(wrphttp.DefaultDecoder())))

		actualResponseBody     map[string]interface{}
		expectedDeviceResponse = &device.Response{
			Device:  d,
			Message: responseMessage,
			Format:  wrp.Msgpack,
		}
	)

	router.On(
		"Route",
		mock.MatchedBy(func(candidate *device.Request) bool {
			return candidate.Message != nil &&
				len(candidate.Contents) > 0 &&
				candidate.Format == wrp.Msgpack
		}),
	).Once().Return(expectedDeviceResponse, nil)

	request.SetBasicAuth("test", "test")
	handler.ServeHTTP(response, request)
	assert.Equal(http.StatusInternalServerError, response.Code)
	assert.Equal("application/json", response.Header().Get("Content-Type"))
	responseContents, err := io.ReadAll(response.Body)
	require.NoError(err)
	assert.NoError(json.Unmarshal(responseContents, &actualResponseBody))

	router.AssertExpectations(t)
	d.AssertExpectations(t)
}

func testMessageHandlerServeHTTPRouteError(t *testing.T, routeError error, expectedCode int) {
	var (
		assert  = assert.New(t)
		require = require.New(t)

		message = &wrp.Message{
			Type:        wrp.SimpleEventMessageType,
			Source:      testWRPSource,
			Destination: testMAC,
		}

		requestContents []byte
	)

	require.NoError(wrp.NewEncoderBytes(&requestContents, wrp.Msgpack).Encode(message))

	var (
		response           = httptest.NewRecorder()
		request            = httptest.NewRequest("POST", "/foo", bytes.NewReader(requestContents))
		actualResponseBody map[string]interface{}

		router  = new(mockRouter)
		handler = wrphttp.NewHTTPHandler(wrpRouterHandler(nil, router, nil, InboundMeasures{}), wrphttp.WithDecoder(decorateRequestDecoder(wrphttp.DefaultDecoder())))
	)

	router.On(
		"Route",
		mock.MatchedBy(func(candidate *device.Request) bool {
			fmt.Printf("candidate.Message: %v\n", candidate.Message)
			fmt.Printf("candidate.Contents: %v\n", candidate.Contents)
			fmt.Printf("candidate.Format: %v\n", candidate.Format)
			return candidate.Message != nil &&
				len(candidate.Contents) > 0 &&
				candidate.Format == wrp.Msgpack
		}),
	).Once().Return(nil, routeError)

	request.SetBasicAuth("test", "test")
	handler.ServeHTTP(response, request)
	assert.Equal(expectedCode, response.Code)
	assert.Equal("application/json", response.Header().Get("Content-Type"))
	responseContents, err := io.ReadAll(response.Body)
	require.NoError(err)
	assert.NoError(json.Unmarshal(responseContents, &actualResponseBody))

	router.AssertExpectations(t)
}

func testMessageHandlerServeHTTPEvent(t *testing.T, requestFormat wrp.Format) {
	var (
		assert  = assert.New(t)
		require = require.New(t)

		event = &wrp.Message{
			Source:      testWRPSource,
			Destination: testMAC,
			ContentType: testWRPContentType,
			Payload:     []byte("eyJjb21tYW5kIjoiR0VUIiwibmFtZXMiOlsiU29tZXRoaW5nIl19"),
			Headers:     []string{testHeader1, testHeader2},
			Metadata:    map[string]string{testMetadataFoo: testMetadataBar},
		}

		requestContents []byte
	)

	require.NoError(wrp.NewEncoderBytes(&requestContents, requestFormat).Encode(event))

	var (
		response = httptest.NewRecorder()
		request  = httptest.NewRequest("POST", "/foo", bytes.NewReader(requestContents))

		router  = new(mockRouter)
		handler = wrphttp.NewHTTPHandler(wrpRouterHandler(zaptest.NewLogger(t), router, nil, InboundMeasures{}), wrphttp.WithDecoder(decorateRequestDecoder(wrphttp.DefaultDecoder())))

		actualDeviceRequest *device.Request
	)

	request.Header.Set("Content-Type", requestFormat.ContentType())

	router.On(
		"Route",
		mock.MatchedBy(func(candidate *device.Request) bool {
			actualDeviceRequest = candidate
			return candidate.Message != nil &&
				len(candidate.Contents) > 0 &&
				candidate.Format == requestFormat
		}),
	).Once().Return(nil, nil)

	request.SetBasicAuth("test", "test")
	handler.ServeHTTP(response, request)
	assert.Equal(http.StatusOK, response.Code)
	assert.Equal(0, response.Body.Len())
	require.NotNil(actualDeviceRequest)

	router.AssertExpectations(t)
}

func testMessageHandlerServeHTTPRequestResponse(t *testing.T, responseFormat, requestFormat wrp.Format) {
	const transactionKey = "transaction-key"

	var (
		assert  = assert.New(t)
		require = require.New(t)

		requestMessage = &wrp.Message{
			Type:            wrp.SimpleRequestResponseMessageType,
			Source:          testWRPSource,
			Destination:     testMAC,
			TransactionUUID: transactionKey,
			ContentType:     testWRPContentType,
			Payload:         []byte("eyJjb21tYW5kIjoiR0VUIiwibmFtZXMiOlsiU29tZXRoaW5nIl19"),
			Headers:         []string{testHeader1, testHeader2},
			Metadata:        map[string]string{testMetadataFoo: testMetadataBar},
		}

		responseMessage = &wrp.Message{
			Type:            wrp.SimpleRequestResponseMessageType,
			Destination:     testWRPSource,
			Source:          testMAC,
			TransactionUUID: transactionKey,
		}

		requestContents  []byte
		responseContents []byte
	)

	require.NoError(wrp.NewEncoderBytes(&requestContents, requestFormat).Encode(requestMessage))
	require.NoError(wrp.NewEncoderBytes(&responseContents, responseFormat).Encode(responseMessage))

	var (
		response = httptest.NewRecorder()
		request  = httptest.NewRequest("POST", "/foo", bytes.NewReader(requestContents))

		router  = new(mockRouter)
		d       = new(device.MockDevice)
		handler = wrphttp.NewHTTPHandler(wrpRouterHandler(zaptest.NewLogger(t), router, nil, InboundMeasures{}), wrphttp.WithDecoder(decorateRequestDecoder(wrphttp.DefaultDecoder())))

		actualDeviceRequest    *device.Request
		expectedDeviceResponse = &device.Response{
			Device:   d,
			Message:  responseMessage,
			Format:   wrp.Msgpack,
			Contents: responseContents,
		}
	)

	request.Header.Set("Content-Type", requestFormat.ContentType())
	request.Header.Set("Accept", responseFormat.ContentType())

	router.On(
		"Route",
		mock.MatchedBy(func(candidate *device.Request) bool {
			actualDeviceRequest = candidate
			return candidate.Message != nil &&
				len(candidate.Contents) > 0 &&
				candidate.Format == requestFormat
		}),
	).Once().Return(expectedDeviceResponse, nil)

	request.SetBasicAuth("test", "test")
	handler.ServeHTTP(response, request)
	assert.Equal(http.StatusOK, response.Code)
	assert.Equal(responseFormat.ContentType(), response.Header().Get("Content-Type"))
	require.NotNil(actualDeviceRequest)
	assert.NoError(wrp.NewDecoder(response.Body, responseFormat).Decode(new(wrp.Message)))

	router.AssertExpectations(t)
	d.AssertExpectations(t)
}

func testWRPRouterHandlerErrorCounter(t *testing.T, routeError error, expectedCode int) {
	t.Helper()
	var (
		assert  = assert.New(t)
		require = require.New(t)

		message = &wrp.Message{
			Type:        wrp.SimpleEventMessageType,
			Source:      testWRPSource,
			Destination: testMAC,
		}
		requestContents []byte
	)

	require.NoError(wrp.NewEncoderBytes(&requestContents, wrp.Msgpack).Encode(message))

	var (
		response = httptest.NewRecorder()
		request  = httptest.NewRequest("POST", "/foo", bytes.NewReader(requestContents))
		router   = new(mockRouter)
		counter  = new(mockCounter)
		handler  = wrphttp.NewHTTPHandler(
			wrpRouterHandler(nil, router, nil, InboundMeasures{APIRequestErrors: counter}),
			wrphttp.WithDecoder(decorateRequestDecoder(wrphttp.DefaultDecoder())),
		)
	)

	counter.On("With", prometheus.Labels{
		codeLabel:  strconv.Itoa(expectedCode),
		errorLabel: normalizeDeviceError(routeError),
	}).Return(counter).Once()

	router.On(
		"Route",
		mock.MatchedBy(func(candidate *device.Request) bool {
			return candidate.Message != nil &&
				len(candidate.Contents) > 0 &&
				candidate.Format == wrp.Msgpack
		}),
	).Once().Return(nil, routeError)

	request.SetBasicAuth("test", "test")
	handler.ServeHTTP(response, request)
	assert.Equal(expectedCode, response.Code)

	router.AssertExpectations(t)
	counter.AssertExpectations(t)
}

func testWRPRouterHandlerNoCounterOnSuccess(t *testing.T) {
	t.Helper()
	var (
		require = require.New(t)
		assert  = assert.New(t)

		event = &wrp.Message{
			Type:        wrp.SimpleEventMessageType,
			Source:      testWRPSource,
			Destination: testMAC,
		}
		requestContents []byte
	)

	require.NoError(wrp.NewEncoderBytes(&requestContents, wrp.Msgpack).Encode(event))

	var (
		response = httptest.NewRecorder()
		request  = httptest.NewRequest("POST", "/foo", bytes.NewReader(requestContents))
		router   = new(mockRouter)
		counter  = new(mockCounter)
		handler  = wrphttp.NewHTTPHandler(
			wrpRouterHandler(nil, router, nil, InboundMeasures{APIRequestErrors: counter}),
			wrphttp.WithDecoder(decorateRequestDecoder(wrphttp.DefaultDecoder())),
		)
	)

	router.On(
		"Route",
		mock.MatchedBy(func(candidate *device.Request) bool {
			return candidate.Message != nil
		}),
	).Once().Return(nil, nil)

	request.SetBasicAuth("test", "test")
	request.Header.Set("Content-Type", wrp.Msgpack.ContentType())
	handler.ServeHTTP(response, request)
	assert.Equal(http.StatusOK, response.Code)

	assert.Empty(counter.Calls)
	router.AssertExpectations(t)
}

func TestMessageHandler(t *testing.T) {
	t.Run("NilRouter", testWRPHandlerNilRouter)

	t.Run("ServeHTTP", func(t *testing.T) {
		t.Run("EncodeError", testMessageHandlerServeHTTPEncodeError)

		t.Run("RouteError", func(t *testing.T) {
			testMessageHandlerServeHTTPRouteError(t, device.ErrorInvalidDeviceName, http.StatusBadRequest)
			testMessageHandlerServeHTTPRouteError(t, device.ErrorDeviceNotFound, http.StatusNotFound)
			testMessageHandlerServeHTTPRouteError(t, device.ErrorNonUniqueID, http.StatusBadRequest)
			testMessageHandlerServeHTTPRouteError(t, device.ErrorInvalidTransactionKey, http.StatusBadRequest)
			testMessageHandlerServeHTTPRouteError(t, device.ErrorTransactionAlreadyRegistered, http.StatusBadRequest)
			testMessageHandlerServeHTTPRouteError(t, errors.New("random error"), http.StatusGatewayTimeout)
		})

		t.Run("ErrorCounter", func(t *testing.T) {
			t.Run("InvalidDeviceName", func(t *testing.T) {
				testWRPRouterHandlerErrorCounter(t, device.ErrorInvalidDeviceName, http.StatusBadRequest)
			})
			t.Run("DeviceNotFound", func(t *testing.T) {
				testWRPRouterHandlerErrorCounter(t, device.ErrorDeviceNotFound, http.StatusNotFound)
			})
			t.Run("NonUniqueID", func(t *testing.T) {
				testWRPRouterHandlerErrorCounter(t, device.ErrorNonUniqueID, http.StatusBadRequest)
			})
			t.Run("InvalidTransactionKey", func(t *testing.T) {
				testWRPRouterHandlerErrorCounter(t, device.ErrorInvalidTransactionKey, http.StatusBadRequest)
			})
			t.Run("TransactionAlreadyRegistered", func(t *testing.T) {
				testWRPRouterHandlerErrorCounter(t, device.ErrorTransactionAlreadyRegistered, http.StatusBadRequest)
			})
			t.Run("TransactionCanceled", func(t *testing.T) {
				testWRPRouterHandlerErrorCounter(t, device.ErrorTransactionCanceled, http.StatusGatewayTimeout)
			})
			t.Run("DeviceBusy", func(t *testing.T) {
				testWRPRouterHandlerErrorCounter(t, device.ErrorDeviceBusy, http.StatusGatewayTimeout)
			})
			t.Run("DeviceClosed", func(t *testing.T) {
				testWRPRouterHandlerErrorCounter(t, device.ErrorDeviceClosed, http.StatusGatewayTimeout)
			})
			t.Run("TransactionsClosed", func(t *testing.T) {
				testWRPRouterHandlerErrorCounter(t, device.ErrorTransactionsClosed, http.StatusGatewayTimeout)
			})
			t.Run("ContextCanceled", func(t *testing.T) {
				testWRPRouterHandlerErrorCounter(t, context.Canceled, http.StatusGatewayTimeout)
			})
			t.Run("ContextDeadlineExceeded", func(t *testing.T) {
				testWRPRouterHandlerErrorCounter(t, context.DeadlineExceeded, http.StatusGatewayTimeout)
			})
			t.Run("UnknownError", func(t *testing.T) {
				testWRPRouterHandlerErrorCounter(t, errors.New("random error"), http.StatusGatewayTimeout)
			})
			t.Run("NoCounterOnSuccess", testWRPRouterHandlerNoCounterOnSuccess)
		})

		t.Run("Event", func(t *testing.T) {
			for _, requestFormat := range []wrp.Format{wrp.Msgpack, wrp.JSON} {
				testMessageHandlerServeHTTPEvent(t, requestFormat)
			}
		})

		t.Run("RequestResponse", func(t *testing.T) {
			for _, responseFormat := range []wrp.Format{wrp.Msgpack, wrp.JSON} {
				for _, requestFormat := range []wrp.Format{wrp.Msgpack, wrp.JSON} {
					testMessageHandlerServeHTTPRequestResponse(t, responseFormat, requestFormat)
				}
			}
		})
	})
}

func TestNormalizeDeviceError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		// Known device sentinel errors — returned directly
		{"InvalidDeviceName", device.ErrorInvalidDeviceName, device.ErrorInvalidDeviceName.Error()},
		{"DeviceNotFound", device.ErrorDeviceNotFound, device.ErrorDeviceNotFound.Error()},
		{"NonUniqueID", device.ErrorNonUniqueID, device.ErrorNonUniqueID.Error()},
		{"InvalidTransactionKey", device.ErrorInvalidTransactionKey, device.ErrorInvalidTransactionKey.Error()},
		{"TransactionAlreadyRegistered", device.ErrorTransactionAlreadyRegistered, device.ErrorTransactionAlreadyRegistered.Error()},
		{"TransactionCanceled", device.ErrorTransactionCanceled, device.ErrorTransactionCanceled.Error()},
		{"DeviceBusy", device.ErrorDeviceBusy, device.ErrorDeviceBusy.Error()},
		{"DeviceClosed", device.ErrorDeviceClosed, device.ErrorDeviceClosed.Error()},
		{"TransactionsClosed", device.ErrorTransactionsClosed, device.ErrorTransactionsClosed.Error()},
		// Known device sentinel errors — wrapped (errors.Is must unwrap them)
		{"WrappedDeviceNotFound", fmt.Errorf("routing: %w", device.ErrorDeviceNotFound), device.ErrorDeviceNotFound.Error()},
		{"WrappedDeviceClosed", fmt.Errorf("send: %w", device.ErrorDeviceClosed), device.ErrorDeviceClosed.Error()},
		{"WrappedTransactionCanceled", fmt.Errorf("tx: %w", device.ErrorTransactionCanceled), device.ErrorTransactionCanceled.Error()},
		// Context errors
		{"ContextCanceled", context.Canceled, "context canceled"},
		{"WrappedContextCanceled", fmt.Errorf("op: %w", context.Canceled), "context canceled"},
		{"ContextDeadlineExceeded", context.DeadlineExceeded, "context deadline exceeded"},
		{"WrappedContextDeadlineExceeded", fmt.Errorf("op: %w", context.DeadlineExceeded), "context deadline exceeded"},
		// Unknown errors always map to "unknown"
		{"UnknownBareError", errors.New("something unexpected"), "unknown"},
		{"UnknownWrappedError", fmt.Errorf("wrapper: %w", errors.New("inner")), "unknown"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, normalizeDeviceError(tc.err))
		})
	}
}

func TestMessageHandlerWithDeviceAccessCheck(t *testing.T) {
	t.Run("Authorized", func(t *testing.T) {
		testWithDeviceAccessCheck(t, true)
	})

	t.Run("Denied", func(t *testing.T) {
		testWithDeviceAccessCheck(t, false)
	})
}

type testWRPResponseWriter struct {
	http.ResponseWriter
}

func (t *testWRPResponseWriter) WriteWRP(_ *wrphttp.Entity) (int, error) {
	return 0, nil
}

func (t *testWRPResponseWriter) WriteWRPBytes(_ wrp.Format, _ []byte) (int, error) {
	return 0, nil
}

func (t *testWRPResponseWriter) WRPFormat() wrp.Format {
	return wrp.Msgpack
}

func newTestWRPResponseWriter(w *httptest.ResponseRecorder) *testWRPResponseWriter {
	return &testWRPResponseWriter{
		ResponseWriter: w,
	}
}
