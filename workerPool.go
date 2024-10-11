// SPDX-FileCopyrightText: 2017 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

// WorkerPool describes a pool of goroutines that dispatch http.Request objects to
// a transactor function
type WorkerPool struct {
	logger          *zap.Logger
	outbounds       <-chan outboundEnvelope
	workerPoolSize  uint
	queueSize       prometheus.Gauge
	droppedMessages CounterVec
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

	scheme, ok := e.request.Context().Value(schemeContextKey{}).(string)
	if !ok {
		scheme = unknown
	}

	// bail out early if the request has been on the queue too long
	if err := e.request.Context().Err(); err != nil {
		reason := getDroppedMessageReason(err)
		wp.droppedMessages.With(prometheus.Labels{schemeLabel: scheme, codeLabel: messageDroppedCode, reasonLabel: reason}).Add(1)
		wp.logger.Error("Outbound message expired while on queue", zap.String(schemeLabel, scheme), zap.String("reason", reason), zap.Error(err))

		return
	}

	response, err := wp.transactor(e.request)
	if err != nil {
		reason := getDroppedMessageReason(err)
		code := messageDroppedCode
		if response != nil {
			code = strconv.Itoa(response.StatusCode)
		}

		wp.droppedMessages.With(prometheus.Labels{schemeLabel: scheme, codeLabel: code, reasonLabel: reason}).Add(1)
		wp.logger.Error("HTTP transaction error", zap.String(schemeLabel, scheme), zap.String(codeLabel, code), zap.String(reasonLabel, reason), zap.Error(err))

		return
	}

	code := strconv.Itoa(response.StatusCode)
	switch response.StatusCode {
	case http.StatusAccepted:
		wp.logger.Debug("HTTP response", zap.String("status", response.Status), zap.String(schemeLabel, scheme), zap.String(codeLabel, code), zap.String(reasonLabel, expectedCodeReason))
	default:
		wp.droppedMessages.With(prometheus.Labels{schemeLabel: scheme, codeLabel: code, reasonLabel: non202CodeReason}).Add(1)
		wp.logger.Warn("HTTP response", zap.String(schemeLabel, scheme), zap.String(codeLabel, code), zap.String(reasonLabel, non202CodeReason))
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

func getDoErrReason(err error) string {
	var d *net.DNSError
	if err == nil {
		return noErrReason
	} else if errors.Is(err, context.DeadlineExceeded) {
		return deadlineExceededReason
	} else if errors.Is(err, context.Canceled) {
		return contextCanceledReason
	} else if errors.Is(err, &net.AddrError{}) {
		return addressErrReason
	} else if errors.Is(err, &net.ParseError{}) {
		return parseAddrErrReason
	} else if errors.Is(err, net.InvalidAddrError("")) {
		return invalidAddrReason
	} else if errors.As(err, &d) {
		if d.IsNotFound {
			return hostNotFoundReason
		}
		return dnsErrReason
	} else if errors.Is(err, net.ErrClosed) {
		return connClosedReason
	} else if errors.Is(err, &net.OpError{}) {
		return opErrReason
	} else if errors.Is(err, net.UnknownNetworkError("")) {
		return networkErrReason
	}

	// nolint: errorlint
	if err, ok := err.(*url.Error); ok {
		if strings.TrimSpace(strings.ToLower(err.Unwrap().Error())) == "eof" {
			return connectionUnexpectedlyClosedEOFReason
		}
	}

	return genericDoReason
}
