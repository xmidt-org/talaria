package main

import (
	"testing"
)

func TestReceiveOnlineEvent(t *testing.T) {
	runIt(t, testConfig{
		configFile:   "talaria_no_kafka_template.yaml",
		writeToKafka: false,
	})
}

func TestReceiveOnlineEventWithKafka(t *testing.T) {
	runIt(t, testConfig{
		configFile:   "talaria_template.yaml",
		writeToKafka: true,
	})
}
