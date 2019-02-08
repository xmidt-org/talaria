package main

import (
	"fmt"
	"time"

	"github.com/Comcast/webpa-common/device"
	"github.com/Comcast/webpa-common/wrp"
)

func onlineDestination(d device.Interface) string {
	return fmt.Sprintf("event:device-status/%s/online", d.ID())
}

func onlinePayload(t time.Time, d device.Interface) []byte {
	return []byte(fmt.Sprintf(`{
		"id": "%s",
		"ts": "%s",
	}`, d.ID(), t.Format(time.RFC3339Nano)))
}

func newOnlineMessage(source string, d device.Interface) *wrp.Message {
	return &wrp.Message{
		Type:        wrp.SimpleEventMessageType,
		Source:      source,
		Destination: onlineDestination(d),
		ContentType: "json",
		PartnerIDs:  d.PartnerIDs(),
		Payload:     onlinePayload(time.Now(), d),
	}
}

func offlineDestination(d device.Interface) string {
	return fmt.Sprintf("event:device-status/%s/offline", d.ID())
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

func newOfflineMessage(source string, closeReason string, d device.Interface) *wrp.Message {
	return &wrp.Message{
		Type:        wrp.SimpleEventMessageType,
		Source:      source,
		Destination: offlineDestination(d),
		ContentType: "json",
		PartnerIDs:  d.PartnerIDs(),
		Payload:     offlinePayload(time.Now(), closeReason, d),
	}
}
