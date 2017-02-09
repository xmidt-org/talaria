package routing

import (
	"bytes"
	"fmt"
	"github.com/Comcast/webpa-common/device"
	"github.com/Comcast/webpa-common/logging"
	"github.com/Comcast/webpa-common/wrp"
	"github.com/spf13/viper"
	"net/http"
	"strings"
	"time"
)

const (
	// OutbounderKey is the Viper subkey which is expected to hold Outbounder configuration
	OutbounderKey = "device.outbound"

	EventPrefix = "event:"
	URLPrefix   = "url:"

	DefaultMethod                            = "POST"
	DefaultEventEndpoint                     = "http://localhost:8090/api/v2/notify"
	DefaultAssumeScheme                      = "https"
	DefaultAllowedScheme                     = "https"
	DefaultContentType                       = "application/wrp"
	DefaultTimeout             time.Duration = 10 * time.Second
	DefaultMaxIdleConns                      = 0
	DefaultMaxIdleConnsPerHost               = 100
	DefaultIdleConnTimeout     time.Duration = 0
)

// RequestFactory is a simple function type for creating an outbound HTTP request
// for a given WRP message.
type RequestFactory func(device.Interface, []byte, *wrp.Message) (*http.Request, error)

// Outbounder is a Manager listener that accepts device messages and dispatches them
// to the notification endpoint.
type Outbounder struct {
	Method              string
	EventEndpoint       string
	DeviceNameHeader    string
	AssumeScheme        string
	AllowedSchemes      []string
	ContentType         string
	Timeout             time.Duration
	MaxIdleConns        int
	MaxIdleConnsPerHost int
	IdleConnTimeout     time.Duration
}

// NewOutbounder returns an Outbounder unmarshalled from a Viper environment.
// This function allows the Viper instance to be nil, in which case a default
// Outbounder is returned.
func NewOutbounder(v *viper.Viper) (o *Outbounder, err error) {
	o = new(Outbounder)
	if v != nil {
		err = v.Unmarshal(o)
	}

	return
}

func (o *Outbounder) method() string {
	if len(o.Method) > 0 {
		return o.Method
	}

	return DefaultMethod
}

func (o *Outbounder) eventEndpoint() string {
	if len(o.EventEndpoint) > 0 {
		return o.EventEndpoint
	}

	return DefaultEventEndpoint
}

func (o *Outbounder) deviceNameHeader() string {
	if len(o.DeviceNameHeader) > 0 {
		return o.DeviceNameHeader
	}

	return device.DefaultDeviceNameHeader
}

func (o *Outbounder) assumeScheme() string {
	if len(o.AssumeScheme) > 0 {
		return o.AssumeScheme
	}

	return DefaultAssumeScheme
}

func (o *Outbounder) allowedSchemes() map[string]bool {
	if len(o.AllowedSchemes) > 0 {
		schemes := make(map[string]bool, len(o.AllowedSchemes))
		for _, scheme := range o.AllowedSchemes {
			schemes[scheme] = true
		}

		return schemes
	}

	return map[string]bool{
		DefaultAllowedScheme: true,
	}
}

func (o *Outbounder) contentType() string {
	if len(o.ContentType) > 0 {
		return o.ContentType
	}

	return DefaultContentType
}

func (o *Outbounder) timeout() time.Duration {
	if o.Timeout > 0 {
		return o.Timeout
	}

	return DefaultTimeout
}

func (o *Outbounder) maxIdleConns() int {
	if o.MaxIdleConns > 0 {
		return o.MaxIdleConns
	}

	return DefaultMaxIdleConns
}

func (o *Outbounder) maxIdleConnsPerHost() int {
	if o.MaxIdleConnsPerHost > 0 {
		return o.MaxIdleConnsPerHost
	}

	return DefaultMaxIdleConnsPerHost
}

func (o *Outbounder) idleConnTimeout() time.Duration {
	if o.IdleConnTimeout > 0 {
		return o.IdleConnTimeout
	}

	return DefaultIdleConnTimeout
}

func (o *Outbounder) newTransport() *http.Transport {
	return &http.Transport{
		MaxIdleConns:        o.maxIdleConns(),
		MaxIdleConnsPerHost: o.maxIdleConnsPerHost(),
		IdleConnTimeout:     o.idleConnTimeout(),
	}
}

func (o *Outbounder) newClient() *http.Client {
	return &http.Client{
		Transport: o.newTransport(),
		Timeout:   o.timeout(),
	}
}

// newRequestFactory produces a RequestFactory function that creates an outbound HTTP request
// for a given WRP message from a specific device.  Once created, the returned factory is isolated
// from any changes made to this Outbounder instance.
func (o *Outbounder) newRequestFactory() RequestFactory {
	var (
		method           = o.method()
		eventEndpoint    = o.eventEndpoint()
		contentType      = o.contentType()
		deviceNameHeader = o.deviceNameHeader()
		assumeScheme     = o.assumeScheme()
		allowedSchemes   = o.allowedSchemes()
	)

	return func(d device.Interface, raw []byte, message *wrp.Message) (r *http.Request, err error) {
		if strings.HasPrefix(message.Destination, EventPrefix) {
			// route this to the configured endpoint that receives all events
			r, err = http.NewRequest(method, eventEndpoint, bytes.NewBuffer(raw))
		} else if strings.HasPrefix(message.Destination, URLPrefix) {
			// route this to the given URL, subject to some validation
			if r, err = http.NewRequest(method, message.Destination[len(URLPrefix):], bytes.NewBuffer(raw)); err == nil {
				if len(r.URL.Scheme) == 0 {
					// if no scheme is supplied, use the configured AssumeScheme
					r.URL.Scheme = assumeScheme
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

		r.Header.Set(deviceNameHeader, string(d.ID()))
		r.Header.Set("Content-Type", contentType)
		// TODO: Need to set Convey?

		return
	}
}

// NewMessageListener returns a MessageListener which dispatches an HTTP transaction
// for each WRP message.
func (o *Outbounder) NewMessageListener(logger logging.Logger) device.MessageListener {
	var (
		client         = o.newClient()
		requestFactory = o.newRequestFactory()
	)

	return func(d device.Interface, raw []byte, message *wrp.Message) {
		request, err := requestFactory(d, raw, message)
		if err != nil {
			logger.Error("Unable to create request for device [%s]: %s", d.ID(), err)
			return
		}

		response, err := client.Do(request)
		if err != nil {
			logger.Error("HTTP error for device [%s]: %s", d.ID(), err)
			return
		}

		if response.StatusCode < 400 {
			logger.Debug("HTTP response for device [%s]: %s", d.ID(), response.Status)
		} else {
			logger.Error("HTTP response for device [%s]: %s", d.ID(), response.Status)
		}
	}
}
