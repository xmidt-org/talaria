// SPDX-FileCopyrightText: 2017 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xmidt-org/webpa-common/v2/convey"
	"github.com/xmidt-org/webpa-common/v2/device"
	"github.com/xmidt-org/wrp-go/v3"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
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
		assert  = assert.New(t)
		require = require.New(t)
		d       = new(device.MockDevice)
	)

	om, err := NewTestOutboundMeasures()
	require.NoError(err)
	dispatcher, outbounds, err := NewEventDispatcher(om, nil, nil)
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
		assert  = assert.New(t)
		require = require.New(t)
		d       = new(device.MockDevice)
	)

	om, err := NewTestOutboundMeasures()
	require.NoError(err)
	dispatcher, outbounds, err := NewEventDispatcher(om, nil, nil)
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
		assert  = assert.New(t)
		require = require.New(t)
	)

	om, err := NewTestOutboundMeasures()
	require.NoError(err)
	dispatcher, outbounds, err := NewEventDispatcher(om, nil, nil)
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
		assert  = assert.New(t)
		require = require.New(t)
	)

	om, err := NewTestOutboundMeasures()
	require.NoError(err)
	dispatcher, outbounds, err := NewEventDispatcher(om, &Outbounder{DefaultScheme: "bad"}, nil)
	assert.Nil(dispatcher)
	assert.Nil(outbounds)
	assert.Error(err)
}

func testEventDispatcherOnDeviceEventDispatchEvent(t *testing.T) {
	var (
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
		t.Run(record.destination, func(t *testing.T) {
			assert := assert.New(t)
			require := require.New(t)
			for _, format := range []wrp.Format{wrp.Msgpack, wrp.JSON} {
				t.Logf("%#v, method=%s, format=%s", record, record.outbounder.method(), format)

				var (
					expectedContents = []byte{1, 2, 3, 4}
					urlFilter        = new(mockURLFilter)
				)

				om, err := NewTestOutboundMeasures()
				require.NoError(err)
				dispatcher, outbounds, err := NewEventDispatcher(om, record.outbounder, urlFilter)
				require.NotNil(dispatcher)
				require.NotNil(outbounds)
				require.NoError(err)

				testDevice := device.MockDevice{}
				testDevice.On("Metadata").Return(genTestMetadata())
				dispatcher.OnDeviceEvent(&device.Event{
					Type:     device.MessageReceived,
					Message:  &wrp.Message{Destination: record.destination},
					Format:   format,
					Contents: expectedContents,
					Device:   &testDevice,
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
		})
	}
}

func testEventDispatcherOnDeviceEventFullQueue(t *testing.T) {
	var (
		b                 bytes.Buffer
		assert            = assert.New(t)
		require           = require.New(t)
		expectedEventType = "node-change"

		outbounder = &Outbounder{
			RequestTimeout: 100 * time.Millisecond,
			EventEndpoints: map[string]interface{}{"default": []string{"nowhere.com"}},
			Logger: zap.New(
				zapcore.NewCore(zapcore.NewJSONEncoder(
					zapcore.EncoderConfig{
						MessageKey: "message",
					}), zapcore.AddSync(&b), zapcore.ErrorLevel),
			),
		}
	)

	om, err := NewTestOutboundMeasures()
	require.NoError(err)
	dm := new(mockCounter)
	om.DroppedMessages = dm
	d, _, err := NewEventDispatcher(om, outbounder, nil)

	require.NotNil(d)
	require.NoError(err)

	d.(*eventDispatcher).outbounds = make(chan outboundEnvelope)
	dm.On("With", prometheus.Labels{eventLabel: expectedEventType, codeLabel: messageDroppedCode, reasonLabel: fullQueueReason, urlLabel: "nowhere.com"}).Return().Once()
	dm.On("Add", 1.).Return().Once()
	testDevice := device.MockDevice{}
	metadata := new(device.Metadata)
	metadata.SetClaims(map[string]interface{}{
		device.PartnerIDClaimKey: "partner-1",
		device.TrustClaimKey:     1000,
	})
	testDevice.On("Metadata").Return(metadata)
	d.OnDeviceEvent(&device.Event{
		Type:     device.MessageReceived,
		Message:  &wrp.Message{Destination: fmt.Sprintf("event:%s/mac:11:22:33:44:55:66/Online/unknown/deb2eb69999", expectedEventType)},
		Contents: []byte{1, 2},
		Device:   &testDevice,
	})
	assert.Greater(b.Len(), 0)
	dm.AssertExpectations(t)
}

func testEventDispatcherOnDeviceEventMessageReceived(t *testing.T) {
	var (
		assert            = assert.New(t)
		require           = require.New(t)
		b                 bytes.Buffer
		expectedEventType = "node-change"
		m                 = wrp.Message{Destination: fmt.Sprintf("event:%s/mac:11:22:33:44:55:66/Online/unknown/deb2eb69999", expectedEventType)}
		o                 = Outbounder{
			Method:         "PATCH",
			EventEndpoints: map[string]interface{}{"default": []string{"nowhere.com"}},
			Logger: zap.New(
				zapcore.NewCore(zapcore.NewJSONEncoder(
					zapcore.EncoderConfig{
						MessageKey: "message",
					}), zapcore.AddSync(&b), zapcore.ErrorLevel),
			),
		}
	)

	om, err := NewTestOutboundMeasures()
	require.NoError(err)
	dispatcher, outbounds, err := NewEventDispatcher(om, &o, nil)
	require.NotNil(dispatcher)
	require.NotNil(outbounds)
	require.NoError(err)

	testDevice := device.MockDevice{}
	testDevice.On("Metadata").Return(genTestMetadata())
	dispatcher.OnDeviceEvent(&device.Event{
		Type:    device.MessageReceived,
		Message: &m,
		Device:  &testDevice,
	})

	require.Equal(1, len(outbounds))
	e := <-outbounds
	e.cancel()
	<-e.request.Context().Done()

	assert.Equal(o.method(), e.request.Method)
	assert.Zero(b)
	eventType, ok := e.request.Context().Value(eventTypeContextKey{}).(string)
	require.True(ok)
	assert.Equal(expectedEventType, eventType)
}

func testEventDispatcherOnDeviceEventFilterError(t *testing.T) {
	var (
		assert        = assert.New(t)
		require       = require.New(t)
		urlFilter     = new(mockURLFilter)
		expectedError = errors.New("expected")
		b             bytes.Buffer
		o             = Outbounder{
			Method:         "PATCH",
			EventEndpoints: map[string]interface{}{"default": []string{"nowhere.com"}},
			Logger: zap.New(
				zapcore.NewCore(zapcore.NewJSONEncoder(
					zapcore.EncoderConfig{
						MessageKey: "message",
					}), zapcore.AddSync(&b), zapcore.ErrorLevel),
			),
		}
	)

	om, err := NewTestOutboundMeasures()
	require.NoError(err)
	dispatcher, outbounds, err := NewEventDispatcher(om, &o, urlFilter)
	require.NotNil(dispatcher)
	require.NotNil(outbounds)
	require.NoError(err)

	urlFilter.On("Filter", "doesnotmatter.com").Once().
		Return("", expectedError)

	testDevice := device.MockDevice{}
	testDevice.On("Metadata").Return(genTestMetadata())
	dispatcher.OnDeviceEvent(&device.Event{
		Type:    device.MessageReceived,
		Message: &wrp.Message{Destination: "dns:doesnotmatter.com"},
		Device:  &testDevice,
	})

	assert.Equal(0, len(outbounds))
	assert.NotZero(b)
	urlFilter.AssertExpectations(t)
}

func testEventDispatcherOnDeviceEventDispatchTo(t *testing.T) {
	var (
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
		t.Run(record.destination, func(t *testing.T) {
			assert := assert.New(t)
			require := require.New(t)
			for _, format := range []wrp.Format{wrp.Msgpack, wrp.JSON} {
				t.Logf("%#v, method=%s, format=%s", record, record.outbounder.method(), format)

				var (
					expectedContents = []byte{4, 7, 8, 1}
					urlFilter        = new(mockURLFilter)
				)

				om, err := NewTestOutboundMeasures()
				require.NoError(err)
				dispatcher, outbounds, err := NewEventDispatcher(om, record.outbounder, urlFilter)
				require.NotNil(dispatcher)
				require.NotNil(outbounds)
				require.NoError(err)

				urlFilter.On("Filter", record.expectedUnfilteredURL).Once().
					Return(record.expectedEndpoint, (error)(nil))

				testDevice := device.MockDevice{}
				testDevice.On("Metadata").Return(genTestMetadata())
				dispatcher.OnDeviceEvent(&device.Event{
					Type:     device.MessageReceived,
					Message:  &wrp.Message{Destination: record.destination},
					Format:   format,
					Contents: expectedContents,
					Device:   &testDevice,
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
		})
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
	logger := zap.New(
		zapcore.NewCore(zapcore.NewJSONEncoder(
			zapcore.EncoderConfig{
				MessageKey: "message",
			}), zapcore.AddSync(&b), zapcore.ErrorLevel),
	)
	o.Logger = logger
	om, err := NewTestOutboundMeasures()
	require.NoError(err)
	dp, _, err := NewEventDispatcher(om, o, nil)
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
	require := require.New(t)
	o := &Outbounder{EventEndpoints: map[string]interface{}{"bad": -17.6}}
	om, err := NewTestOutboundMeasures()
	require.NoError(err)
	dp, _, err := NewEventDispatcher(om, o, nil)
	assert.Nil(dp)
	assert.Error(err)
}

func TestEventDispatcherOnDeviceEvent(t *testing.T) {
	// TODO improve tests by included 0 trust cases
	tests := []struct {
		description string
		test        func(*testing.T)
	}{
		{"ConnectEvent", testEventDispatcherOnDeviceEventConnectEvent},
		{"CorrectEventType", testEventDispatcherOnDeviceEventMessageReceived},
		{"DisconnectEvent", testEventDispatcherOnDeviceEventDisconnectEvent},
		{"Unroutable", testEventDispatcherOnDeviceEventUnroutable},
		{"BadURLFilter", testEventDispatcherOnDeviceEventBadURLFilter},
		{"DispatchEvent", testEventDispatcherOnDeviceEventDispatchEvent},
		{"FullQueue", testEventDispatcherOnDeviceEventFullQueue},
		{"FilterError", testEventDispatcherOnDeviceEventFilterError},
		{"DispatchTo", testEventDispatcherOnDeviceEventDispatchTo},
		{"NilEventError", testEventDispatcherOnDeviceEventNilEventError},
		{"EventMapError", testEventDispatcherOnDeviceEventEventMapError},
	}

	for _, tc := range tests {
		t.Run(tc.description, tc.test)
	}
}
