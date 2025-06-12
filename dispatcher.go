// SPDX-FileCopyrightText: 2017 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"errors"
	"net/http"

	"github.com/xmidt-org/webpa-common/v2/device"
)

var ErrOutboundQueueFull = errors.New("outbound message queue full")

// Dispatcher handles the creation and routing of HTTP requests in response to device events.
// A Dispatcher represents the send side for enqueuing HTTP requests.
type Dispatcher interface {
	// OnDeviceEvent is the device.Listener function that processes outbound events.  Inject
	// this function as a device listener for a manager.
	OnDeviceEvent(*device.Event)
}

// outboundEnvelope is a tuple of information related to handling an asynchronous HTTP request.
type outboundEnvelope struct {
	request *http.Request
	cancel  func()
}

// schemeContextKey is the internal key type for storing the event type
type schemeContextKey struct{}
