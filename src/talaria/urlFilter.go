package main

import (
	"fmt"
	"regexp"
)

// urlPattern is the regular expression used to parsed out the scheme from URLs
var urlPattern = regexp.MustCompile(`(?P<prefix>(?P<scheme>[a-z0-9\+\-\.]*)://)?.*`)

// urlFilter performs validation and mutation on URLs supplied by devices
type urlFilter struct {
	assumeScheme   string
	allowedSchemes map[string]bool
}

// newURLFilter preprocesses its parameters, applying appropriate defaults, and produces
// a urlFilter instance.
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

// filter accepts a URL and ensures that there is a scheme and that the scheme is allowed.
func (uf *urlFilter) filter(v string) (string, error) {
	matches := urlPattern.FindStringSubmatchIndex(v)

	// indices (2, 3) are the prefix range
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
