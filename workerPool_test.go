/**
 * Copyright 2017 Comcast Cable Communications Management, LLC
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 */
package main

import (
	"bytes"
	"errors"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/xmidt-org/webpa-common/logging"
)

func testWorkerPoolTransactTransactorError(t *testing.T) {
	var (
		assert          = assert.New(t)
		logger          = logging.NewTestLogger(nil, t)
		expectedRequest = httptest.NewRequest("POST", "/", nil)
		envelope        = outboundEnvelope{expectedRequest, func() {}}

		wp = &WorkerPool{
			errorLog: logging.Error(logger),
			debugLog: logging.Debug(logger),
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
		logger          = logging.NewTestLogger(nil, t)
		expectedRequest = httptest.NewRequest("POST", "/", nil)
		envelope        = outboundEnvelope{expectedRequest, func() {}}

		wp = &WorkerPool{
			errorLog: logging.Error(logger),
			debugLog: logging.Debug(logger),
			transactor: func(actualRequest *http.Request) (*http.Response, error) {
				assert.Equal(expectedRequest, actualRequest)
				return &http.Response{
					Status:     "200 OK",
					StatusCode: 200,
					Body:       ioutil.NopCloser(new(bytes.Buffer)),
				}, nil
			},
		}
	)

	wp.transact(envelope)
}

func testWorkerPoolTransactHTTPError(t *testing.T) {
	var (
		assert          = assert.New(t)
		logger          = logging.NewTestLogger(nil, t)
		expectedRequest = httptest.NewRequest("POST", "/", nil)
		envelope        = outboundEnvelope{expectedRequest, func() {}}

		wp = &WorkerPool{
			errorLog: logging.Error(logger),
			debugLog: logging.Debug(logger),
			transactor: func(actualRequest *http.Request) (*http.Response, error) {
				assert.Equal(expectedRequest, actualRequest)
				return &http.Response{
					Status:     "500 It Burns!",
					StatusCode: 500,
					Body:       ioutil.NopCloser(new(bytes.Buffer)),
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
