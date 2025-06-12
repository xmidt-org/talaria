// SPDX-FileCopyrightText: 2017 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xmidt-org/sallust"
	"github.com/xmidt-org/touchstone"
	"github.com/xmidt-org/webpa-common/v2/adapter"
	"github.com/xmidt-org/webpa-common/v2/device"
	"github.com/xmidt-org/webpa-common/v2/event"
	"github.com/xmidt-org/wrp-go/v3"
	"go.uber.org/zap/zaptest"
)

func ExampleOutbounder() {
	var (
		finish = new(sync.WaitGroup)
		server = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			defer finish.Done()
			if body, err := io.ReadAll(request.Body); err != nil {
				fmt.Println(err)
			} else {
				fmt.Printf("%s:%s:%s\n", request.Method, request.Header.Get("Content-Type"), body)
			}
		}))
	)

	defer server.Close()

	var (
		// set the workerPoolSize to 1 so that output order is deterministic
		configuration = []byte(fmt.Sprintf(
			`{
				"defaultScheme": "http",
				"allowedSchemes": ["http", "https"],
				"eventEndpoints": {"default": ["%s"]},
				"workerPoolSize": 1
			}`,
			server.URL,
		))

		v = viper.New()
	)

	v.SetConfigType("json")
	if err := v.ReadConfig(bytes.NewReader(configuration)); err != nil {
		fmt.Println(err)
		return
	}

	o, _, err := NewOutbounder(adapter.DefaultLogger().Logger, v)
	if err != nil {
		fmt.Println(err)
		return
	}

	cfg := touchstone.Config{
		DefaultNamespace: "n",
		DefaultSubsystem: "s",
	}
	_, pr, err := touchstone.New(cfg)
	if err != nil {
		fmt.Println(err)
		return
	}

	tf := touchstone.NewFactory(cfg, sallust.Default(), pr)
	om, err := NewOutboundMeasures(tf)
	if err != nil {
		fmt.Println(err)
		return
	}

	listeners, err := o.Start(om)
	if err != nil {
		fmt.Println(err)
		return
	}

	finish.Add(2)

	for _, l := range listeners {
		l(&device.Event{
			Type:     device.MessageReceived,
			Message:  &wrp.Message{Destination: "event:iot"},
			Format:   wrp.Msgpack,
			Contents: []byte("iot event"),
		})

		l(&device.Event{
			Type:     device.MessageReceived,
			Message:  &wrp.Message{Destination: "dns:" + server.URL},
			Format:   wrp.JSON,
			Contents: []byte("dns message"),
		})
	}

	finish.Wait()

	// Output:
	// POST:application/msgpack:iot event
	// POST:application/json:dns message
}

func testOutbounderDefaults(t *testing.T) {
	require := require.New(t)
	nilViper, _, err := NewOutbounder(nil, nil)
	require.NotNil(nilViper)
	require.NoError(err)

	withViper, _, err := NewOutbounder(nil, viper.New())
	require.NotNil(withViper)
	require.NoError(err)

	assert := assert.New(t)
	for _, o := range []*Outbounder{nil, new(Outbounder), nilViper, withViper} {
		assert.Equal(adapter.DefaultLogger().Logger, o.logger())
		assert.Equal(DefaultMethod, o.method())
		assert.Equal(DefaultRequestTimeout, o.requestTimeout())
		assert.Equal(DefaultDefaultScheme, o.defaultScheme())
		assert.Equal(map[string]bool{DefaultAllowedScheme: true}, o.allowedSchemes())

		m, err := o.eventMap()
		assert.Empty(m)
		assert.NoError(err)

		assert.Equal(DefaultOutboundQueueSize, o.outboundQueueSize())
		assert.Equal(DefaultWorkerPoolSize, o.workerPoolSize())

		transport := o.transport()
		assert.Equal(DefaultMaxIdleConns, transport.MaxIdleConns)
		assert.Equal(DefaultMaxIdleConnsPerHost, transport.MaxIdleConnsPerHost)
		assert.Equal(DefaultIdleConnTimeout, transport.IdleConnTimeout)
		assert.Equal(DefaultClientTimeout, o.clientTimeout())
		assert.Equal(DefaultSource, o.source())
	}
}

func testOutbounderConfiguration(t *testing.T) {
	var (
		assert        = assert.New(t)
		require       = require.New(t)
		logger        = zaptest.NewLogger(t)
		configuration = []byte(`{
			"method": "PATCH",
			"requestTimeout": "30s",
			"defaultScheme": "ftp",
			"allowedSchemes": ["ftp", "nntp"],
			"defaultEventEndpoints": ["https://default.endpoint.com"],
			"eventEndpoints": {
				"iot": ["https://endpoint1.com", "https://endpoint2.com"],
				"something": ["https://endpoint3.com"]
			},
			"outboundQueueSize": 281,
			"workerPoolSize": 17,
			"clientTimeout": "1m10s",
			"source": "talaria.xmidt.comcast.net",
			"transport": {
				"maxIdleConns": 5681,
				"maxIdleConnsPerHost": 99,
				"idleConnTimeout": "2m17s"
			}
		}`)

		v = viper.New()
	)

	v.SetConfigType("json")
	require.NoError(v.ReadConfig(bytes.NewReader(configuration)))

	o, _, err := NewOutbounder(logger, v)
	require.NotNil(o)
	require.NoError(err)

	assert.Equal(logger, o.logger())
	assert.Equal("PATCH", o.method())
	assert.Equal(30*time.Second, o.requestTimeout())
	assert.Equal("ftp", o.defaultScheme())
	assert.Equal(map[string]bool{"nntp": true, "ftp": true}, o.allowedSchemes())

	m, err := o.eventMap()
	assert.NoError(err)
	assert.Equal(
		event.MultiMap{
			"iot":       {"https://endpoint1.com", "https://endpoint2.com"},
			"something": {"https://endpoint3.com"},
		},
		m,
	)
	assert.Equal(uint(281), o.outboundQueueSize())
	assert.Equal(uint(17), o.workerPoolSize())
	assert.Equal(time.Minute+10*time.Second, o.clientTimeout())

	transport := o.transport()
	assert.Equal(5681, transport.MaxIdleConns)
	assert.Equal(99, transport.MaxIdleConnsPerHost)
	assert.Equal(2*time.Minute+17*time.Second, transport.IdleConnTimeout)
	assert.Equal("talaria.xmidt.comcast.net", o.source())
}

func testOutbounderStartError(t *testing.T) {
	var (
		assert        = assert.New(t)
		badOutbounder = &Outbounder{
			DefaultScheme: "ftp",
		}

		listener, err = badOutbounder.Start(OutboundMeasures{})
	)

	assert.Nil(listener)
	assert.Error(err)
}

func TestOutbounder(t *testing.T) {
	t.Run("Defaults", testOutbounderDefaults)
	t.Run("Configuration", testOutbounderConfiguration)
	t.Run("StartError", testOutbounderStartError)
}
