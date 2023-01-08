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
	"bytes"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xmidt-org/webpa-common/v2/convey"
	"github.com/xmidt-org/webpa-common/v2/device"
	"github.com/xmidt-org/wrp-go/v3"
)

func genTestMetadata() *device.Metadata {
	m := new(device.Metadata)

	claims := map[string]interface{}{
		device.PartnerIDClaimKey: "partner-1",
		device.TrustClaimKey:     0,
	}

	m.SetClaims(claims)
	return m
}

func testEventDispatcherOnDeviceEventConnectEvent(t *testing.T) {
	var (
		assert                     = assert.New(t)
		require                    = require.New(t)
		d                          = new(device.MockDevice)
		dispatcher, outbounds, err = NewEventDispatcher(NewTestOutboundMeasures(), nil, nil)
	)

	require.NotNil(dispatcher)
	require.NotNil(outbounds)
	require.NoError(err)

	deviceMetadata := genTestMetadata()

	d.On("ID").Return(device.ID("mac:123412341234"))
	d.On("Metadata").Return(deviceMetadata)
	d.On("Convey").Return(convey.C(nil))

	dispatcher.OnDeviceEvent(&device.Event{Type: device.Connect, Device: d})
	assert.Equal(0, len(outbounds))
	d.AssertExpectations(t)
}

func testEventDispatcherOnDeviceEventDisconnectEvent(t *testing.T) {
	var (
		assert                     = assert.New(t)
		require                    = require.New(t)
		d                          = new(device.MockDevice)
		dispatcher, outbounds, err = NewEventDispatcher(NewTestOutboundMeasures(), nil, nil)
	)

	require.NotNil(dispatcher)
	require.NotNil(outbounds)
	require.NoError(err)

	deviceMetadata := genTestMetadata()

	d.On("ID").Return(device.ID("mac:123412341234"))
	d.On("Metadata").Return(deviceMetadata)
	d.On("Convey").Return(convey.C(nil))
	d.On("Statistics").Return(device.NewStatistics(nil, time.Now()))
	d.On("Statistics").Return(device.NewStatistics(nil, time.Now()))
	d.On("CloseReason").Return(device.CloseReason{})

	dispatcher.OnDeviceEvent(&device.Event{Type: device.Disconnect, Device: d})
	assert.Equal(0, len(outbounds))
	d.AssertExpectations(t)
}

func testEventDispatcherOnDeviceEventUnroutable(t *testing.T) {
	var (
		assert                     = assert.New(t)
		require                    = require.New(t)
		dispatcher, outbounds, err = NewEventDispatcher(NewTestOutboundMeasures(), nil, nil)
	)

	require.NotNil(dispatcher)
	require.NotNil(outbounds)
	require.NoError(err)

	dispatcher.OnDeviceEvent(&device.Event{
		Type:    device.MessageReceived,
		Message: &wrp.Message{Destination: "this is not a routable destination"},
	})

	assert.Equal(0, len(outbounds))
}

func testEventDispatcherOnDeviceEventBadURLFilter(t *testing.T) {
	var (
		assert                     = assert.New(t)
		dispatcher, outbounds, err = NewEventDispatcher(NewTestOutboundMeasures(), &Outbounder{DefaultScheme: "bad"}, nil)
	)

	assert.Nil(dispatcher)
	assert.Nil(outbounds)
	assert.Error(err)
}

func testEventDispatcherOnDeviceEventDispatchEvent(t *testing.T) {
	var (
		assert   = assert.New(t)
		require  = require.New(t)
		testData = []struct {
			outbounder        *Outbounder
			destination       string
			expectedEndpoints map[string]bool
		}{
			{
				outbounder:        nil,
				destination:       "event:iot",
				expectedEndpoints: map[string]bool{},
			},
			{
				outbounder:        &Outbounder{Method: "BADMETHOD&%*(!@(&%(", EventEndpoints: map[string]interface{}{"iot": []string{"http://endpoint1.com"}}},
				destination:       "event:iot",
				expectedEndpoints: map[string]bool{},
			},
			{
				outbounder:        &Outbounder{EventEndpoints: map[string]interface{}{"another": []string{"http://endpoint1.com"}}},
				destination:       "event:iot",
				expectedEndpoints: map[string]bool{},
			},
			{
				outbounder:        &Outbounder{EventEndpoints: map[string]interface{}{"another": []string{"http://endpoint1.com"}}},
				destination:       "event:iot",
				expectedEndpoints: map[string]bool{},
			},
			{
				outbounder:        &Outbounder{EventEndpoints: map[string]interface{}{"default": []string{"http://endpoint1.com"}}},
				destination:       "event:iot",
				expectedEndpoints: map[string]bool{"http://endpoint1.com": true},
			},
			{
				outbounder:        &Outbounder{Method: "PATCH", EventEndpoints: map[string]interface{}{"default": []string{"http://endpoint1.com"}}},
				destination:       "event:iot",
				expectedEndpoints: map[string]bool{"http://endpoint1.com": true},
			},
			{
				outbounder:        &Outbounder{EventEndpoints: map[string]interface{}{"default": []string{"http://endpoint1.com", "http://endpoint2.com"}}},
				destination:       "event:iot",
				expectedEndpoints: map[string]bool{"http://endpoint1.com": true, "http://endpoint2.com": true},
			},
			{
				outbounder:        &Outbounder{Method: "PATCH", EventEndpoints: map[string]interface{}{"default": []string{"http://endpoint1.com", "http://endpoint2.com"}}},
				destination:       "event:iot",
				expectedEndpoints: map[string]bool{"http://endpoint1.com": true, "http://endpoint2.com": true},
			},
			{
				outbounder:        &Outbounder{EventEndpoints: map[string]interface{}{"iot": []string{"http://endpoint1.com"}}},
				destination:       "event:iot",
				expectedEndpoints: map[string]bool{"http://endpoint1.com": true},
			},
			{
				outbounder:        &Outbounder{Method: "PATCH", EventEndpoints: map[string]interface{}{"iot": []string{"http://endpoint1.com"}}},
				destination:       "event:iot",
				expectedEndpoints: map[string]bool{"http://endpoint1.com": true},
			},
			{
				outbounder:        &Outbounder{EventEndpoints: map[string]interface{}{"iot": []string{"http://endpoint1.com", "http://endpoint2.com"}}},
				destination:       "event:iot",
				expectedEndpoints: map[string]bool{"http://endpoint1.com": true, "http://endpoint2.com": true},
			},
			{
				outbounder:        &Outbounder{Method: "PATCH", EventEndpoints: map[string]interface{}{"iot": []string{"http://endpoint1.com", "http://endpoint2.com"}}},
				destination:       "event:iot",
				expectedEndpoints: map[string]bool{"http://endpoint1.com": true, "http://endpoint2.com": true},
			},
		}
	)

	for _, record := range testData {
		for _, format := range []wrp.Format{wrp.Msgpack, wrp.JSON} {
			t.Logf("%#v, method=%s, format=%s", record, record.outbounder.method(), format)

			var (
				expectedContents           = []byte{1, 2, 3, 4}
				urlFilter                  = new(mockURLFilter)
				dispatcher, outbounds, err = NewEventDispatcher(NewTestOutboundMeasures(), record.outbounder, urlFilter)
			)

			require.NotNil(dispatcher)
			require.NotNil(outbounds)
			require.NoError(err)

			dispatcher.OnDeviceEvent(&device.Event{
				Type:     device.MessageReceived,
				Message:  &wrp.Message{Destination: record.destination},
				Format:   format,
				Contents: expectedContents,
			})

			assert.Equal(len(record.expectedEndpoints), len(outbounds), "incorrect envelope count")
			actualEndpoints := make(map[string]bool, len(record.expectedEndpoints))
			for len(outbounds) > 0 {
				select {
				case e := <-outbounds:
					e.cancel()
					<-e.request.Context().Done()

					assert.Equal(record.outbounder.method(), e.request.Method)
					assert.Equal(format.ContentType(), e.request.Header.Get("Content-Type"))

					urlString := e.request.URL.String()
					assert.False(actualEndpoints[urlString])
					actualEndpoints[urlString] = true

					actualContents, err := io.ReadAll(e.request.Body)
					assert.NoError(err)
					assert.Equal(expectedContents, actualContents)

				default:
				}
			}

			assert.Equal(record.expectedEndpoints, actualEndpoints)
			urlFilter.AssertExpectations(t)
		}
	}
}

func testEventDispatcherOnDeviceEventEventTimeout(t *testing.T) {
	var (
		require    = require.New(t)
		outbounder = &Outbounder{
			RequestTimeout: 100 * time.Millisecond,
			EventEndpoints: map[string]interface{}{"default": []string{"nowhere.com"}},
		}

		d, _, err = NewEventDispatcher(NewTestOutboundMeasures(), outbounder, nil)
	)

	require.NotNil(d)
	require.NoError(err)

	d.(*eventDispatcher).outbounds = make(chan outboundEnvelope)
	// TODO verify logger's buffer isn't empty
	d.OnDeviceEvent(&device.Event{
		Type:     device.MessageReceived,
		Message:  &wrp.Message{Destination: "event:iot"},
		Contents: []byte{1, 2},
	})
}

func testEventDispatcherOnDeviceEventFilterError(t *testing.T) {
	var (
		assert        = assert.New(t)
		require       = require.New(t)
		urlFilter     = new(mockURLFilter)
		expectedError = errors.New("expected")

		dispatcher, outbounds, err = NewEventDispatcher(NewTestOutboundMeasures(), nil, urlFilter)
	)

	require.NotNil(dispatcher)
	require.NotNil(outbounds)
	require.NoError(err)

	urlFilter.On("Filter", "doesnotmatter.com").Once().
		Return("", expectedError)

	dispatcher.OnDeviceEvent(&device.Event{
		Type:    device.MessageReceived,
		Message: &wrp.Message{Destination: "dns:doesnotmatter.com"},
	})

	// TODO verify logger's buffer isn't empty
	assert.Equal(0, len(outbounds))
	urlFilter.AssertExpectations(t)
}

func testEventDispatcherOnDeviceEventDispatchTo(t *testing.T) {
	var (
		assert   = assert.New(t)
		require  = require.New(t)
		testData = []struct {
			outbounder            *Outbounder
			destination           string
			expectedUnfilteredURL string
			expectedEndpoint      string
			expectsEnvelope       bool
		}{
			{
				outbounder:            nil,
				destination:           "dns:foobar.com",
				expectedUnfilteredURL: "foobar.com",
				expectedEndpoint:      "http://foobar.com",
				expectsEnvelope:       true,
			},
			{
				outbounder:            &Outbounder{Method: "PATCH"},
				destination:           "dns:foobar.com",
				expectedUnfilteredURL: "foobar.com",
				expectedEndpoint:      "http://foobar.com",
				expectsEnvelope:       true,
			},
			// TODO sync with john on how we want to handle authorization in eventDispatcher.newRequest(...)
			{
				outbounder:            &Outbounder{Method: "PATCH", AuthKey: "foobar"},
				destination:           "dns:foobar.com",
				expectedUnfilteredURL: "foobar.com",
				expectedEndpoint:      "http://foobar.com",
				expectsEnvelope:       true,
			},
			{
				outbounder:            &Outbounder{Method: "BADMETHOD$(*@#)*%"},
				destination:           "dns:foobar.com",
				expectedUnfilteredURL: "foobar.com",
				expectedEndpoint:      "http://foobar.com",
				expectsEnvelope:       false,
			},
			{
				outbounder:            nil,
				destination:           "dns:https://foobar.com",
				expectedUnfilteredURL: "https://foobar.com",
				expectedEndpoint:      "https://foobar.com",
				expectsEnvelope:       true,
			},
			{
				outbounder:            &Outbounder{Method: "BADMETHOD$(*@#)*%"},
				destination:           "dns:https://foobar.com",
				expectedUnfilteredURL: "https://foobar.com",
				expectedEndpoint:      "https://foobar.com",
				expectsEnvelope:       false,
			},
		}
	)

	for _, record := range testData {
		for _, format := range []wrp.Format{wrp.Msgpack, wrp.JSON} {
			t.Logf("%#v, method=%s, format=%s", record, record.outbounder.method(), format)

			var (
				expectedContents           = []byte{4, 7, 8, 1}
				urlFilter                  = new(mockURLFilter)
				dispatcher, outbounds, err = NewEventDispatcher(NewTestOutboundMeasures(), record.outbounder, urlFilter)
			)

			require.NotNil(dispatcher)
			require.NotNil(outbounds)
			require.NoError(err)

			urlFilter.On("Filter", record.expectedUnfilteredURL).Once().
				Return(record.expectedEndpoint, (error)(nil))

			dispatcher.OnDeviceEvent(&device.Event{
				Type:     device.MessageReceived,
				Message:  &wrp.Message{Destination: record.destination},
				Format:   format,
				Contents: expectedContents,
			})

			if !record.expectsEnvelope {
				assert.Equal(0, len(outbounds))
				continue
			}

			e := <-outbounds
			e.cancel()
			<-e.request.Context().Done()

			assert.Equal(record.outbounder.method(), e.request.Method)
			assert.Equal(format.ContentType(), e.request.Header.Get("Content-Type"))
			assert.Equal(record.expectedEndpoint, e.request.URL.String())

			actualContents, err := io.ReadAll(e.request.Body)
			assert.NoError(err)
			assert.Equal(expectedContents, actualContents)

			urlFilter.AssertExpectations(t)
		}
	}
}

func testEventDispatcherOnDeviceEventNilEventError(t *testing.T) {
	var (
		b bytes.Buffer
		e *device.Event
	)

	require := require.New(t)
	assert := assert.New(t)
	o := &Outbounder{}
	// NewJSONLogger is the default logger for the outbounder
	o.Logger = log.NewJSONLogger(&b)
	dp, _, err := NewEventDispatcher(NewTestOutboundMeasures(), o, nil)
	require.NotNil(dp)
	require.NoError(err)
	// Purge init logs
	b.Reset()

	dp.OnDeviceEvent(e)
	// Errors should have been logged
	assert.Greater(b.Len(), 0)
}

func testEventDispatcherOnDeviceEventEventMapError(t *testing.T) {
	assert := assert.New(t)
	o := &Outbounder{EventEndpoints: map[string]interface{}{"bad": -17.6}}
	dp, _, err := NewEventDispatcher(NewTestOutboundMeasures(), o, nil)
	assert.Nil(dp)
	assert.Error(err)
}

func TestEventDispatcherOnDeviceEvent(t *testing.T) {
	tests := []struct {
		description string
		test        func(*testing.T)
	}{
		{"ConnectEvent", testEventDispatcherOnDeviceEventConnectEvent},
		{"DisconnectEvent", testEventDispatcherOnDeviceEventDisconnectEvent},
		{"Unroutable", testEventDispatcherOnDeviceEventUnroutable},
		{"BadURLFilter", testEventDispatcherOnDeviceEventBadURLFilter},
		{"DispatchEvent", testEventDispatcherOnDeviceEventDispatchEvent},
		{"EventTimeout", testEventDispatcherOnDeviceEventEventTimeout},
		{"FilterError", testEventDispatcherOnDeviceEventFilterError},
		{"DispatchTo", testEventDispatcherOnDeviceEventDispatchTo},
		{"NilEventError", testEventDispatcherOnDeviceEventNilEventError},
		{"EventMapError", testEventDispatcherOnDeviceEventEventMapError},
	}

	for _, tc := range tests {
		t.Run(tc.description, tc.test)
	}
}
