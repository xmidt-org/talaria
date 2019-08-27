package main

import (
	"fmt"
	"time"

	"github.com/xmidt-org/webpa-common/convey"
	"github.com/xmidt-org/webpa-common/device"
	"github.com/xmidt-org/wrp-go/wrp"
	"github.com/xmidt-org/wrp-go/wrp/wrpmeta"
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
		wrpmeta.Field{From: "protocol", To: "/protocol"}).
		Set("/trust", d.Trust()).
		Build()

	if allFieldsPresent {
		metadata["/compliance"] = d.ConveyCompliance().String()
	} else {
		metadata["/compliance"] = convey.MissingFields.String()
	}

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

	return eventType, &wrp.Message{
		Type:        wrp.SimpleEventMessageType,
		Source:      source,
		Destination: "event:" + eventType,
		ContentType: "json",
		PartnerIDs:  d.PartnerIDs(),
		Metadata:    statusMetadata(d),
		Payload:     onlinePayload(time.Now(), d),
	}
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

	return eventType, &wrp.Message{
		Type:        wrp.SimpleEventMessageType,
		Source:      source,
		Destination: "event:" + eventType,
		ContentType: "json",
		PartnerIDs:  d.PartnerIDs(),
		Metadata:    statusMetadata(d),
		Payload:     offlinePayload(time.Now(), d),
	}
}
