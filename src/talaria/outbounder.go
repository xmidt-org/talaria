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
	"encoding/json"
	"net/http"
	"time"

	"github.com/Comcast/webpa-common/device"
	"github.com/Comcast/webpa-common/event"
	"github.com/Comcast/webpa-common/logging"
	"github.com/go-kit/kit/log"
	"github.com/spf13/viper"
)

const (
	// OutbounderKey is the Viper subkey which is expected to hold Outbounder configuration
	OutbounderKey = "device.outbound"

	// EventPrefix is the string prefix for WRP destinations that should be treated as events
	EventPrefix = "event:"

	// DNSPrefix is the string prefix for WRP destinations that should be treated as exact URLs
	DNSPrefix = "dns:"

	DefaultDefaultScheme = "https"
	DefaultAllowedScheme = "https"

	DefaultEventType                         = "default"
	DefaultMethod                            = "POST"
	DefaultRetries                           = 1
	DefaultWorkerPoolSize      uint          = 100
	DefaultOutboundQueueSize   uint          = 1000
	DefaultRequestTimeout      time.Duration = 125 * time.Second
	DefaultClientTimeout       time.Duration = 160 * time.Second
	DefaultMaxIdleConns                      = 0
	DefaultMaxIdleConnsPerHost               = 100
	DefaultIdleConnTimeout     time.Duration = 0
)

// Outbounder encapsulates the configuration necessary for handling outbound traffic
// and grants the ability to start the outbounding infrastructure.
type Outbounder struct {
	Method            string                 `json:"method"`
	Retries           int                    `json:"retries"`
	RequestTimeout    time.Duration          `json:"requestTimeout"`
	DefaultScheme     string                 `json:"defaultScheme"`
	AllowedSchemes    []string               `json:"allowedSchemes"`
	EventEndpoints    map[string]interface{} `json:"eventEndpoints"`
	OutboundQueueSize uint                   `json:"outboundQueueSize"`
	WorkerPoolSize    uint                   `json:"workerPoolSize"`
	Transport         http.Transport         `json:"transport"`
	ClientTimeout     time.Duration          `json:"clientTimeout"`
	AuthKey           []string               `json:"authKey"`
	Logger            log.Logger             `json:"-"`
}

// NewOutbounder returns an Outbounder unmarshalled from a Viper environment.
// This function allows the Viper instance to be nil, in which case a default
// Outbounder is returned.
func NewOutbounder(logger log.Logger, v *viper.Viper) (o *Outbounder, err error) {
	o = &Outbounder{
		Method:            DefaultMethod,
		RequestTimeout:    DefaultRequestTimeout,
		DefaultScheme:     DefaultDefaultScheme,
		AllowedSchemes:    []string{DefaultAllowedScheme},
		OutboundQueueSize: DefaultOutboundQueueSize,
		WorkerPoolSize:    DefaultWorkerPoolSize,
		Transport: http.Transport{
			MaxIdleConns:        DefaultMaxIdleConns,
			MaxIdleConnsPerHost: DefaultMaxIdleConnsPerHost,
			IdleConnTimeout:     DefaultIdleConnTimeout,
		},
		ClientTimeout: DefaultClientTimeout,
		Logger:        logger,
	}

	if v != nil {
		err = v.Unmarshal(o)
	}

	return
}

// String emits a JSON string representing this outbounder, primarily useful for debugging.
func (o *Outbounder) String() string {
	if data, err := json.Marshal(o); err != nil {
		return err.Error()
	} else {
		return string(data)
	}
}

func (o *Outbounder) logger() log.Logger {
	if o != nil && o.Logger != nil {
		return o.Logger
	}

	return logging.DefaultLogger()
}

func (o *Outbounder) method() string {
	if o != nil && len(o.Method) > 0 {
		return o.Method
	}

	return DefaultMethod
}

func (o *Outbounder) retries() int {
	if o != nil && o.Retries > 0 {
		return o.Retries
	}

	return DefaultRetries
}

func (o *Outbounder) requestTimeout() time.Duration {
	if o != nil && o.RequestTimeout > 0 {
		return o.RequestTimeout
	}

	return DefaultRequestTimeout
}

func (o *Outbounder) defaultScheme() string {
	if o != nil && len(o.DefaultScheme) > 0 {
		return o.DefaultScheme
	}

	return DefaultDefaultScheme
}

func (o *Outbounder) allowedSchemes() map[string]bool {
	if o != nil && len(o.AllowedSchemes) > 0 {
		allowedSchemes := make(map[string]bool, len(o.AllowedSchemes))
		for _, as := range o.AllowedSchemes {
			allowedSchemes[as] = true
		}

		return allowedSchemes
	}

	return map[string]bool{DefaultAllowedScheme: true}
}

func (o *Outbounder) eventMap() (event.MultiMap, error) {
	if o != nil {
		return event.NestedToMultiMap(".", o.EventEndpoints)
	}

	// don't return nil, as we want to make sure client code can
	// always do lookups for event types
	return event.MultiMap{}, nil
}

func (o *Outbounder) outboundQueueSize() uint {
	if o != nil && o.OutboundQueueSize > 0 {
		return o.OutboundQueueSize
	}

	return DefaultOutboundQueueSize
}

func (o *Outbounder) workerPoolSize() uint {
	if o != nil && o.WorkerPoolSize > 0 {
		return o.WorkerPoolSize
	}

	return DefaultWorkerPoolSize
}

func (o *Outbounder) transport() *http.Transport {
	if o != nil {
		copyOf := new(http.Transport)
		*copyOf = o.Transport

		if copyOf.MaxIdleConns <= 0 {
			copyOf.MaxIdleConns = DefaultMaxIdleConns
		}

		if copyOf.MaxIdleConnsPerHost <= 0 {
			copyOf.MaxIdleConnsPerHost = DefaultMaxIdleConnsPerHost
		}

		if copyOf.IdleConnTimeout <= 0 {
			copyOf.IdleConnTimeout = DefaultIdleConnTimeout
		}

		return copyOf
	}

	return &http.Transport{
		MaxIdleConns:        DefaultMaxIdleConns,
		MaxIdleConnsPerHost: DefaultMaxIdleConnsPerHost,
		IdleConnTimeout:     DefaultIdleConnTimeout,
	}
}

func (o *Outbounder) authKey() []string {
	if o != nil {
		return o.AuthKey
	}

	return nil
}

func (o *Outbounder) clientTimeout() time.Duration {
	if o != nil && o.ClientTimeout > 0 {
		return o.ClientTimeout
	}

	return DefaultClientTimeout
}

// Start spawns all necessary goroutines and returns a device.Listener
func (o *Outbounder) Start(om OutboundMeasures) (device.Listener, error) {
	logging.Info(o.logger()).Log(logging.MessageKey(), "Starting outbounder")
	dispatcher, outbounds, err := NewDispatcher(om, o, nil)
	if err != nil {
		return nil, err
	}

	workerPool := NewWorkerPool(om, o, outbounds)
	workerPool.Run()

	return dispatcher.OnDeviceEvent, nil
}
