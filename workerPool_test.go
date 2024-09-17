// SPDX-FileCopyrightText: 2017 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func testWorkerPoolTransactHTTPSuccess(t *testing.T) {
	var (
		b       bytes.Buffer
		assert  = assert.New(t)
		require = require.New(t)
		logger  = zap.New(
			zapcore.NewCore(zapcore.NewJSONEncoder(
				zapcore.EncoderConfig{
					MessageKey: "message",
				}), zapcore.AddSync(&b), zapcore.ErrorLevel),
		)
		target          = "http://localhost/foo"
		expectedRequest = httptest.NewRequest("POST", target, nil).WithContext(context.WithValue(context.Background(), eventTypeContextKey{}, EventPrefix))
		envelope        = outboundEnvelope{expectedRequest, func() {}}
		dm              = new(mockCounter)
		wp              = &WorkerPool{
			logger: logger,
			transactor: func(actualRequest *http.Request) (*http.Response, error) {
				assert.Equal(expectedRequest, actualRequest)
				return &http.Response{
					Status:     "202 Accepted",
					StatusCode: http.StatusAccepted,
					Body:       io.NopCloser(new(bytes.Buffer)),
					Request:    httptest.NewRequest("POST", target, nil),
				}, nil
			},
			droppedMessages: dm,
		}
	)

	dm.On("With", prometheus.Labels{eventLabel: EventPrefix, codeLabel: strconv.Itoa(http.StatusAccepted), reasonLabel: non202CodeReason, urlLabel: target}).Panic("Func dm.With should have not been called")
	dm.On("Add", 1.).Panic("Func dm.Add should have not been called")
	require.NotPanics(func() { wp.transact(envelope) })
	assert.Equal(b.Len(), 0)
}

func testWorkerPoolTransactHTTPError(t *testing.T) {
	var (
		assert          = assert.New(t)
		target          = "http://localhost/foo"
		ctx             = context.WithValue(context.Background(), eventTypeContextKey{}, EventPrefix)
		expectedRequest = httptest.NewRequest("POST", target, nil).WithContext(context.WithValue(ctx, trustClaimKey{}, 1000))
		envelope        = outboundEnvelope{expectedRequest, func() {}}
		tests           = []struct {
			description    string
			wp             *WorkerPool
			expectedCode   int
			expectedReason string
		}{
			{
				description: "failure 500",
				wp: &WorkerPool{
					transactor: func(actualRequest *http.Request) (*http.Response, error) {
						assert.Equal(expectedRequest, actualRequest)
						return &http.Response{
							Status:     "500 It Burns!",
							StatusCode: http.StatusInternalServerError,
							Body:       io.NopCloser(new(bytes.Buffer)),
							Request:    httptest.NewRequest("POST", target, nil),
						}, nil
					},
				},
				expectedCode:   http.StatusInternalServerError,
				expectedReason: non202CodeReason,
			},
			{
				description: "failure 415, caduceus /notify response case",
				wp: &WorkerPool{
					transactor: func(actualRequest *http.Request) (*http.Response, error) {
						assert.Equal(expectedRequest, actualRequest)
						return &http.Response{
							Status:     "415",
							StatusCode: http.StatusUnsupportedMediaType,
							Body:       io.NopCloser(new(bytes.Buffer)),
							Request:    httptest.NewRequest("POST", target, nil),
						}, nil
					},
				},
				expectedCode:   http.StatusUnsupportedMediaType,
				expectedReason: non202CodeReason,
			},
			{
				description: "failure 503, caduceus /notify response case",
				wp: &WorkerPool{
					transactor: func(actualRequest *http.Request) (*http.Response, error) {
						assert.Equal(expectedRequest, actualRequest)
						return &http.Response{
							Status:     "503",
							StatusCode: http.StatusServiceUnavailable,
							Body:       io.NopCloser(new(bytes.Buffer)),
							Request:    httptest.NewRequest("POST", target, nil),
						}, nil
					},
				},
				expectedCode:   http.StatusServiceUnavailable,
				expectedReason: non202CodeReason,
			},
			{
				description: "failure 400, caduceus /notify response case",
				wp: &WorkerPool{
					transactor: func(actualRequest *http.Request) (*http.Response, error) {
						assert.Equal(expectedRequest, actualRequest)
						return &http.Response{
							Status:     "400",
							StatusCode: http.StatusBadRequest,
							Body:       io.NopCloser(new(bytes.Buffer)),
							Request:    httptest.NewRequest("POST", target, nil),
						}, nil
					},
				},
				expectedCode:   http.StatusBadRequest,
				expectedReason: non202CodeReason,
			},
			{
				description: "failure 408 timeout, caduceus /notify response case",
				wp: &WorkerPool{
					transactor: func(actualRequest *http.Request) (*http.Response, error) {
						assert.Equal(expectedRequest, actualRequest)
						return &http.Response{
							Status:     "408",
							StatusCode: http.StatusRequestTimeout,
							Body:       io.NopCloser(new(bytes.Buffer)),
							Request:    httptest.NewRequest("POST", target, nil),
						}, context.DeadlineExceeded
					},
				},
				expectedCode:   http.StatusRequestTimeout,
				expectedReason: deadlineExceededReason,
			},
		}
	)

	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {
			var b bytes.Buffer
			tc.wp.logger = zap.New(
				zapcore.NewCore(zapcore.NewJSONEncoder(
					zapcore.EncoderConfig{
						MessageKey: "message",
					}), zapcore.AddSync(&b), zapcore.WarnLevel),
			)
			dm := new(mockCounter)
			tc.wp.droppedMessages = dm
			dm.On("With", prometheus.Labels{eventLabel: EventPrefix, codeLabel: strconv.Itoa(tc.expectedCode), reasonLabel: tc.expectedReason, urlLabel: target}).Return().Once()
			dm.On("Add", 1.).Return().Once()
			tc.wp.transact(envelope)
			assert.Greater(b.Len(), 0)
			dm.AssertExpectations(t)
		})
	}
}

func TestWorkerPool(t *testing.T) {
	// TODO improve tests by included 0 trust cases
	t.Run("Transact", func(t *testing.T) {
		t.Run("HTTPSuccess", testWorkerPoolTransactHTTPSuccess)
		t.Run("HTTPError", testWorkerPoolTransactHTTPError)
	})
}
