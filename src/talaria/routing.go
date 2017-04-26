package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// outboundEnvelope is a tuple of information related to handling an asynchronous HTTP request
type outboundEnvelope struct {
	request *http.Request
	cancel  func()
}

// done is syntactic sugar for request.Context().Done()
func (oe *outboundEnvelope) done() <-chan struct{} {
	return oe.request.Context().Done()
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
type envelopeFactory func(string, []byte) ([]*outboundEnvelope, error)

// urlPattern is the regular expression used to parsed out the scheme from URLs
var urlPattern = regexp.MustCompile(`(?P<prefix>(?P<scheme>[a-z0-9\+\-\.]*)://)?.*`)

// urlFilter performs validation and mutation on URLs supplied by devices
type urlFilter struct {
	assumeScheme   string
	allowedSchemes map[string]bool
}

func newURLFilter(assumeScheme string, allowedSchemes []string) *urlFilter {
	uf := &urlFilter{
		assumeScheme:   assumeScheme,
		allowedSchemes: make(map[string]bool, len(allowedSchemes)),
	}

	if len(uf.assumeScheme) == 0 {
		uf.assumeScheme = DefaultAssumeScheme
	}

	if len(allowedSchemes) > 0 {
		for _, v := range allowedSchemes {
			uf.allowedSchemes[v] = true
		}
	} else {
		uf.allowedSchemes[DefaultAllowedScheme] = true
	}

	if !uf.allowedSchemes[uf.assumeScheme] {
		panic(fmt.Errorf("Allowed schemes %v do not include the default scheme %s", uf.allowedSchemes, uf.assumeScheme))
	}

	return uf
}

func (uf *urlFilter) filter(v string) (string, error) {
	matches := urlPattern.FindStringSubmatchIndex(v)
	if matches[2] < 0 {
		// no prefix at all
		return (uf.assumeScheme + `://` + v), nil
	}

	scheme := v[matches[4]:matches[5]]
	if !uf.allowedSchemes[scheme] {
		return "", fmt.Errorf("Scheme not allowed: %s", scheme)
	}

	return v, nil
}

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
		urlFilter = newURLFilter(r.AssumeScheme, r.AllowedSchemes)
		method    = r.Method
		timeout   = r.Timeout
	)

	if len(method) == 0 {
		method = DefaultMethod
	}

	if timeout < 1 {
		timeout = DefaultRequestTimeout
	}

	return func(d string, c []byte) (envelopes []*outboundEnvelope, err error) {
		if strings.HasPrefix(d, EventPrefix) {
			endpoints := r.EventEndpoints[d[len(EventPrefix):]]
			if len(endpoints) == 0 {
				endpoints = r.DefaultEventEndpoints
			}

			envelopes = make([]*outboundEnvelope, len(endpoints))
			for i := 0; i < len(endpoints) && err == nil; i++ {
				envelopes[i], err = newOutboundEnvelope(timeout, method, endpoints[i], bytes.NewReader(c))
			}
		} else {
			var urlString string
			if strings.HasPrefix(d, URLPrefix) {
				urlString, err = urlFilter.filter(d[len(URLPrefix):])
			} else {
				urlString, err = urlFilter.filter(d)
			}

			if err == nil {
				envelopes = make([]*outboundEnvelope, 1)
				envelopes[0], err = newOutboundEnvelope(timeout, method, urlString, bytes.NewReader(c))
			}
		}

		return
	}
}
