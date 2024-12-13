// SPDX-FileCopyrightText: 2017 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"fmt"
	"strconv"
	"time"
	"strings"

	"github.com/xmidt-org/webpa-common/v2/convey"
	"github.com/xmidt-org/webpa-common/v2/device"
	"github.com/xmidt-org/wrp-go/v3"
	"github.com/xmidt-org/wrp-go/v3/wrpmeta"
)

func statusMetadata(d device.Interface) map[string]string {
	metadata, allFieldsPresent := wrpmeta.NewBuilder().Apply(
		d.Convey(),
		wrpmeta.Field{From: "boot-time", To: "/boot-time"},
		wrpmeta.Field{From: "hw-model", To: "/hw-model"},
		wrpmeta.Field{From: "hw-manufacturer", To: "/hw-manufacturer"},
		wrpmeta.Field{From: "hw-serial-number", To: "/hw-serial-number"},
		wrpmeta.Field{From: "hw-last-reboot-reason", To: "/hw-last-reboot-reason"},
		wrpmeta.Field{From: "fw-name", To: "/fw-name"},
		wrpmeta.Field{From: "last-reconnect-reason", To: "/last-reconnect-reason"},
		wrpmeta.Field{From: "webpa-last-reconnect-reason", To: "/last-reconnect-reason"},
		wrpmeta.Field{From: "webpa-protocol", To: "/protocol"},
		wrpmeta.Field{From: "webpa-interface-used", To: "/interface-used"},
		wrpmeta.Field{From: "boot-time-retry-wait", To: "/boot-time-retry-wait"}).
		Set("/trust", strconv.Itoa(d.Metadata().TrustClaim())).
		Build()

	if allFieldsPresent {
		metadata["/compliance"] = d.ConveyCompliance().String()
	} else {
		metadata["/compliance"] = convey.MissingFields.String()
	}

	metadata["hw-mac"] = strings.TrimPrefix(string(d.ID()), "mac:")
	return metadata
}

func statusEventType(id device.ID, subtype string) string {
	return fmt.Sprintf("device-status/%s/%s", id, subtype)
}

func onlinePayload(t time.Time, d device.Interface) []byte {
	return []byte(fmt.Sprintf(`{
		"id": "%s",
		"ts": "%s"
	}`, d.ID(), t.Format(time.RFC3339Nano)))
}

func newOnlineMessage(source string, d device.Interface) (string, *wrp.Message) {
	eventType := statusEventType(d.ID(), "online")
	t := time.Now()
	m := &wrp.Message{
		Type:        wrp.SimpleEventMessageType,
		Source:      source,
		Destination: "event:" + eventType,
		ContentType: wrp.MimeTypeJson,
		PartnerIDs:  []string{d.Metadata().PartnerIDClaim()},
		SessionID:   d.Metadata().SessionID(),
		Metadata:    statusMetadata(d),
		Payload:     onlinePayload(t, d),
	}
	updateTimestampMetadata(m, t)

	return m.FindEventStringSubMatch(), m
}

func offlinePayload(t time.Time, d device.Interface) []byte {
	statistics := d.Statistics()

	return []byte(fmt.Sprintf(`{
		"id": "%s",
		"ts": "%s",
		"bytes-sent": %d,
		"messages-sent": %d,
		"bytes-received": %d,
		"messages-received": %d,
		"connected-at": "%s",
		"up-time": "%s",
		"reason-for-closure": "%s"
	}`, d.ID(),
		t.Format(time.RFC3339Nano),
		statistics.BytesSent(),
		statistics.MessagesSent(),
		statistics.BytesReceived(),
		statistics.MessagesReceived(),
		statistics.ConnectedAt().Format(time.RFC3339Nano),
		statistics.UpTime(),
		d.CloseReason(),
	))
}

func newOfflineMessage(source string, d device.Interface) (string, *wrp.Message) {
	eventType := statusEventType(d.ID(), "offline")
	t := time.Now()
	m := &wrp.Message{
		Type:        wrp.SimpleEventMessageType,
		Source:      source,
		Destination: "event:" + eventType,
		ContentType: wrp.MimeTypeJson,
		PartnerIDs:  []string{d.Metadata().PartnerIDClaim()},
		SessionID:   d.Metadata().SessionID(),
		Metadata:    statusMetadata(d),
		Payload:     offlinePayload(t, d),
	}
	updateTimestampMetadata(m, t)

	return m.FindEventStringSubMatch(), m
}

func updateTimestampMetadata(m *wrp.Message, t time.Time) {
	m.Metadata[device.WRPTimestampMetadataKey] = t.Format(time.RFC3339Nano)
}
