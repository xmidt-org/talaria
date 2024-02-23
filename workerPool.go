// SPDX-FileCopyrightText: 2017 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"

	"github.com/go-kit/kit/metrics"
	"go.uber.org/zap"
)

// WorkerPool describes a pool of goroutines that dispatch http.Request objects to
// a transactor function
type WorkerPool struct {
	logger          *zap.Logger
	outbounds       <-chan outboundEnvelope
	workerPoolSize  uint
	queueSize       metrics.Gauge
	droppedMessages metrics.Counter
	transactor      func(*http.Request) (*http.Response, error)

	runOnce sync.Once
}

func NewWorkerPool(om OutboundMeasures, o *Outbounder, outbounds <-chan outboundEnvelope) *WorkerPool {
	logger := o.logger()
	return &WorkerPool{
		logger:          logger,
		outbounds:       outbounds,
		workerPoolSize:  o.workerPoolSize(),
		queueSize:       om.QueueSize,
		droppedMessages: om.DroppedMessages,
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

	eventType, ok := e.request.Context().Value(eventTypeContextKey{}).(string)
	if !ok {
		eventType = unknown
	}

	// bail out early if the request has been on the queue too long
	if err := e.request.Context().Err(); err != nil {
		url := e.request.URL.String()
		reason := getDoErrReason(err)
		wp.droppedMessages.With(eventType, "", reason, url).Add(1)
		wp.logger.Error("Outbound message expired while on queue", zap.String("event", eventType), zap.String("reason", reason), zap.Error(err), zap.String("url", url))

		return
	}

	response, err := wp.transactor(e.request)
	if err != nil {
		url := response.Request.URL.String()
		reason := getDoErrReason(err)
		code := strconv.Itoa(response.StatusCode)
		wp.droppedMessages.With(eventType, code, reason, url).Add(1)
		wp.logger.Error("HTTP transaction error", zap.String("event", eventType), zap.String("reason", code), zap.String("reason", reason), zap.Error(err), zap.String("url", url))

		return
	}

	if response.StatusCode != 200 {
		url := response.Request.URL.String()
		code := strconv.Itoa(response.StatusCode)
		wp.droppedMessages.With(eventType, code, non200, url).Add(1)
		wp.logger.Warn("HTTP response", zap.String("event", eventType), zap.String("reason", code), zap.String("reason", non200), zap.String("url", url))
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

func getDoErrReason(e error) string {
	if errors.Is(e, context.DeadlineExceeded) {
		return deadlineExceededReason
	}
	if errors.Is(e, context.Canceled) {
		return contextCanceledReason
	}
	if errors.Is(e, &net.AddrError{}) {
		return addressErrReason
	}
	if errors.Is(e, &net.ParseError{}) {
		return parseAddrErrReason
	}
	if errors.Is(e, net.InvalidAddrError("")) {
		return invalidAddrReason
	}
	var d *net.DNSError
	if errors.As(e, &d) {
		if d.IsNotFound {
			return hostNotFoundReason
		}
		return dnsErrReason
	}
	if errors.Is(e, net.ErrClosed) {
		return connClosedReason
	}
	if errors.Is(e, &net.OpError{}) {
		return opErrReason
	}
	if errors.Is(e, net.UnknownNetworkError("")) {
		return networkErrReason
	}
	return genericDoReason
}
