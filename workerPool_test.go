// SPDX-FileCopyrightText: 2017 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
)

func testWorkerPoolTransactTransactorError(t *testing.T) {
	var (
		assert          = assert.New(t)
		logger          = zaptest.NewLogger(t)
		expectedRequest = httptest.NewRequest("POST", "/", nil)
		envelope        = outboundEnvelope{expectedRequest, func() {}}

		wp = &WorkerPool{
			logger: logger,
			transactor: func(actualRequest *http.Request) (*http.Response, error) {
				assert.Equal(expectedRequest, actualRequest)
				return nil, errors.New("expected error")
			},
		}
	)

	wp.transact(envelope)
}

func testWorkerPoolTransactHTTPSuccess(t *testing.T) {
	var (
		assert          = assert.New(t)
		logger          = zaptest.NewLogger(t)
		expectedRequest = httptest.NewRequest("POST", "/", nil)
		envelope        = outboundEnvelope{expectedRequest, func() {}}

		wp = &WorkerPool{
			logger: logger,
			transactor: func(actualRequest *http.Request) (*http.Response, error) {
				assert.Equal(expectedRequest, actualRequest)
				return &http.Response{
					Status:     "200 OK",
					StatusCode: 200,
					Body:       io.NopCloser(new(bytes.Buffer)),
				}, nil
			},
		}
	)

	wp.transact(envelope)
}

func testWorkerPoolTransactHTTPError(t *testing.T) {
	var (
		assert          = assert.New(t)
		logger          = zaptest.NewLogger(t)
		expectedRequest = httptest.NewRequest("POST", "/", nil)
		envelope        = outboundEnvelope{expectedRequest, func() {}}

		wp = &WorkerPool{
			logger: logger,
			transactor: func(actualRequest *http.Request) (*http.Response, error) {
				assert.Equal(expectedRequest, actualRequest)
				return &http.Response{
					Status:     "500 It Burns!",
					StatusCode: 500,
					Body:       io.NopCloser(new(bytes.Buffer)),
				}, nil
			},
		}
	)

	wp.transact(envelope)
}

func TestWorkerPool(t *testing.T) {
	t.Run("Transact", func(t *testing.T) {
		t.Run("TransactorError", testWorkerPoolTransactTransactorError)
		t.Run("HTTPSuccess", testWorkerPoolTransactHTTPSuccess)
		t.Run("HTTPError", testWorkerPoolTransactHTTPError)
	})
}
