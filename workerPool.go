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
	"io"
	"io/ioutil"
	"net/http"
	"sync"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/metrics"
	"github.com/xmidt-org/webpa-common/v2/logging"
)

// WorkerPool describes a pool of goroutines that dispatch http.Request objects to
// a transactor function
type WorkerPool struct {
	errorLog       log.Logger
	debugLog       log.Logger
	outbounds      <-chan outboundEnvelope
	workerPoolSize uint
	queueSize      metrics.Gauge
	transactor     func(*http.Request) (*http.Response, error)

	runOnce sync.Once
}

func NewWorkerPool(om OutboundMeasures, o *Outbounder, outbounds <-chan outboundEnvelope) *WorkerPool {
	logger := o.logger()
	return &WorkerPool{
		errorLog:       logging.Error(logger),
		debugLog:       logging.Debug(logger),
		outbounds:      outbounds,
		workerPoolSize: o.workerPoolSize(),
		queueSize:      om.QueueSize,
		transactor: (&http.Client{
			Transport: NewOutboundRoundTripper(om, o),
			Timeout:   o.clientTimeout(),
		}).Do,
	}
}

// Run spawns the configured number of goroutines to service the outbound channel.
// This method is idempotent.
func (wp *WorkerPool) Run() {
	wp.runOnce.Do(func() {
		for repeat := uint(0); repeat < wp.workerPoolSize; repeat++ {
			go wp.worker()
		}
	})
}

// transact performs all the logic necessary to fulfill an outbound request.
// This method ensures that the Context associated with the request is properly cancelled.
func (wp *WorkerPool) transact(e outboundEnvelope) {
	defer e.cancel()

	// bail out early if the request has been on the queue too long
	if err := e.request.Context().Err(); err != nil {
		wp.errorLog.Log(logging.MessageKey(), "Outbound message expired while on queue", logging.ErrorKey(), err)
		return
	}

	response, err := wp.transactor(e.request)
	if err != nil {
		wp.errorLog.Log(logging.MessageKey(), "HTTP transaction error", logging.ErrorKey(), err)
		return
	}

	if response.StatusCode < 400 {
		wp.debugLog.Log(logging.MessageKey(), "HTTP response", "status", response.Status, "url", e.request.URL)
	} else {
		wp.errorLog.Log(logging.MessageKey(), "HTTP response", "status", response.Status, "url", e.request.URL)
	}

	io.Copy(ioutil.Discard, response.Body)
	response.Body.Close()
}

// worker represents a single goroutine that processes the outbounds channel.
// This method simply invokes transact for each *outboundEnvelope
func (wp *WorkerPool) worker() {
	for e := range wp.outbounds {
		wp.queueSize.Add(-1.0)
		wp.transact(e)
	}
}
