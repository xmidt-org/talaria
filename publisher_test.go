// SPDX-FileCopyrightText: 2017 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xmidt-org/wrp-go/v3"
	"go.uber.org/zap"
)

func TestNewKafkaPublisher(t *testing.T) {
	tests := []struct {
		name        string
		config      map[string]interface{}
		expectError bool
		expectNoop  bool
	}{
		{
			name:        "No config returns noop",
			config:      map[string]interface{}{},
			expectError: false,
			expectNoop:  true,
		},
		{
			name: "Disabled config returns noop",
			config: map[string]interface{}{
				"kafka": map[string]interface{}{
					"enabled": false,
					"brokers": []string{"localhost:9092"},
					"topic":   "test-topic",
				},
			},
			expectError: false,
			expectNoop:  true,
		},
		{
			name: "Missing brokers returns error",
			config: map[string]interface{}{
				"kafka": map[string]interface{}{
					"enabled": true,
					"topic":   "test-topic",
				},
			},
			expectError: true,
		},
		{
			name: "Valid config creates publisher",
			config: map[string]interface{}{
				"kafka": map[string]interface{}{
					"enabled": true,
					"brokers": []string{"localhost:9092"},
					"topic":   "test-topic",
				},
			},
			expectError: false,
			expectNoop:  false,
		},
		{
			name: "Valid config with all options",
			config: map[string]interface{}{
				"kafka": map[string]interface{}{
					"enabled":            true,
					"brokers":            []string{"localhost:9092", "localhost:9093"},
					"topic":              "test-topic",
					"maxBufferedRecords": 5000,
					"maxBufferedBytes":   50000000,
					"maxRetries":         5,
					"requestTimeout":     "45s",
					"tls": map[string]interface{}{
						"enabled":            true,
						"insecureSkipVerify": false,
					},
					"sasl": map[string]interface{}{
						"mechanism": "PLAIN",
						"username":  "test-user",
						"password":  "test-pass",
					},
				},
			},
			expectError: false,
			expectNoop:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := viper.New()
			for key, value := range tt.config {
				v.Set(key, value)
			}

			logger := zap.NewNop()
			publisher, err := NewKafkaPublisher(logger, v)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, publisher)

			if tt.expectNoop {
				assert.False(t, publisher.IsEnabled())
				_, ok := publisher.(*noopPublisher)
				assert.True(t, ok, "Expected noopPublisher")
			} else {
				assert.True(t, publisher.IsEnabled())
				_, ok := publisher.(*kafkaPublisher)
				assert.True(t, ok, "Expected kafkaPublisher")
			}
		})
	}
}

func TestNoopPublisher(t *testing.T) {
	noop := &noopPublisher{}

	assert.False(t, noop.IsEnabled())
	assert.NoError(t, noop.Start())
	assert.NoError(t, noop.Stop(context.Background()))

	msg := &wrp.Message{
		Type:        wrp.SimpleEventMessageType,
		Source:      "test",
		Destination: "mac:112233445566",
	}
	assert.NoError(t, noop.Publish(context.Background(), msg))
}

func TestKafkaPublisher_NotStarted(t *testing.T) {
	v := viper.New()
	v.Set("kafka.enabled", true)
	v.Set("kafka.brokers", []string{"localhost:9092"})
	v.Set("kafka.topic", "test-topic")

	logger := zap.NewNop()
	publisher, err := NewKafkaPublisher(logger, v)
	require.NoError(t, err)

	msg := &wrp.Message{
		Type:        wrp.SimpleEventMessageType,
		Source:      "test",
		Destination: "mac:112233445566",
	}

	// Should fail when not started
	err = publisher.Publish(context.Background(), msg)
	assert.ErrorIs(t, err, ErrKafkaNotStarted)
}

func TestConvertV3ToV5(t *testing.T) {
	status := int64(200)
	v3msg := &wrp.Message{
		Type:             wrp.SimpleEventMessageType,
		Source:           "test-source",
		Destination:      "mac:112233445566",
		TransactionUUID:  "test-uuid",
		ContentType:      "application/json",
		Status:           &status,
		Headers:          []string{"X-Test: value"},
		Metadata:         map[string]string{"key": "value"},
		Payload:          []byte("test payload"),
		PartnerIDs:       []string{"partner1", "partner2"},
		SessionID:        "session-123",
		QualityOfService: 50,
	}

	v5msg := convertV3ToV5(v3msg)

	assert.NotNil(t, v5msg)
	assert.Equal(t, v3msg.Source, v5msg.Source)
	assert.Equal(t, v3msg.Destination, v5msg.Destination)
	assert.Equal(t, v3msg.TransactionUUID, v5msg.TransactionUUID)
	assert.Equal(t, v3msg.ContentType, v5msg.ContentType)
	assert.Equal(t, v3msg.Status, v5msg.Status)
	assert.Equal(t, v3msg.Headers, v5msg.Headers)
	assert.Equal(t, v3msg.Metadata, v5msg.Metadata)
	assert.Equal(t, v3msg.Payload, v5msg.Payload)
	assert.Equal(t, v3msg.PartnerIDs, v5msg.PartnerIDs)
	assert.Equal(t, v3msg.SessionID, v5msg.SessionID)
}

func TestConvertV3ToV5_Nil(t *testing.T) {
	v5msg := convertV3ToV5(nil)
	assert.Nil(t, v5msg)
}

func TestKafkaConfig_Defaults(t *testing.T) {
	v := viper.New()
	v.Set("kafka.enabled", true)
	v.Set("kafka.brokers", []string{"localhost:9092"})
	v.Set("kafka.topic", "test-topic")

	logger := zap.NewNop()
	publisher, err := NewKafkaPublisher(logger, v)
	require.NoError(t, err)

	kp, ok := publisher.(*kafkaPublisher)
	require.True(t, ok)

	// Check defaults
	assert.Equal(t, 10000, kp.config.MaxBufferedRecords)
	assert.Equal(t, 100*1024*1024, kp.config.MaxBufferedBytes)
	assert.Equal(t, 3, kp.config.MaxRetries)
	assert.Equal(t, 30*time.Second, kp.config.RequestTimeout)
}
