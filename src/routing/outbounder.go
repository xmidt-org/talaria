package routing

import (
	"bytes"
	"fmt"
	"github.com/Comcast/webpa-common/device"
	"github.com/Comcast/webpa-common/logging"
	"github.com/Comcast/webpa-common/wrp"
	"github.com/spf13/viper"
	"io"
	"io/ioutil"
	"net/http"
	"strings"
	"time"
)

const (
	// OutbounderKey is the Viper subkey which is expected to hold Outbounder configuration
	OutbounderKey = "device.outbound"

	// OutboundContentType is the Content-Type header value for device messages leaving talaria
	OutboundContentType = "application/wrp"

	EventPrefix = "event:"
	URLPrefix   = "url:"

	DefaultMethod                            = "POST"
	DefaultEventEndpoint                     = "http://localhost:8090/api/v2/notify"
	DefaultAssumeScheme                      = "https"
	DefaultAllowedScheme                     = "https"
	DefaultWorkerPoolSize                    = 100
	DefaultRequestQueueSize                  = 1000
	DefaultTimeout             time.Duration = 10 * time.Second
	DefaultMaxIdleConns                      = 0
	DefaultMaxIdleConnsPerHost               = 100
	DefaultIdleConnTimeout     time.Duration = 0
)

// RequestFactory is a simple function type for creating an outbound HTTP request
// for a given WRP message.
type RequestFactory func(device.Interface, []byte, *wrp.Message) (*http.Request, error)

// Outbounder acts as a configurable endpoint for dispatching WRP messages from devices
// and handling any failed messages.
type Outbounder struct {
	Method              string
	EventEndpoint       string
	DeviceNameHeader    string
	AssumeScheme        string
	AllowedSchemes      []string
	WorkerPoolSize      int
	RequestQueueSize    int
	Timeout             time.Duration
	MaxIdleConns        int
	MaxIdleConnsPerHost int
	IdleConnTimeout     time.Duration
	Logger              logging.Logger
}

// NewOutbounder returns an Outbounder unmarshalled from a Viper environment.
// This function allows the Viper instance to be nil, in which case a default
// Outbounder is returned.
func NewOutbounder(logger logging.Logger, v *viper.Viper) (o *Outbounder, err error) {
	o = &Outbounder{
		Method:              DefaultMethod,
		EventEndpoint:       DefaultEventEndpoint,
		DeviceNameHeader:    device.DefaultDeviceNameHeader,
		AssumeScheme:        DefaultAssumeScheme,
		AllowedSchemes:      []string{DefaultAllowedScheme},
		WorkerPoolSize:      DefaultWorkerPoolSize,
		RequestQueueSize:    DefaultRequestQueueSize,
		Timeout:             DefaultTimeout,
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

// NewTransport creates an HTTP RoundTripper (transport) using this Outbounder's configuration.
func (o *Outbounder) newRoundTripper() http.RoundTripper {
	return &http.Transport{
		MaxIdleConns:        o.MaxIdleConns,
		MaxIdleConnsPerHost: o.MaxIdleConnsPerHost,
		IdleConnTimeout:     o.IdleConnTimeout,
	}
}

// newTransactor returns a closure which can execute HTTP transactions
func (o *Outbounder) newTransactor() func(*http.Request) (*http.Response, error) {
	client := &http.Client{
		Transport: o.newRoundTripper(),
		Timeout:   o.Timeout,
	}

	return client.Do
}

// newRequestFactory produces a RequestFactory function that creates an outbound HTTP request
// for a given WRP message from a specific device.  Once created, the returned factory is isolated
// from any changes made to this Outbounder instance.
func (o *Outbounder) newRequestFactory() RequestFactory {
	allowedSchemes := make(map[string]bool, len(o.AllowedSchemes))
	for _, scheme := range o.AllowedSchemes {
		allowedSchemes[scheme] = true
	}

	return func(d device.Interface, raw []byte, message *wrp.Message) (r *http.Request, err error) {
		if strings.HasPrefix(message.Destination, EventPrefix) {
			// route this to the configured endpoint that receives all events
			r, err = http.NewRequest(o.Method, o.EventEndpoint, bytes.NewBuffer(raw))
		} else if strings.HasPrefix(message.Destination, URLPrefix) {
			// route this to the given URL, subject to some validation
			if r, err = http.NewRequest(o.Method, message.Destination[len(URLPrefix):], bytes.NewBuffer(raw)); err == nil {
				if len(r.URL.Scheme) == 0 {
					// if no scheme is supplied, use the configured AssumeScheme
					r.URL.Scheme = o.AssumeScheme
				} else if !allowedSchemes[r.URL.Scheme] {
					err = fmt.Errorf("Scheme not allowed: %s", r.URL.Scheme)
				}
			}
		} else {
			err = fmt.Errorf("Bad WRP destination: %s", message.Destination)
		}

		if err != nil {
			return
		}

		r.Header.Set(o.DeviceNameHeader, string(d.ID()))
		r.Header.Set("Content-Type", OutboundContentType)
		// TODO: Need to set Convey?

		return
	}
}

func (o *Outbounder) Start(listeners *device.Listeners) {
	var (
		transactor     = o.newTransactor()
		requestFactory = o.newRequestFactory()
		requests       = make(chan *http.Request, o.RequestQueueSize)
	)

	for repeat := 0; repeat < o.WorkerPoolSize; repeat++ {
		go func() {
			for request := range requests {
				response, err := transactor(request)
				if err != nil {
					o.Logger.Error("HTTP error: %s", err)
					continue
				}

				if response.StatusCode < 400 {
					o.Logger.Debug("HTTP response status: %s", response.Status)
				} else {
					o.Logger.Error("HTTP response status: %s", response.Status)
				}

				io.Copy(ioutil.Discard, response.Body)
				response.Body.Close()
			}
		}()
	}

	listeners.MessageReceived = func(d device.Interface, message *wrp.Message, raw []byte) {
		request, err := requestFactory(d, raw, message)
		if err != nil {
			o.Logger.Error("Unable to create request for device [%s]: %s", d.ID(), err)
			return
		}

		requests <- request
	}

	// TODO: Need to handle message failures
}
