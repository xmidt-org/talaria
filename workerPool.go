// SPDX-FileCopyrightText: 2017 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"io"
	"net/http"
	"sync"

	"github.com/go-kit/kit/metrics"
	"go.uber.org/zap"
)

// WorkerPool describes a pool of goroutines that dispatch http.Request objects to
// a transactor function
type WorkerPool struct {
	logger         *zap.Logger
	outbounds      <-chan outboundEnvelope
	workerPoolSize uint
	queueSize      metrics.Gauge
	transactor     func(*http.Request) (*http.Response, error)

	runOnce sync.Once
}

func NewWorkerPool(om OutboundMeasures, o *Outbounder, outbounds <-chan outboundEnvelope) *WorkerPool {
	logger := o.logger()
	return &WorkerPool{
		logger:         logger,
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
// This method ensures that the Context associated with the request is properly canceled.
func (wp *WorkerPool) transact(e outboundEnvelope) {
	defer e.cancel()

	// bail out early if the request has been on the queue too long
	if err := e.request.Context().Err(); err != nil {
		wp.logger.Error("Outbound message expired while on queue", zap.Error(err))
		return
	}

	response, err := wp.transactor(e.request)
	if err != nil {
		wp.logger.Error("HTTP transaction error", zap.Error(err))
		return
	}

	if response.StatusCode < 400 {
		wp.logger.Debug("HTTP response", zap.String("status", response.Status), zap.Any("url", e.request.URL))
	} else {
		wp.logger.Error("HTTP response", zap.String("status", response.Status), zap.Any("url", e.request.URL))
	}

	io.Copy(io.Discard, response.Body)
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
