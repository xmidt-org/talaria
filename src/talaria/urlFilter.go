package main

import (
	"fmt"
	"strings"
)

// URLFilter represents a strategy for validating and possibly mutating URLs from devices.
type URLFilter interface {
	// Filter accepts a URL and performs validation on it.  This method can return
	// a different URL, if the internal configuration requires it.  For example, if the
	// supplied URL has no scheme, this method may prepend one.
	Filter(string) (string, error)
}

// urlFilter is the internal URLFilter implementation
type urlFilter struct {
	assumeScheme   string
	allowedSchemes map[string]bool
}

// NewURLFilter returns a URLFilter using the supplied configuration.  If assumeScheme is empty,
// DefaultAssumeScheme is used.  If allowedSchemes is empty, the DefaultAllowedScheme is
// used as the sole allowed scheme.  An error is returned if the assumeScheme is not present
// in the allowedSchemes.
func NewURLFilter(o *Outbounder) (URLFilter, error) {
	uf := &urlFilter{
		assumeScheme:   o.assumeScheme(),
		allowedSchemes: o.allowedSchemes(),
	}

	if !uf.allowedSchemes[uf.assumeScheme] {
		return nil, fmt.Errorf(
			"Allowed schemes %v do not include the default scheme %s", uf.allowedSchemes, uf.assumeScheme,
		)
	}

	return uf, nil
}

func (uf *urlFilter) Filter(v string) (string, error) {
	position := strings.Index(v, "://")
	if position < 0 {
		return (uf.assumeScheme + "://" + v), nil
	}

	scheme := v[:position]
	if !uf.allowedSchemes[v[:position]] {
		return "", fmt.Errorf("Scheme not allowed: %s", scheme)
	}

	return v, nil
}
