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
	"io/ioutil"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
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

	deviceMetadata := genTestMetadata()

	d.On("ID").Return(device.ID("mac:123412341234"))
	d.On("Metadata").Return(deviceMetadata)
	d.On("Convey").Return(convey.C(nil))

	dispatcher.OnDeviceEvent(&device.Event{Type: device.Connect, Device: d})
	assert.Equal(0, len(outbounds))
	d.AssertExpectations(t)
}

func testDispatcherDisconnectEvent(t *testing.T) {
	var (
		assert                     = assert.New(t)
		require                    = require.New(t)
		d                          = new(device.MockDevice)
		dispatcher, outbounds, err = NewDispatcher(NewTestOutboundMeasures(), nil, nil)
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
	// TODO verify logger's buffer isn't empty
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

	// TODO verify logger's buffer isn't empty
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

func testDispatcherOnDeviceEventQOSAckEventFailure(t *testing.T) {
	tests := []struct {
		description string
		event       *device.Event
	}{
		// Failure case
		{
			description: "Invaild partnerIDs error, empty list",
			event: &device.Event{
				Device: new(device.MockDevice),
				Message: &wrp.Message{
					Type:             wrp.SimpleEventMessageType,
					PartnerIDs:       []string{},
					QualityOfService: wrp.QOSMediumValue,
				},
				Type: device.MessageReceived,
			},
		},
		{
			description: "Invaild partnerIDs error, more than 1",
			event: &device.Event{
				Device: new(device.MockDevice),
				Message: &wrp.Message{
					Type:             wrp.SimpleEventMessageType,
					PartnerIDs:       []string{"foo", "bar"},
					QualityOfService: wrp.QOSMediumValue,
				},
				Type: device.MessageReceived,
			},
		},
		{
			description: "Invaild empty message error",
			event: &device.Event{
				Device:  new(device.MockDevice),
				Message: &wrp.Message{},
				Type:    device.MessageReceived,
			},
		},
		{
			description: "Invaild empty event error",
			event:       &device.Event{},
		},
		{
			description: "Invalid nil event error",
			event:       nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {
			var b bytes.Buffer

			require := require.New(t)
			assert := assert.New(t)
			mQOSAckSuccess := new(mockCounter)
			mQOSAckFailure := new(mockCounter)
			mQOSAckSuccessLatency := new(mockHistogram)
			mQOSAckFailureLatency := new(mockHistogram)
			p, mt, qosl := "failure case", "failure case", "failure case"
			// Some tests have nil events
			if tc.event != nil {
				// Some tests have nil Messages
				if m, ok := tc.event.Message.(*wrp.Message); ok {
					qosl = m.QualityOfService.Level().String()
					mt = m.Type.FriendlyName()
					// Some messages will have invalid PartnerIDs
					if len(m.PartnerIDs) == 1 {
						p = m.PartnerIDs[0]
					}
				}
			}

			// Setup labels for metrics
			l := []string{qosLevelLabel, qosl, partnerIDLabel, p, messageType, mt}
			om := OutboundMeasures{
				QOSAckSuccess:        mQOSAckSuccess,
				QOSAckFailure:        mQOSAckFailure,
				QOSAckSuccessLatency: mQOSAckSuccessLatency,
				QOSAckFailureLatency: mQOSAckFailureLatency,
			}
			o := &Outbounder{}
			// NewJSONLogger is the default logger for the outbounder
			o.Logger = log.NewJSONLogger(&b)
			dpi, _, err := NewDispatcher(om, o, nil)
			require.NotNil(dpi)
			require.NoError(err)
			dp, ok := dpi.(*dispatcher)
			require.True(ok)
			// Purge init logs
			b.Reset()
			// Some tests have nil events
			if tc.event != nil {
				// Some tests have nil devices
				if d, ok := tc.event.Device.(*device.MockDevice); ok {
					// All tests should fail and never reach Device.Send
					d.On("Send", mock.AnythingOfType("*device.Request")).Panic("Func Device.Send should have not been called")
				}
			}

			// Setup mock panics
			mQOSAckSuccess.On("With", l).Panic("Func QOSAck.With should have not been called")
			mQOSAckSuccess.On("Add", 1.).Panic("Func QOSAck.Add should have not been called")
			mQOSAckFailure.On("With", l).Panic("Func QOSAckFailure.With should have not been called")
			mQOSAckFailure.On("Add", 1.).Panic("Func QOSAckFailure.Add should have not been called")
			mQOSAckSuccessLatency.On("With", l).Panic("Func QOSAckSuccessLatency.With should have not been called")
			mQOSAckSuccessLatency.On("Observe", mock.AnythingOfType("float64")).Panic("Func QOSAckSuccessLatency.Observe should have not been called")
			mQOSAckFailureLatency.On("With", l).Panic("Func QOSAckFailureLatency.With should have not been called")
			mQOSAckFailureLatency.On("Observe", mock.AnythingOfType("float64")).Panic("Func QOSAckFailureLatency.Observe should have not been called")

			// Ensure mock panics are not trigger
			require.NotPanics(func() { dp.qosAck(tc.event) })
			// Errors should have been logged
			assert.Greater(b.Len(), 0)
		})
	}
}

func testDispatcherOnDeviceEventNilEventFailure(t *testing.T) {
	var (
		b bytes.Buffer
		e *device.Event
	)

	require := require.New(t)
	assert := assert.New(t)
	o := &Outbounder{}
	// NewJSONLogger is the default logger for the outbounder
	o.Logger = log.NewJSONLogger(&b)
	dp, _, err := NewDispatcher(NewTestOutboundMeasures(), o, nil)
	require.NotNil(dp)
	require.NoError(err)
	// Purge init logs
	b.Reset()

	dp.OnDeviceEvent(e)
	// Errors should have been logged
	assert.Greater(b.Len(), 0)
}

func testDispatcherOnDeviceEventQOSAckDeviceFailure(t *testing.T) {
	var (
		expectedStatus                  int64 = 3471
		expectedRequestDeliveryResponse int64 = 34
		expectedIncludeSpans            bool  = true
	)

	tests := []struct {
		description string
		event       *device.Event
	}{
		// Failure case
		{
			description: "Device error",
			event: &device.Event{
				Device: new(device.MockDevice),
				Message: &wrp.Message{
					Type:                    wrp.SimpleEventMessageType,
					Source:                  "dns:external.com",
					Destination:             "MAC:11:22:33:44:55:66",
					TransactionUUID:         "DEADBEEF",
					ContentType:             "ContentType",
					Accept:                  "Accept",
					Status:                  &expectedStatus,
					RequestDeliveryResponse: &expectedRequestDeliveryResponse,
					Headers:                 []string{"Header1", "Header2"},
					Metadata:                map[string]string{"name": "value"},
					Spans:                   [][]string{{"1", "2"}, {"3"}},
					IncludeSpans:            &expectedIncludeSpans,
					Path:                    "/some/where/over/the/rainbow",
					Payload:                 []byte{1, 2, 3, 4, 0xff, 0xce},
					ServiceName:             "ServiceName",
					URL:                     "someURL.com",
					PartnerIDs:              []string{"foo"},
					SessionID:               "sessionID123",
					QualityOfService:        wrp.QOSMediumValue,
				},
				Type: device.MessageReceived,
			},
		},
		{
			description: "Invalid nil device error",
			event: &device.Event{
				Device: nil,
				Message: &wrp.Message{
					Type:                    wrp.SimpleEventMessageType,
					Source:                  "dns:external.com",
					Destination:             "MAC:11:22:33:44:55:66",
					TransactionUUID:         "DEADBEEF",
					ContentType:             "ContentType",
					Accept:                  "Accept",
					Status:                  &expectedStatus,
					RequestDeliveryResponse: &expectedRequestDeliveryResponse,
					Headers:                 []string{"Header1", "Header2"},
					Metadata:                map[string]string{"name": "value"},
					Spans:                   [][]string{{"1", "2"}, {"3"}},
					IncludeSpans:            &expectedIncludeSpans,
					Path:                    "/some/where/over/the/rainbow",
					Payload:                 []byte{1, 2, 3, 4, 0xff, 0xce},
					ServiceName:             "ServiceName",
					URL:                     "someURL.com",
					PartnerIDs:              []string{"foo"},
					SessionID:               "sessionID123",
					QualityOfService:        wrp.QOSMediumValue,
				},
				Type: device.MessageReceived,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {
			var b bytes.Buffer

			require := require.New(t)
			assert := assert.New(t)
			mQOSAckSuccess := new(mockCounter)
			mQOSAckFailure := new(mockCounter)
			mQOSAckSuccessLatency := new(mockHistogram)
			mQOSAckFailureLatency := new(mockHistogram)
			m, ok := tc.event.Message.(*wrp.Message)
			require.True(ok)
			// Setup labels for metrics
			l := []string{qosLevelLabel, m.QualityOfService.Level().String(), partnerIDLabel, m.PartnerIDs[0], messageType, m.Type.FriendlyName()}
			// Setup metrics for the dispatcher
			om := OutboundMeasures{
				QOSAckSuccess:        mQOSAckSuccess,
				QOSAckFailure:        mQOSAckFailure,
				QOSAckSuccessLatency: mQOSAckSuccessLatency,
				QOSAckFailureLatency: mQOSAckFailureLatency,
			}
			o := &Outbounder{}
			// Monitor logs for errors
			// NewJSONLogger is the default logger for the outbounder
			o.Logger = log.NewJSONLogger(&b)
			dpi, _, err := NewDispatcher(om, o, nil)
			require.NotNil(dpi)
			require.NoError(err)
			dp, ok := dpi.(*dispatcher)
			require.True(ok)
			// Purge init logs
			b.Reset()
			// Setup mock panics
			mQOSAckSuccess.On("With", l).Panic("Func QOSAck.With should have not been called")
			mQOSAckSuccess.On("Add", 1.).Panic("Func QOSAck.Add should have not been called")
			mQOSAckSuccessLatency.On("With", l).Panic("Func QOSAckSuccessLatency.With should have not been called")
			mQOSAckSuccessLatency.On("Observe", mock.AnythingOfType("float64")).Panic("Func QOSAckSuccessLatency.Observe should have not been called")
			d, ok := tc.event.Device.(*device.MockDevice)
			switch {
			case ok:
				// Setup mock calls
				d.On("Send", mock.AnythingOfType("*device.Request")).Return(nil, errors.New(""))
				mQOSAckFailure.On("With", l).Return().Once()
				mQOSAckFailure.On("Add", 1.).Return().Once()
				mQOSAckFailureLatency.On("With", l).Return().Once()
				mQOSAckFailureLatency.On("Observe", mock.AnythingOfType("float64")).Return().Once()
			default:
				// Setup mock panics
				mQOSAckFailure.On("With", l).Panic("Func QOSAckFailure.With should have not been called")
				mQOSAckFailure.On("Add", 1.).Panic("Func QOSAckFailure.Add should have not been called")
				mQOSAckFailureLatency.On("With", l).Panic("Func QOSAckFailureLatency.With should have not been called")
				mQOSAckFailureLatency.On("Observe", mock.AnythingOfType("float64")).Panic("Func QOSAckFailureLatency.Observe should have not been called")
			}

			// Ensure mock panics are not trigger
			require.NotPanics(func() { dp.qosAck(tc.event) })
			// Errors should have been logged
			assert.Greater(b.Len(), 0)
			// Some tests have nil devices
			if d != nil {
				// Ensure mock calls were made
				d.AssertExpectations(t)
				mQOSAckFailure.AssertExpectations(t)
				mQOSAckFailureLatency.AssertExpectations(t)
			}
		})
	}
}

func testDispatcherOnDeviceEventQOSAckFailure(t *testing.T) {
	tests := []struct {
		description string
		test        func(*testing.T)
	}{
		{"Event Failure", testDispatcherOnDeviceEventQOSAckEventFailure},
		{"Device Failure", testDispatcherOnDeviceEventQOSAckDeviceFailure},
	}

	for _, tc := range tests {
		t.Run(tc.description, tc.test)
	}
}

func testDispatcherOnDeviceEventQOSAckSuccess(t *testing.T) {
	var invaildMsg interface{ wrp.Typed }

	var (
		expectedStatus                  int64 = 3471
		expectedRequestDeliveryResponse int64 = 34
		expectedIncludeSpans            bool  = true
	)

	tests := []struct {
		description string
		event       *device.Event
		ack         bool
	}{
		// Success case, ack case
		{
			description: "Ack QOS level Medium",
			event: &device.Event{
				Device: new(device.MockDevice),
				Message: &wrp.Message{
					Type:                    wrp.SimpleEventMessageType,
					Source:                  "dns:external.com",
					Destination:             "MAC:11:22:33:44:55:66",
					TransactionUUID:         "DEADBEEF",
					ContentType:             "ContentType",
					Accept:                  "Accept",
					Status:                  &expectedStatus,
					RequestDeliveryResponse: &expectedRequestDeliveryResponse,
					Headers:                 []string{"Header1", "Header2"},
					Metadata:                map[string]string{"name": "value"},
					Spans:                   [][]string{{"1", "2"}, {"3"}},
					IncludeSpans:            &expectedIncludeSpans,
					Path:                    "/some/where/over/the/rainbow",
					Payload:                 []byte{1, 2, 3, 4, 0xff, 0xce},
					ServiceName:             "ServiceName",
					URL:                     "someURL.com",
					PartnerIDs:              []string{"foo"},
					SessionID:               "sessionID123",
					QualityOfService:        wrp.QOSMediumValue,
				},
				Type: device.MessageReceived,
			},
			ack: true,
		},
		{
			description: "Ack QOS level High success",
			event: &device.Event{
				Device: new(device.MockDevice),
				Message: &wrp.Message{
					Type:                    wrp.SimpleEventMessageType,
					Source:                  "dns:external.com",
					Destination:             "MAC:11:22:33:44:55:66",
					TransactionUUID:         "DEADBEEF",
					ContentType:             "ContentType",
					Accept:                  "Accept",
					Status:                  &expectedStatus,
					RequestDeliveryResponse: &expectedRequestDeliveryResponse,
					Headers:                 []string{"Header1", "Header2"},
					Metadata:                map[string]string{"name": "value"},
					Spans:                   [][]string{{"1", "2"}, {"3"}},
					IncludeSpans:            &expectedIncludeSpans,
					Path:                    "/some/where/over/the/rainbow",
					Payload:                 []byte{1, 2, 3, 4, 0xff, 0xce},
					ServiceName:             "ServiceName",
					URL:                     "someURL.com",
					PartnerIDs:              []string{"foo"},
					SessionID:               "sessionID123",
					QualityOfService:        wrp.QOSHighValue,
				},
				Type: device.MessageReceived,
			},
			ack: true,
		},
		{
			description: "Ack QOS level Critical",
			event: &device.Event{
				Device: new(device.MockDevice),
				Message: &wrp.Message{
					Type:                    wrp.SimpleEventMessageType,
					Source:                  "dns:external.com",
					Destination:             "MAC:11:22:33:44:55:66",
					TransactionUUID:         "DEADBEEF",
					ContentType:             "ContentType",
					Accept:                  "Accept",
					Status:                  &expectedStatus,
					RequestDeliveryResponse: &expectedRequestDeliveryResponse,
					Headers:                 []string{"Header1", "Header2"},
					Metadata:                map[string]string{"name": "value"},
					Spans:                   [][]string{{"1", "2"}, {"3"}},
					IncludeSpans:            &expectedIncludeSpans,
					Path:                    "/some/where/over/the/rainbow",
					Payload:                 []byte{1, 2, 3, 4, 0xff, 0xce},
					ServiceName:             "ServiceName",
					URL:                     "someURL.com",
					PartnerIDs:              []string{"foo"},
					SessionID:               "sessionID123",
					QualityOfService:        wrp.QOSCriticalValue,
				},
				Type: device.MessageReceived,
			},
			ack: true,
		},
		{
			description: "Ack QOS level Critical success with invalid message specs",
			event: &device.Event{
				Device: new(device.MockDevice),
				Message: &wrp.Message{
					Type: wrp.SimpleEventMessageType,
					// Empty Source
					Source: "",
					// Invalid Mac
					Destination:     "MAC:+++BB-44-55",
					TransactionUUID: "DEADBEEF",
					ContentType:     "ContentType",
					Headers:         []string{"Header1", "Header2"},
					Metadata:        map[string]string{"name": "value"},
					Path:            "/some/where/over/the/rainbow",
					Payload:         []byte{1, 2, 3, 4, 0xff, 0xce},
					ServiceName:     "ServiceName",
					// Not UFT8 URL string
					URL:              "someURL\xed\xbf\xbf.com",
					PartnerIDs:       []string{"foo"},
					SessionID:        "sessionID123",
					QualityOfService: wrp.QOSCriticalValue,
				},
				Type: device.MessageReceived,
			},
			ack: true,
		},
		// Success case, no ack case
		{
			description: "No ack invalid event type",
			event: &device.Event{
				Device: new(device.MockDevice),
				Message: &wrp.Message{
					Type:                    wrp.SimpleEventMessageType,
					Source:                  "dns:external.com",
					Destination:             "MAC:11:22:33:44:55:66",
					TransactionUUID:         "DEADBEEF",
					ContentType:             "ContentType",
					Accept:                  "Accept",
					Status:                  &expectedStatus,
					RequestDeliveryResponse: &expectedRequestDeliveryResponse,
					Headers:                 []string{"Header1", "Header2"},
					Metadata:                map[string]string{"name": "value"},
					Spans:                   [][]string{{"1", "2"}, {"3"}},
					IncludeSpans:            &expectedIncludeSpans,
					Path:                    "/some/where/over/the/rainbow",
					Payload:                 []byte{1, 2, 3, 4, 0xff, 0xce},
					ServiceName:             "ServiceName",
					URL:                     "someURL.com",
					PartnerIDs:              []string{"foo"},
					SessionID:               "sessionID123",
					QualityOfService:        wrp.QOSCriticalValue,
				},
				Type: device.TransactionComplete,
			},
		},
		{
			description: "No ack QOS level Low",
			event: &device.Event{
				Device: new(device.MockDevice),
				Message: &wrp.Message{
					Type:                    wrp.SimpleEventMessageType,
					Source:                  "dns:external.com",
					Destination:             "MAC:11:22:33:44:55:66",
					TransactionUUID:         "DEADBEEF",
					ContentType:             "ContentType",
					Accept:                  "Accept",
					Status:                  &expectedStatus,
					RequestDeliveryResponse: &expectedRequestDeliveryResponse,
					Headers:                 []string{"Header1", "Header2"},
					Metadata:                map[string]string{"name": "value"},
					Spans:                   [][]string{{"1", "2"}, {"3"}},
					IncludeSpans:            &expectedIncludeSpans,
					Path:                    "/some/where/over/the/rainbow",
					Payload:                 []byte{1, 2, 3, 4, 0xff, 0xce},
					ServiceName:             "ServiceName",
					URL:                     "someURL.com",
					PartnerIDs:              []string{"foo"},
					SessionID:               "sessionID123",
					QualityOfService:        wrp.QOSLowValue,
				},
				Type: device.MessageReceived,
			},
		},
		{
			description: "No ack non SimpleEventMessageType message",
			event: &device.Event{
				Device: new(device.MockDevice),
				Message: &wrp.Message{
					Type: wrp.Invalid0MessageType,
					// Empty Source
					Source: "",
					// Invalid Mac
					Destination:     "MAC:+++BB-44-55",
					TransactionUUID: "DEADBEEF",
					ContentType:     "ContentType",
					Headers:         []string{"Header1", "Header2"},
					Metadata:        map[string]string{"name": "value"},
					Path:            "/some/where/over/the/rainbow",
					Payload:         []byte{1, 2, 3, 4, 0xff, 0xce},
					ServiceName:     "ServiceName",
					// Not UFT8 URL string
					URL:              "someURL\xed\xbf\xbf.com",
					PartnerIDs:       []string{"foo"},
					SessionID:        "sessionID123",
					QualityOfService: wrp.QOSLowValue,
				},
				Type: device.MessageReceived,
			},
		},
		{
			description: "Invaild message error",
			event: &device.Event{
				Device:  new(device.MockDevice),
				Message: invaildMsg,
				Type:    device.MessageReceived,
			},
		},
		{
			description: "Invaild nil message",
			event: &device.Event{
				Device:  new(device.MockDevice),
				Message: nil,
				Type:    device.MessageReceived,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.description, func(t *testing.T) {
			var b bytes.Buffer

			require := require.New(t)
			assert := assert.New(t)
			mQOSAckSuccess := new(mockCounter)
			mQOSAckFailure := new(mockCounter)
			mQOSAckSuccessLatency := new(mockHistogram)
			mQOSAckFailureLatency := new(mockHistogram)
			p, mt, qosl := "failure case", "failure case", "failure case"
			// Some tests have invalid or nil messages
			if m, ok := tc.event.Message.(*wrp.Message); ok {
				qosl = m.QualityOfService.Level().String()
				mt = m.Type.FriendlyName()
				// Some messages will have invalid PartnerIDs
				if len(m.PartnerIDs) == 1 {
					p = m.PartnerIDs[0]
				}
			}

			// Setup labels for metrics
			l := []string{qosLevelLabel, qosl, partnerIDLabel, p, messageType, mt}
			// Setup metrics for the dispatcher
			om := OutboundMeasures{
				QOSAckSuccess:        mQOSAckSuccess,
				QOSAckFailure:        mQOSAckFailure,
				QOSAckSuccessLatency: mQOSAckSuccessLatency,
				QOSAckFailureLatency: mQOSAckFailureLatency,
			}
			o := &Outbounder{}
			// Monitor logs for errors
			// NewJSONLogger is the default logger for the outbounder
			o.Logger = log.NewJSONLogger(&b)
			dpi, _, err := NewDispatcher(om, o, nil)
			require.NotNil(dpi)
			require.NoError(err)
			dp, ok := dpi.(*dispatcher)
			require.True(ok)
			// Purge init logs
			b.Reset()
			// Setup mock panics
			mQOSAckFailure.On("With", l).Panic("Func QOSAckFailure.With should have not been called")
			mQOSAckFailure.On("Add", 1.).Panic("Func QOSAckFailure.Add should have not been called")
			mQOSAckFailureLatency.On("With", l).Panic("Func QOSAckFailureLatency.With should have not been called")
			mQOSAckFailureLatency.On("Observe", mock.AnythingOfType("float64")).Panic("Func QOSAckFailureLatency.Observe should have not been called")
			d, ok := tc.event.Device.(*device.MockDevice)
			require.True(ok)
			switch {
			case tc.ack:
				// Setup mock calls
				d.On("Send", mock.AnythingOfType("*device.Request")).Return(nil, error(nil))
				mQOSAckSuccess.On("With", l).Return().Once()
				mQOSAckSuccess.On("Add", 1.).Return().Once()
				mQOSAckSuccessLatency.On("With", l).Return().Once()
				mQOSAckSuccessLatency.On("Observe", mock.AnythingOfType("float64")).Return().Once()
			default:
				// Setup mock panics
				d.On("Send", mock.AnythingOfType("*device.Request")).Panic("Func Device.Send should have not been called")
				mQOSAckSuccess.On("With", l).Panic("Func QOSAck.With should have not been called")
				mQOSAckSuccess.On("Add", 1.).Panic("Func QOSAck.Add should have not been called")
				mQOSAckSuccessLatency.On("With", l).Panic("Func QOSAckSuccessLatency.With should have not been called")
				mQOSAckSuccessLatency.On("Observe", mock.AnythingOfType("float64")).Panic("Func QOSAckSuccessLatency.Observe should have not been called")
			}

			// Ensure mock panics are not trigger
			require.NotPanics(func() { dp.qosAck(tc.event) })
			// No errors should have been logged
			assert.Zero(b.Len())
			if tc.ack {
				// Ensure mock calls were made
				d.AssertExpectations(t)
				mQOSAckSuccess.AssertExpectations(t)
				mQOSAckSuccessLatency.AssertExpectations(t)
			}
		})
	}
}

func testDispatcherOnDeviceEventQOSAck(t *testing.T) {
	tests := []struct {
		description string
		test        func(*testing.T)
	}{
		{"Success", testDispatcherOnDeviceEventQOSAckSuccess},
		{"Failure", testDispatcherOnDeviceEventQOSAckFailure},
	}

	for _, tc := range tests {
		t.Run(tc.description, tc.test)
	}
}

func testDispatcherOnDeviceEvent(t *testing.T) {
	tests := []struct {
		description string
		test        func(*testing.T)
	}{
		{"DispatchEvent", testDispatcherOnDeviceEventDispatchEvent},
		{"EventTimeout", testDispatcherOnDeviceEventEventTimeout},
		{"FilterError", testDispatcherOnDeviceEventFilterError},
		{"DispatchTo", testDispatcherOnDeviceEventDispatchTo},
		{"NilEventError", testDispatcherOnDeviceEventNilEventFailure},
		{"QOSAck", testDispatcherOnDeviceEventQOSAck},
	}

	for _, tc := range tests {
		t.Run(tc.description, tc.test)
	}
}

func TestDispatcher(t *testing.T) {
	tests := []struct {
		description string
		test        func(*testing.T)
	}{
		{"ConnectEvent", testDispatcherConnectEvent},
		{"DisconnectEvent", testDispatcherDisconnectEvent},
		{"Unroutable", testDispatcherUnroutable},
		{"BadURLFilter", testDispatcherBadURLFilter},
		{"OnDeviceEvent", testDispatcherOnDeviceEvent},
	}

	for _, tc := range tests {
		t.Run(tc.description, tc.test)
	}
}
