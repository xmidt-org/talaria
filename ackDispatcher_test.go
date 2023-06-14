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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/xmidt-org/webpa-common/v2/device"
	"github.com/xmidt-org/wrp-go/v3"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const failure_case = "failure case"

func testAckDispatcherOnDeviceEventQOSEventFailure(t *testing.T) {
	tests := []struct {
		description string
		event       *device.Event
	}{
		// Failure case
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
			mAckSuccess := new(mockCounter)
			mAckFailure := new(mockCounter)
			mAckSuccessLatency := new(mockHistogram)
			mAckFailureLatency := new(mockHistogram)
			p, mt, qosl := failure_case, failure_case, failure_case
			// Setup labels for metrics
			l := []string{qosLevelLabel, qosl, partnerIDLabel, p, messageType, mt}
			om := OutboundMeasures{
				AckSuccess:        mAckSuccess,
				AckFailure:        mAckFailure,
				AckSuccessLatency: mAckSuccessLatency,
				AckFailureLatency: mAckFailureLatency,
			}
			o := &Outbounder{}
			logger := zap.New(
				zapcore.NewCore(zapcore.NewJSONEncoder(
					zapcore.EncoderConfig{
						MessageKey: "message",
					}), zapcore.AddSync(&b), zapcore.ErrorLevel),
			)
			o.Logger = logger
			dp, err := NewAckDispatcher(om, o)
			require.NotNil(dp)
			require.NoError(err)
			// Purge init logs
			b.Reset()

			// Setup mock panics
			mAckSuccess.On("With", l).Panic("Func Ack.With should have not been called")
			mAckSuccess.On("Add", 1.).Panic("Func Ack.Add should have not been called")
			mAckFailure.On("With", l).Panic("Func AckFailure.With should have not been called")
			mAckFailure.On("Add", 1.).Panic("Func AckFailure.Add should have not been called")
			mAckSuccessLatency.On("With", l).Panic("Func AckSuccessLatency.With should have not been called")
			mAckSuccessLatency.On("Observe", mock.AnythingOfType("float64")).Panic("Func AckSuccessLatency.Observe should have not been called")
			mAckFailureLatency.On("With", l).Panic("Func AckFailureLatency.With should have not been called")
			mAckFailureLatency.On("Observe", mock.AnythingOfType("float64")).Panic("Func AckFailureLatency.Observe should have not been called")

			// Ensure mock panics are not trigger
			require.NotPanics(func() { dp.OnDeviceEvent(tc.event) })
			// Errors should have been logged
			assert.Greater(b.Len(), 0)
		})
	}
}

func testAckDispatcherOnDeviceEventQOSDeviceFailure(t *testing.T) {
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
			mAckSuccess := new(mockCounter)
			mAckFailure := new(mockCounter)
			mAckSuccessLatency := new(mockHistogram)
			mAckFailureLatency := new(mockHistogram)
			m, ok := tc.event.Message.(*wrp.Message)
			require.True(ok)
			// Setup labels for metrics
			dm := genTestMetadata()
			l := []string{qosLevelLabel, m.QualityOfService.Level().String(), partnerIDLabel, dm.PartnerIDClaim(), messageType, m.Type.FriendlyName()}
			// Setup metrics for the dispatcher
			om := OutboundMeasures{
				AckSuccess:        mAckSuccess,
				AckFailure:        mAckFailure,
				AckSuccessLatency: mAckSuccessLatency,
				AckFailureLatency: mAckFailureLatency,
			}
			o := &Outbounder{}
			// Monitor logs for errors
			logger := zap.New(
				zapcore.NewCore(zapcore.NewJSONEncoder(
					zapcore.EncoderConfig{
						MessageKey: "message",
					}), zapcore.AddSync(&b), zapcore.ErrorLevel),
			)
			o.Logger = logger
			dp, err := NewAckDispatcher(om, o)
			require.NotNil(dp)
			require.NoError(err)
			// Purge init logs
			b.Reset()
			// Setup mock panics
			mAckSuccess.On("With", l).Panic("Func Ack.With should have not been called")
			mAckSuccess.On("Add", 1.).Panic("Func Ack.Add should have not been called")
			mAckSuccessLatency.On("With", l).Panic("Func AckSuccessLatency.With should have not been called")
			mAckSuccessLatency.On("Observe", mock.AnythingOfType("float64")).Panic("Func AckSuccessLatency.Observe should have not been called")
			d, ok := tc.event.Device.(*device.MockDevice)
			switch {
			case ok:
				// Setup mock calls
				d.On("Send", mock.AnythingOfType("*device.Request")).Return(nil, errors.New(""))
				d.On("Metadata").Return(dm)
				mAckFailure.On("With", l).Return().Once()
				mAckFailure.On("Add", 1.).Return().Once()
				mAckFailureLatency.On("With", l).Return().Once()
				mAckFailureLatency.On("Observe", mock.AnythingOfType("float64")).Return().Once()

			default:
				// Setup mock panics
				mAckFailure.On("With", l).Panic("Func AckFailure.With should have not been called")
				mAckFailure.On("Add", 1.).Panic("Func AckFailure.Add should have not been called")
				mAckFailureLatency.On("With", l).Panic("Func AckFailureLatency.With should have not been called")
				mAckFailureLatency.On("Observe", mock.AnythingOfType("float64")).Panic("Func AckFailureLatency.Observe should have not been called")
			}

			// Ensure mock panics are not trigger
			require.NotPanics(func() { dp.OnDeviceEvent(tc.event) })
			// Errors should have been logged
			assert.Greater(b.Len(), 0)
			// Some tests have nil devices
			if d != nil {
				// Ensure mock calls were made
				d.AssertExpectations(t)
				mAckFailure.AssertExpectations(t)
				mAckFailureLatency.AssertExpectations(t)
			}
		})
	}
}

func testAckDispatcherOnDeviceEventQOSFailure(t *testing.T) {
	tests := []struct {
		description string
		test        func(*testing.T)
	}{
		{"Event Failure", testAckDispatcherOnDeviceEventQOSEventFailure},
		{"Device Failure", testAckDispatcherOnDeviceEventQOSDeviceFailure},
	}

	for _, tc := range tests {
		t.Run(tc.description, tc.test)
	}
}

func testAckDispatcherOnDeviceEventQOSSuccess(t *testing.T) {
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
			mAckSuccess := new(mockCounter)
			mAckFailure := new(mockCounter)
			mAckSuccessLatency := new(mockHistogram)
			mAckFailureLatency := new(mockHistogram)
			dm := genTestMetadata()
			p, mt, qosl := dm.PartnerIDClaim(), failure_case, failure_case
			// Some tests have invalid or nil messages
			if m, ok := tc.event.Message.(*wrp.Message); ok {
				qosl = m.QualityOfService.Level().String()
				mt = m.Type.FriendlyName()
			}

			// Setup labels for metrics
			l := []string{qosLevelLabel, qosl, partnerIDLabel, p, messageType, mt}
			// Setup metrics for the dispatcher
			om := OutboundMeasures{
				AckSuccess:        mAckSuccess,
				AckFailure:        mAckFailure,
				AckSuccessLatency: mAckSuccessLatency,
				AckFailureLatency: mAckFailureLatency,
			}
			o := &Outbounder{}
			// Monitor logs for errors
			logger := zap.New(
				zapcore.NewCore(zapcore.NewJSONEncoder(
					zapcore.EncoderConfig{
						MessageKey: "message",
					}), zapcore.AddSync(&b), zapcore.ErrorLevel),
			)
			o.Logger = logger
			dp, err := NewAckDispatcher(om, o)
			require.NotNil(dp)
			require.NoError(err)
			// Purge init logs
			b.Reset()
			// Setup mock panics
			mAckFailure.On("With", l).Panic("Func AckFailure.With should have not been called")
			mAckFailure.On("Add", 1.).Panic("Func AckFailure.Add should have not been called")
			mAckFailureLatency.On("With", l).Panic("Func AckFailureLatency.With should have not been called")
			mAckFailureLatency.On("Observe", mock.AnythingOfType("float64")).Panic("Func AckFailureLatency.Observe should have not been called")
			d, ok := tc.event.Device.(*device.MockDevice)
			require.True(ok)
			d.On("Metadata").Return(dm)
			switch {
			case tc.ack:
				// Setup mock calls
				d.On("Send", mock.AnythingOfType("*device.Request")).Return(nil, error(nil))
				mAckSuccess.On("With", l).Return().Once()
				mAckSuccess.On("Add", 1.).Return().Once()
				mAckSuccessLatency.On("With", l).Return().Once()
				mAckSuccessLatency.On("Observe", mock.AnythingOfType("float64")).Return().Once()

			default:
				// Setup mock panics
				d.On("Send", mock.AnythingOfType("*device.Request")).Panic("Func Device.Send should have not been called")
				mAckSuccess.On("With", l).Panic("Func Ack.With should have not been called")
				mAckSuccess.On("Add", 1.).Panic("Func Ack.Add should have not been called")
				mAckSuccessLatency.On("With", l).Panic("Func AckSuccessLatency.With should have not been called")
				mAckSuccessLatency.On("Observe", mock.AnythingOfType("float64")).Panic("Func AckSuccessLatency.Observe should have not been called")
			}

			// Ensure mock panics are not trigger
			require.NotPanics(func() { dp.OnDeviceEvent(tc.event) })
			// No errors should have been logged
			assert.Zero(b.Len())
			if tc.ack {
				// Ensure mock calls were made
				d.AssertExpectations(t)
				mAckSuccess.AssertExpectations(t)
				mAckSuccessLatency.AssertExpectations(t)
			}
		})
	}
}

func testAckDispatcherOnDeviceEventQOS(t *testing.T) {
	/*
		Ack conditions (WRP):
		- Implements [basic wrp spec](https://github.com/xmidt-org/wrp-go/blob/main/spec_validator.go) https://xmidt.io/docs/wrp/basics
		- Message type is 4 (SimpleEventMessageType)
		- Any qos above [QOSLowValue [QOSLowValue < 25]](https://github.com/xmidt-org/wrp-go/blob/main/qos.go)

		Prometheus metrics:
		- outbound_ack_success
		- outbound_ack_failure
		- outbound_ack_success_latency_seconds
		- outbound_ack_failure_latency_seconds
	*/

	tests := []struct {
		description string
		test        func(*testing.T)
	}{
		{"Success", testAckDispatcherOnDeviceEventQOSSuccess},
		{"Failure", testAckDispatcherOnDeviceEventQOSFailure},
	}

	for _, tc := range tests {
		t.Run(tc.description, tc.test)
	}
}

func TestAckDispatcherOnDeviceEvent(t *testing.T) {
	tests := []struct {
		description string
		test        func(*testing.T)
	}{
		{"QOS", testAckDispatcherOnDeviceEventQOS},
	}

	for _, tc := range tests {
		t.Run(tc.description, tc.test)
	}
}
