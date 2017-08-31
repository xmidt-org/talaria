package main

import (
	"io"
	"io/ioutil"
	"net/http"
	"sync"

	"github.com/Comcast/webpa-common/logging"
	"github.com/go-kit/kit/log"
)

// NewTransactor returns a closure which can handle HTTP transactions.
func NewTransactor(o *Outbounder) func(*http.Request) (*http.Response, error) {
	client := &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:        o.maxIdleConns(),
			MaxIdleConnsPerHost: o.maxIdleConnsPerHost(),
			IdleConnTimeout:     o.idleConnTimeout(),
		},
		Timeout: o.clientTimeout(),
	}

	return client.Do
}

// WorkerPool describes a pool of goroutines that dispatch http.Request objects to
// a transactor function
type WorkerPool struct {
	errorLog       log.Logger
	debugLog       log.Logger
	outbounds      <-chan *outboundEnvelope
	workerPoolSize uint
	transactor     func(*http.Request) (*http.Response, error)

	runOnce sync.Once
}

func NewWorkerPool(o *Outbounder, outbounds <-chan *outboundEnvelope) *WorkerPool {
	logger := o.logger()
	return &WorkerPool{
		errorLog:       logging.Error(logger),
		debugLog:       logging.Debug(logger),
		outbounds:      outbounds,
		workerPoolSize: o.workerPoolSize(),
		transactor:     NewTransactor(o),
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
func (wp *WorkerPool) transact(e *outboundEnvelope) {
	defer e.cancel()

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
		wp.transact(e)
	}
}
