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
	defaultScheme  string
	allowedSchemes map[string]bool
}

// NewURLFilter returns a URLFilter using the supplied configuration.  If defaultScheme is empty,
// DefaultAssumeScheme is used.  If allowedSchemes is empty, the DefaultAllowedScheme is
// used as the sole allowed scheme.  An error is returned if the defaultScheme is not present
// in the allowedSchemes.
func NewURLFilter(o *Outbounder) (URLFilter, error) {
	uf := &urlFilter{
		defaultScheme:  o.defaultScheme(),
		allowedSchemes: o.allowedSchemes(),
	}

	if !uf.allowedSchemes[uf.defaultScheme] {
		return nil, fmt.Errorf(
			"Allowed schemes %v do not include the default scheme %s", uf.allowedSchemes, uf.defaultScheme,
		)
	}

	return uf, nil
}

func (uf *urlFilter) Filter(v string) (string, error) {
	position := strings.Index(v, "://")
	if position < 0 {
		return (uf.defaultScheme + "://" + v), nil
	}

	scheme := v[:position]
	if !uf.allowedSchemes[v[:position]] {
		return "", fmt.Errorf("Scheme not allowed: %s", scheme)
	}

	return v, nil
}
