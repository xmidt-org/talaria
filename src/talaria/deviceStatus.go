package main

import (
	"fmt"
	"time"

	"github.com/Comcast/webpa-common/device"
	"github.com/Comcast/webpa-common/wrp"
)

func statusEventType(id device.ID, subtype string) string {
	return fmt.Sprintf("device-status/%s/%s", id, subtype)
}

func onlinePayload(t time.Time, d device.Interface) []byte {
	return []byte(fmt.Sprintf(`{
		"id": "%s",
		"ts": "%s",
	}`, d.ID(), t.Format(time.RFC3339Nano)))
}

func newOnlineMessage(source string, d device.Interface) (string, *wrp.Message) {
	eventType := statusEventType(d.ID(), "online")

	return eventType, &wrp.Message{
		Type:        wrp.SimpleEventMessageType,
		Source:      source,
		Destination: "event:" + eventType,
		ContentType: "json",
		PartnerIDs:  d.PartnerIDs(),
		Payload:     onlinePayload(time.Now(), d),
	}
}

func offlinePayload(t time.Time, closeReason string, d device.Interface) []byte {
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
		"reason-for-closure": "%s",
	}`, d.ID(),
		t.Format(time.RFC3339Nano),
		statistics.BytesSent(),
		statistics.MessagesSent(),
		statistics.BytesReceived(),
		statistics.MessagesReceived(),
		statistics.ConnectedAt().Format(time.RFC3339Nano),
		statistics.UpTime(),
		closeReason,
	))
}

func newOfflineMessage(source string, closeReason string, d device.Interface) (string, *wrp.Message) {
	eventType := statusEventType(d.ID(), "offline")

	return eventType, &wrp.Message{
		Type:        wrp.SimpleEventMessageType,
		Source:      source,
		Destination: "event:" + eventType,
		ContentType: "json",
		PartnerIDs:  d.PartnerIDs(),
		Payload:     offlinePayload(time.Now(), closeReason, d),
	}
}
