package main

import (
	"github.com/Comcast/webpa-common/device"
	"github.com/Comcast/webpa-common/logging"
	"github.com/spf13/viper"
	"time"
)

const (
	// OutbounderKey is the Viper subkey which is expected to hold Outbounder configuration
	OutbounderKey = "device.outbound"

	// EventPrefix is the string prefix for WRP destinations that should be treated as events
	EventPrefix = "event:"

	// DNSPrefix is the string prefix for WRP destinations that should be treated as exact URLs
	DNSPrefix = "dns:"

	DefaultAssumeScheme  = "https"
	DefaultAllowedScheme = "https"

	DefaultMethod                            = "POST"
	DefaultWorkerPoolSize                    = 100
	DefaultOutboundQueueSize                 = 1000
	DefaultRequestTimeout      time.Duration = 15 * time.Second
	DefaultClientTimeout       time.Duration = 3 * time.Second
	DefaultMaxIdleConns                      = 0
	DefaultMaxIdleConnsPerHost               = 100
	DefaultIdleConnTimeout     time.Duration = 0
)

// Outbounder encapsulates the configuration necessary for handling outbound traffic
// and grants the ability to start the outbounding infrastructure.
type Outbounder struct {
	Method                string
	RequestTimeout        time.Duration
	AssumeScheme          string
	AllowedSchemes        []string
	DefaultEventEndpoints []string
	EventEndpoints        map[string][]string
	OutboundQueueSize     uint
	WorkerPoolSize        uint
	ClientTimeout         time.Duration
	MaxIdleConns          int
	MaxIdleConnsPerHost   int
	IdleConnTimeout       time.Duration
	Logger                logging.Logger
}

// NewOutbounder returns an Outbounder unmarshalled from a Viper environment.
// This function allows the Viper instance to be nil, in which case a default
// Outbounder is returned.
func NewOutbounder(logger logging.Logger, v *viper.Viper) (o *Outbounder, err error) {
	o = &Outbounder{
		Method:              DefaultMethod,
		RequestTimeout:      DefaultRequestTimeout,
		AssumeScheme:        DefaultAssumeScheme,
		AllowedSchemes:      []string{DefaultAllowedScheme},
		OutboundQueueSize:   DefaultOutboundQueueSize,
		WorkerPoolSize:      DefaultWorkerPoolSize,
		ClientTimeout:       DefaultClientTimeout,
		MaxIdleConns:        DefaultMaxIdleConns,
		MaxIdleConnsPerHost: DefaultMaxIdleConnsPerHost,
		IdleConnTimeout:     DefaultIdleConnTimeout,
		Logger:              logger,
	}

	if v != nil {
		err = v.Unmarshal(o)
	}

	return
}

func (o *Outbounder) logger() logging.Logger {
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

func (o *Outbounder) requestTimeout() time.Duration {
	if o != nil && o.RequestTimeout > 0 {
		return o.RequestTimeout
	}

	return DefaultRequestTimeout
}

func (o *Outbounder) assumeScheme() string {
	if o != nil && len(o.AssumeScheme) > 0 {
		return o.AssumeScheme
	}

	return DefaultAssumeScheme
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

func (o *Outbounder) defaultEventEndpoints() []string {
	if o != nil && len(o.DefaultEventEndpoints) > 0 {
		return o.DefaultEventEndpoints
	}

	// no reason not to return nil here, as client code
	// only iterates over this slice and gets the length
	return nil
}

func (o *Outbounder) eventEndpoints() map[string][]string {
	if o != nil {
		return o.EventEndpoints
	}

	// don't return nil, as we want to make sure client code can
	// always do lookups for event types
	return map[string][]string{}
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

func (o *Outbounder) maxIdleConns() int {
	if o != nil && o.MaxIdleConns > 0 {
		return o.MaxIdleConns
	}

	return DefaultMaxIdleConns
}

func (o *Outbounder) maxIdleConnsPerHost() int {
	if o != nil && o.MaxIdleConnsPerHost > 0 {
		return o.MaxIdleConnsPerHost
	}

	return DefaultMaxIdleConnsPerHost
}

func (o *Outbounder) idleConnTimeout() time.Duration {
	if o != nil && o.IdleConnTimeout > 0 {
		return o.IdleConnTimeout
	}

	return DefaultIdleConnTimeout
}

func (o *Outbounder) clientTimeout() time.Duration {
	if o != nil && o.ClientTimeout > 0 {
		return o.ClientTimeout
	}

	return DefaultClientTimeout
}

// Start spawns all necessary goroutines and returns a device.Listener
func (o *Outbounder) Start() (device.Listener, error) {
	dispatcher, outbounds, err := NewDispatcher(o, nil)
	if err != nil {
		return nil, err
	}

	workerPool := NewWorkerPool(o, outbounds)
	workerPool.Run()

	return dispatcher.OnDeviceEvent, nil
}
