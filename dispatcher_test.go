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
	"errors"
	"io/ioutil"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xmidt-org/webpa-common/convey"
	"github.com/xmidt-org/webpa-common/device"
	"github.com/xmidt-org/webpa-common/secure"
	"github.com/xmidt-org/wrp-go/v2"
)

func testDispatcherConnectEvent(t *testing.T) {
	var (
		assert                     = assert.New(t)
		require                    = require.New(t)
		d                          = new(device.MockDevice)
		dispatcher, outbounds, err = NewDispatcher(NewTestOutboundMeasures(), nil, nil)
	)

	require.NotNil(dispatcher)
	require.NotNil(outbounds)
	require.NoError(err)

	d.On("ID").Return(device.ID("mac:123412341234"))
	d.On("PartnerIDs").Return([]string{"partner-1"})
	d.On("SessionID").Return("sessionID")

	d.On("Convey").Return(convey.C(nil))
	d.On("Trust").Return(secure.Untrusted)
	d.On("ConveyCompliance").Return(convey.Full)

	dispatcher.OnDeviceEvent(&device.Event{Type: device.Connect, Device: d})
	assert.Equal(0, len(outbounds))
	d.AssertExpectations(t)
}

func testDispatcherUnroutable(t *testing.T) {
	var (
		assert                     = assert.New(t)
		require                    = require.New(t)
		dispatcher, outbounds, err = NewDispatcher(NewTestOutboundMeasures(), nil, nil)
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

func testDispatcherBadURLFilter(t *testing.T) {
	var (
		assert                     = assert.New(t)
		dispatcher, outbounds, err = NewDispatcher(NewTestOutboundMeasures(), &Outbounder{DefaultScheme: "bad"}, nil)
	)

	assert.Nil(dispatcher)
	assert.Nil(outbounds)
	assert.Error(err)
}

func testDispatcherOnDeviceEventDispatchEvent(t *testing.T) {
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
				dispatcher, outbounds, err = NewDispatcher(NewTestOutboundMeasures(), record.outbounder, urlFilter)
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

					actualContents, err := ioutil.ReadAll(e.request.Body)
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

func testDispatcherOnDeviceEventEventTimeout(t *testing.T) {
	var (
		require    = require.New(t)
		outbounder = &Outbounder{
			RequestTimeout: 100 * time.Millisecond,
			EventEndpoints: map[string]interface{}{"default": []string{"nowhere.com"}},
		}

		d, _, err = NewDispatcher(NewTestOutboundMeasures(), outbounder, nil)
	)

	require.NotNil(d)
	require.NoError(err)

	d.(*dispatcher).outbounds = make(chan outboundEnvelope)
	d.OnDeviceEvent(&device.Event{
		Type:     device.MessageReceived,
		Message:  &wrp.Message{Destination: "event:iot"},
		Contents: []byte{1, 2},
	})
}

func testDispatcherOnDeviceEventFilterError(t *testing.T) {
	var (
		assert        = assert.New(t)
		require       = require.New(t)
		urlFilter     = new(mockURLFilter)
		expectedError = errors.New("expected")

		dispatcher, outbounds, err = NewDispatcher(NewTestOutboundMeasures(), nil, urlFilter)
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

	assert.Equal(0, len(outbounds))
	urlFilter.AssertExpectations(t)
}

func testDispatcherOnDeviceEventDispatchTo(t *testing.T) {
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
				dispatcher, outbounds, err = NewDispatcher(NewTestOutboundMeasures(), record.outbounder, urlFilter)
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

			actualContents, err := ioutil.ReadAll(e.request.Body)
			assert.NoError(err)
			assert.Equal(expectedContents, actualContents)

			urlFilter.AssertExpectations(t)
		}
	}
}

func TestDispatcher(t *testing.T) {
	t.Run("ConnectEvent", testDispatcherConnectEvent)
	t.Run("Unroutable", testDispatcherUnroutable)
	t.Run("BadURLFilter", testDispatcherBadURLFilter)
	t.Run("OnDeviceEvent", func(t *testing.T) {
		t.Run("DispatchEvent", testDispatcherOnDeviceEventDispatchEvent)
		t.Run("EventTimeout", testDispatcherOnDeviceEventEventTimeout)
		t.Run("FilterError", testDispatcherOnDeviceEventFilterError)
		t.Run("DispatchTo", testDispatcherOnDeviceEventDispatchTo)
	})
}
