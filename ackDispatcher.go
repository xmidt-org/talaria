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
	"context"
	"time"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/metrics"
	"github.com/xmidt-org/webpa-common/v2/device"
	"github.com/xmidt-org/webpa-common/v2/logging"
	"github.com/xmidt-org/wrp-go/v3"
)

// ackDispatcher is an internal Dispatcher implementation that processes outbound events
// and determines whether or not an ack to the source device is required.
type ackDispatcher struct {
	errorLog          log.Logger
	timeout           time.Duration
	AckSuccess        metrics.Counter
	AckFailure        metrics.Counter
	AckSuccessLatency metrics.Histogram
	AckFailureLatency metrics.Histogram
}

// NewAckDispatcher an ackDispatcher factory which processes outbound events
// and determines whether or not an ack to the source device is required.
func NewAckDispatcher(om OutboundMeasures, o *Outbounder) (Dispatcher, error) {
	return &ackDispatcher{
		errorLog:          logging.Error(o.logger()),
		timeout:           o.requestTimeout(),
		AckSuccess:        om.AckSuccess,
		AckFailure:        om.AckFailure,
		AckSuccessLatency: om.AckSuccessLatency,
		AckFailureLatency: om.AckFailureLatency,
	}, nil
}

// OnDeviceEvent is the device.Listener function that processes outbound events
// and determines whether or not an ack to the source device is required.
func (d *ackDispatcher) OnDeviceEvent(event *device.Event) {
	var r *device.Request

	if event == nil {
		d.errorLog.Log(logging.MessageKey(), "Error nil event")
		return
	} else if event.Device == nil {
		d.errorLog.Log(logging.MessageKey(), "Error nil device")
		return
	}

	m, ok := event.Message.(*wrp.Message)
	if !ok {
		return
	} else if len(m.PartnerIDs) != 1 {
		// TODO remove this once wrp validation is integrated
		d.errorLog.Log(logging.MessageKey(), "Error invalid number of partnerIDs", "partnerIDLen", len(m.PartnerIDs))
		return
	}

	// Verify ack conditions are met for a given event
	switch event.Type {
	// Atm, we're only supporting acks for MessageReceived events
	case device.MessageReceived:
		// Verify ack conditions are met for a given message
		switch m.Type {
		// Atm, we're only supporting acks for SimpleEventMessageTypes QOS
		case wrp.SimpleEventMessageType:
			// Verify ack conditions are met for a given message type
			// Atm, we're only supporting acks for QOS
			if !m.IsQOSAckPart() {
				return
			}

			// https://xmidt.io/docs/wrp/simple-messages/#qos-details
			r = &device.Request{
				Message: &wrp.Message{
					// When a qos field is specified that requires an ack, the response ack message SHALL be a msg_type=4.
					Type: wrp.SimpleEventMessageType,
					// The `source` SHALL be the component that cannot process the event further.
					Source: "",
					// The `dest` SHALL be the original requesting `source` address.
					Destination: m.Source,
					// The `content_type` and `payload` SHALL be omitted & set to empty, or may set to `application/text` and text to help describe the result.  **DO NOT** process this text beyond for logging/debugging.
					ContentType: "",
					Payload:     []byte{},
					// The `partner_ids` SHALL be the same as the original message.
					PartnerIDs: m.PartnerIDs,
					// The `headers` SHOULD generally be the same as the original message, except where updating their values is correct.
					Headers: m.Headers,
					// The `metadata` map SHALL be populated with the original data or set to empty.
					Metadata: m.Metadata,
					// The `session_id` MAY be added by the cloud.
					SessionID: m.SessionID,
					// The `qos` SHALL be the same as the original message.
					QualityOfService: m.QualityOfService,
					// The `transaction_uuid` SHALL be the same as the original message.
					TransactionUUID: m.TransactionUUID,
					// The `rdr` SHALL be present and represent the outcome of the handling of the message.
					RequestDeliveryResponse: m.RequestDeliveryResponse,
				},
				Format: event.Format,
			}

		default:
			return
		}

	default:
		return
	}

	l := m.QualityOfService.Level()
	p := m.PartnerIDs[0]
	t := m.Type.FriendlyName()
	// Metric labels
	ls := []string{qosLevelLabel, l.String(), partnerIDLabel, p, messageType, t}
	ctx, cancel := context.WithTimeout(context.Background(), d.timeout)
	defer cancel()

	// Observe the latency of sending a qos ack to the source device
	ackFailure := false
	defer func(s time.Time) {
		d.recordAckLatency(s, ackFailure, ls...)
	}(time.Now())

	if _, err := event.Device.Send(r.WithContext(ctx)); err != nil {
		d.errorLog.Log(logging.MessageKey(), "Error dispatching QOS ack", "qosLevel", l, "partnerID", p, "messageType", t, logging.ErrorKey(), err)
		d.AckFailure.With(ls...).Add(1)
		ackFailure = true
		return
	}

	d.AckSuccess.With(ls...).Add(1)
}

// recordAckLatency records the latency for both successful and failed acks
func (d *ackDispatcher) recordAckLatency(s time.Time, f bool, l ...string) {
	switch {
	case f:
		d.AckFailureLatency.With(l...).Observe(time.Since(s).Seconds())

	default:
		d.AckSuccessLatency.With(l...).Observe(time.Since(s).Seconds())
	}
}
