package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// outboundEnvelope is a tuple of information related to handling an asynchronous HTTP request
type outboundEnvelope struct {
	request *http.Request
	cancel  func()
}

// newOutboundEnvelope is an analog to http.NewRequest.  It creates an HTTP request, applies a context,
// and wraps it in an envelope for enqueuing.
func newOutboundEnvelope(timeout time.Duration, method, urlString string, body io.Reader) (*outboundEnvelope, error) {
	r, err := http.NewRequest(method, urlString, body)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	return &outboundEnvelope{r.WithContext(ctx), cancel}, nil
}

// envelopeFactory takes an HTTP request and produces one or more envelopes for transport.
// An envelopeFactory function can make as many calls to newOutboundEnvelope as needed to
// route the message to the appropriate external destinations.
type envelopeFactory func(string, []byte) ([]*outboundEnvelope, error)

// Routing describes how WRP messages are transformed into requests
type Routing struct {
	Method                string
	Timeout               time.Duration
	AssumeScheme          string
	AllowedSchemes        []string
	DefaultEventEndpoints []string
	EventEndpoints        map[string][]string
}

func (r *Routing) NewEnvelopeFactory() envelopeFactory {
	var (
		urlFilter             = newURLFilter(r.AssumeScheme, r.AllowedSchemes)
		method                = r.Method
		timeout               = r.Timeout
		defaultEventEndpoints = r.DefaultEventEndpoints
	)

	if len(method) == 0 {
		method = DefaultMethod
	}

	if timeout < 1 {
		timeout = DefaultRequestTimeout
	}

	if len(defaultEventEndpoints) == 0 {
		defaultEventEndpoints = []string{DefaultDefaultEventEndpoint}
	}

	return func(d string, c []byte) (envelopes []*outboundEnvelope, err error) {
		if strings.HasPrefix(d, EventPrefix) {
			endpoints := r.EventEndpoints[d[len(EventPrefix):]]
			if len(endpoints) == 0 {
				endpoints = defaultEventEndpoints
			}

			envelopes = make([]*outboundEnvelope, len(endpoints))
			for i := 0; i < len(endpoints) && err == nil; i++ {
				envelopes[i], err = newOutboundEnvelope(timeout, method, endpoints[i], bytes.NewReader(c))
			}
		} else if strings.HasPrefix(d, DNSPrefix) {
			if urlString, err := urlFilter.filter(d[len(DNSPrefix):]); err == nil {
				envelopes = make([]*outboundEnvelope, 1)
				envelopes[0], err = newOutboundEnvelope(timeout, method, urlString, bytes.NewReader(c))
			}
		} else {
			err = fmt.Errorf("Unable to route to destination: %s", d)
		}

		return
	}
}
