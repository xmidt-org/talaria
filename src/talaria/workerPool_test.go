package main

import (
	"bytes"
	"errors"
	"github.com/Comcast/webpa-common/logging"
	"github.com/stretchr/testify/assert"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
)

func testWorkerPoolTransactTransactorError(t *testing.T) {
	var (
		assert          = assert.New(t)
		expectedRequest = httptest.NewRequest("POST", "/", nil)
		envelope        = &outboundEnvelope{expectedRequest, func() {}}

		wp = &WorkerPool{
			logger: logging.DefaultLogger(),
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
		expectedRequest = httptest.NewRequest("POST", "/", nil)
		envelope        = &outboundEnvelope{expectedRequest, func() {}}

		wp = &WorkerPool{
			logger: logging.DefaultLogger(),
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
		expectedRequest = httptest.NewRequest("POST", "/", nil)
		envelope        = &outboundEnvelope{expectedRequest, func() {}}

		wp = &WorkerPool{
			logger: logging.DefaultLogger(),
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
