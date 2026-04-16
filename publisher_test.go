// SPDX-FileCopyrightText: 2017 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/xmidt-org/wrp-go/v3"
	wrpv5 "github.com/xmidt-org/wrp-go/v5"
	"github.com/xmidt-org/wrpkafka"
	"go.uber.org/zap"
)

// TestKafkaPublisher_Start tests the Start method with various scenarios
func TestKafkaPublisher_Start(t *testing.T) {
	tests := []struct {
		name          string
		setupMock     func(*mockWrpKafkaPublisher)
		factoryError  error
		expectError   bool
		errorContains string
	}{
		{
			name: "successful start",
			setupMock: func(m *mockWrpKafkaPublisher) {
				m.On("Start").Return(nil).Once()
			},
			expectError: false,
		},
		{
			name: "publisher factory error",
			setupMock: func(m *mockWrpKafkaPublisher) {
				// Factory will return error, so Start won't be called
			},
			factoryError:  errors.New("factory failed"),
			expectError:   true,
			errorContains: "failed to create wrpkafka publisher",
		},
		{
			name: "publisher start error",
			setupMock: func(m *mockWrpKafkaPublisher) {
				m.On("Start").Return(errors.New("kafka connection failed")).Once()
			},
			expectError:   true,
			errorContains: "failed to start wrpkafka publisher",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockPub := new(mockWrpKafkaPublisher)
			tt.setupMock(mockPub)

			config := &KafkaConfig{
				Enabled: true,
				Brokers: []string{"localhost:9092"},
				InitialDynamicConfig: wrpkafka.DynamicConfig{
					TopicMap: []wrpkafka.TopicRoute{
						{Pattern: "*", Topic: "test-topic"},
					},
				},
				MaxBufferedRecords: 1000,
				MaxBufferedBytes:   1000000,
				MaxRetries:         3,
				RequestTimeout:     30 * time.Second,
			}

			kp := &kafkaPublisher{
				config: config,
				logger: zap.NewNop(),
				publisherFactory: func(c *KafkaConfig, promReg prometheus.Registerer) (wrpKafkaPublisher, error) {
					if tt.factoryError != nil {
						return nil, tt.factoryError
					}
					return mockPub, nil
				},
			}

			err := kp.Start()

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
				assert.True(t, kp.started)
				assert.NotNil(t, kp.publisher)
			}

			mockPub.AssertExpectations(t)
		})
	}
}

// TestKafkaPublisher_Start_AlreadyStarted tests starting an already started publisher
func TestKafkaPublisher_Start_AlreadyStarted(t *testing.T) {
	config := &KafkaConfig{
		Enabled: true,
		InitialDynamicConfig: wrpkafka.DynamicConfig{
			TopicMap: []wrpkafka.TopicRoute{
				{Pattern: "*", Topic: "test-topic"},
			},
		},
		Brokers: []string{"localhost:9092"},
	}

	kp := &kafkaPublisher{
		config:  config,
		logger:  zap.NewNop(),
		started: true, // Already started
	}

	err := kp.Start()
	assert.ErrorIs(t, err, ErrKafkaAlreadyStarted)
}

// TestKafkaPublisher_Stop tests the Stop method
func TestKafkaPublisher_Stop(t *testing.T) {
	tests := []struct {
		name      string
		started   bool
		publisher wrpKafkaPublisher
		setupMock func(*mockWrpKafkaPublisher)
	}{
		{
			name:    "stop started publisher",
			started: true,
			setupMock: func(m *mockWrpKafkaPublisher) {
				m.On("Stop", mock.Anything).Return().Once()
			},
		},
		{
			name:      "stop not started publisher",
			started:   false,
			publisher: nil,
			setupMock: func(m *mockWrpKafkaPublisher) {
				// No calls expected
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockPub := new(mockWrpKafkaPublisher)
			tt.setupMock(mockPub)

			config := &KafkaConfig{
				Enabled: true,
				InitialDynamicConfig: wrpkafka.DynamicConfig{
					TopicMap: []wrpkafka.TopicRoute{
						{Pattern: "*", Topic: "test-topic"},
					},
				},
				Brokers: []string{"localhost:9092"},
			}

			kp := &kafkaPublisher{
				config:  config,
				logger:  zap.NewNop(),
				started: tt.started,
			}

			if tt.started {
				kp.publisher = mockPub
			}

			ctx := context.Background()
			err := kp.Stop(ctx)

			assert.NoError(t, err)
			if tt.started {
				assert.False(t, kp.started)
			}

			mockPub.AssertExpectations(t)
		})
	}
}

// TestKafkaPublisher_Publish tests the Publish method
func TestKafkaPublisher_Publish(t *testing.T) {
	tests := []struct {
		name            string
		started         bool
		setNilPublisher bool // explicitly set publisher to nil
		setupMock       func(*mockWrpKafkaPublisher)
		message         *wrp.Message
		expectError     bool
		errorContains   string
	}{
		{
			name:    "successful publish",
			started: true,
			setupMock: func(m *mockWrpKafkaPublisher) {
				m.On("Produce", mock.Anything, mock.AnythingOfType("*wrp.Message")).
					Return(wrpkafka.Accepted, nil).Once()
			},
			message: &wrp.Message{
				Type:        wrp.SimpleEventMessageType,
				Source:      "test",
				Destination: "mac:112233445566",
			},
			expectError: false,
		},
		{
			name:    "publish not started",
			started: false,
			setupMock: func(m *mockWrpKafkaPublisher) {
				// No calls expected
			},
			message: &wrp.Message{
				Type:        wrp.SimpleEventMessageType,
				Source:      "test",
				Destination: "mac:112233445566",
			},
			expectError:   true,
			errorContains: "not started",
		},
		{
			name:    "publish error",
			started: true,
			setupMock: func(m *mockWrpKafkaPublisher) {
				m.On("Produce", mock.Anything, mock.AnythingOfType("*wrp.Message")).
					Return(wrpkafka.Failed, errors.New("kafka error")).Once()
			},
			message: &wrp.Message{
				Type:        wrp.SimpleEventMessageType,
				Source:      "test",
				Destination: "mac:112233445566",
			},
			expectError:   true,
			errorContains: "failed to publish to kafka",
		},
		{
			name:            "nil publisher",
			started:         true,
			setNilPublisher: true,
			setupMock: func(m *mockWrpKafkaPublisher) {
				// No calls expected
			},
			message: &wrp.Message{
				Type:        wrp.SimpleEventMessageType,
				Source:      "test",
				Destination: "mac:112233445566",
			},
			expectError:   true,
			errorContains: "kafka publisher is nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockPub := new(mockWrpKafkaPublisher)
			tt.setupMock(mockPub)

			config := &KafkaConfig{
				Enabled: true,
				InitialDynamicConfig: wrpkafka.DynamicConfig{
					TopicMap: []wrpkafka.TopicRoute{
						{Pattern: "*", Topic: "test-topic"},
					},
				},
				Brokers: []string{"localhost:9092"},
			}

			om, err := NewTestOutboundMeasures()
			require.NoError(t, err)

			kp := &kafkaPublisher{
				config:  config,
				logger:  zap.NewNop(),
				started: tt.started,
				metrics: &om,
			}

			// Set publisher based on test case
			if !tt.setNilPublisher && tt.started {
				kp.publisher = mockPub
			}

			ctx := context.Background()
			err = kp.Publish(ctx, tt.message)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}

			// Only assert expectations if we expected calls to the mock
			if tt.started && !tt.setNilPublisher {
				mockPub.AssertExpectations(t)
			}
		})
	}
}

// TestKafkaPublisher_Publish_MessageConversion tests that WRP v3 to v5 conversion works
func TestKafkaPublisher_Publish_MessageConversion(t *testing.T) {
	mockPub := new(mockWrpKafkaPublisher)

	// Capture the converted message
	var capturedMsg *wrpv5.Message
	mockPub.On("Produce", mock.Anything, mock.AnythingOfType("*wrp.Message")).
		Run(func(args mock.Arguments) {
			capturedMsg = args.Get(1).(*wrpv5.Message)
		}).
		Return(wrpkafka.Accepted, nil).Once()

	config := &KafkaConfig{
		Enabled: true,
		InitialDynamicConfig: wrpkafka.DynamicConfig{
			TopicMap: []wrpkafka.TopicRoute{
				{Pattern: "*", Topic: "test-topic"},
			},
		},
		Brokers: []string{"localhost:9092"},
	}

	om, err := NewTestOutboundMeasures()
	require.NoError(t, err)

	kp := &kafkaPublisher{
		config:    config,
		logger:    zap.NewNop(),
		started:   true,
		publisher: mockPub,
		metrics:   &om,
	}

	status := int64(200)
	originalMsg := &wrp.Message{
		Type:             wrp.SimpleEventMessageType,
		Source:           "test-source",
		Destination:      "test-dest",
		TransactionUUID:  "test-uuid",
		ContentType:      "application/json",
		Status:           &status,
		Headers:          []string{"X-Test: value"},
		Metadata:         map[string]string{"key": "value"},
		Payload:          []byte("test payload"),
		PartnerIDs:       []string{"partner1"},
		SessionID:        "session-123",
		QualityOfService: 50,
	}

	ctx := context.Background()
	err = kp.Publish(ctx, originalMsg)

	require.NoError(t, err)
	require.NotNil(t, capturedMsg)

	// Verify conversion
	assert.Equal(t, wrpv5.MessageType(originalMsg.Type), capturedMsg.Type)
	assert.Equal(t, originalMsg.Source, capturedMsg.Source)
	assert.Equal(t, originalMsg.Destination, capturedMsg.Destination)
	assert.Equal(t, originalMsg.TransactionUUID, capturedMsg.TransactionUUID)
	assert.Equal(t, originalMsg.ContentType, capturedMsg.ContentType)
	assert.Equal(t, originalMsg.Status, capturedMsg.Status)
	assert.Equal(t, originalMsg.Headers, capturedMsg.Headers)
	assert.Equal(t, originalMsg.Metadata, capturedMsg.Metadata)
	assert.Equal(t, originalMsg.Payload, capturedMsg.Payload)
	assert.Equal(t, originalMsg.PartnerIDs, capturedMsg.PartnerIDs)
	assert.Equal(t, originalMsg.SessionID, capturedMsg.SessionID)
	assert.Equal(t, wrpv5.QOSValue(originalMsg.QualityOfService), capturedMsg.QualityOfService)

	mockPub.AssertExpectations(t)
}

// TestKafkaPublisher_IsEnabled tests the IsEnabled method
func TestKafkaPublisher_IsEnabled(t *testing.T) {
	tests := []struct {
		name    string
		enabled bool
		want    bool
	}{
		{
			name:    "enabled",
			enabled: true,
			want:    true,
		},
		{
			name:    "disabled",
			enabled: false,
			want:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &KafkaConfig{
				Enabled: tt.enabled,
				InitialDynamicConfig: wrpkafka.DynamicConfig{
					TopicMap: []wrpkafka.TopicRoute{
						{Pattern: "*", Topic: "test-topic"},
					},
				},
				Brokers: []string{"localhost:9092"},
			}

			kp := &kafkaPublisher{
				config: config,
				logger: zap.NewNop(),
			}

			assert.Equal(t, tt.want, kp.IsEnabled())
		})
	}
}

// TestDefaultPublisherFactory tests the default publisher factory
func TestDefaultPublisherFactory(t *testing.T) {
	tests := []struct {
		name          string
		config        *KafkaConfig
		expectError   bool
		errorContains string
	}{
		{
			name: "basic config",
			config: &KafkaConfig{
				Enabled: true,
				InitialDynamicConfig: wrpkafka.DynamicConfig{
					TopicMap: []wrpkafka.TopicRoute{
						{Pattern: "*", Topic: "test-topic"},
					},
				},
				Brokers:            []string{"localhost:9092"},
				MaxBufferedRecords: 1000,
				MaxBufferedBytes:   1000000,
				MaxRetries:         3,
				RequestTimeout:     30 * time.Second,
			},
			expectError: false,
		},
		{
			name: "with TLS",
			config: &KafkaConfig{
				Enabled: true,
				InitialDynamicConfig: wrpkafka.DynamicConfig{
					TopicMap: []wrpkafka.TopicRoute{
						{Pattern: "*", Topic: "test-topic"},
					},
				},
				Brokers: []string{"localhost:9093"},
				TLS: KafkaTLSConfig{
					Enabled:            true,
					InsecureSkipVerify: true,
				},
			},
			expectError: false,
		},
		{
			name: "with SASL PLAIN",
			config: &KafkaConfig{
				Enabled: true,
				InitialDynamicConfig: wrpkafka.DynamicConfig{
					TopicMap: []wrpkafka.TopicRoute{
						{Pattern: "*", Topic: "test-topic"},
					},
				},
				Brokers: []string{"localhost:9092"},
				SASL: KafkaSASLConfig{
					Mechanism: "PLAIN",
					Username:  "user",
					Password:  "pass",
				},
			},
			expectError: false,
		},
		{
			name: "with SASL SCRAM-SHA-256",
			config: &KafkaConfig{
				Enabled: true,
				InitialDynamicConfig: wrpkafka.DynamicConfig{
					TopicMap: []wrpkafka.TopicRoute{
						{Pattern: "*", Topic: "test-topic"},
					},
				},
				Brokers: []string{"localhost:9092"},
				SASL: KafkaSASLConfig{
					Mechanism: "SCRAM-SHA-256",
					Username:  "user",
					Password:  "pass",
				},
			},
			expectError: false,
		},
		{
			name: "with SASL SCRAM-SHA-512",
			config: &KafkaConfig{
				Enabled: true,
				InitialDynamicConfig: wrpkafka.DynamicConfig{
					TopicMap: []wrpkafka.TopicRoute{
						{Pattern: "*", Topic: "test-topic"},
					},
				},
				Brokers: []string{"localhost:9092"},
				SASL: KafkaSASLConfig{
					Mechanism: "SCRAM-SHA-512",
					Username:  "user",
					Password:  "pass",
				},
			},
			expectError: false,
		},
		{
			name: "unsupported SASL mechanism",
			config: &KafkaConfig{
				Enabled: true,
				InitialDynamicConfig: wrpkafka.DynamicConfig{
					TopicMap: []wrpkafka.TopicRoute{
						{Pattern: "*", Topic: "test-topic"},
					},
				},
				Brokers: []string{"localhost:9092"},
				SASL: KafkaSASLConfig{
					Mechanism: "INVALID",
					Username:  "user",
					Password:  "pass",
				},
			},
			expectError:   true,
			errorContains: "unsupported SASL mechanism",
		},
		{
			name: "with InitialDynamicConfig",
			config: &KafkaConfig{
				Enabled: true,
				Brokers: []string{"localhost:9092"},
				InitialDynamicConfig: wrpkafka.DynamicConfig{
					TopicMap: []wrpkafka.TopicRoute{
						{Pattern: "online", Topic: "lifecycle"},
						{Pattern: "*", Topic: "default"},
					},
				},
			},
			expectError: false,
		},
		{
			name: "with prometheus namespace and subsystem",
			config: &KafkaConfig{
				Enabled: true,
				Brokers: []string{"localhost:9092"},
				InitialDynamicConfig: wrpkafka.DynamicConfig{
					TopicMap: []wrpkafka.TopicRoute{
						{Pattern: "*", Topic: "test-topic"},
					},
				},
				PrometheusNamespace: "xmidt",
				PrometheusSubsystem: "talaria",
			},
			expectError: false,
		},
		{
			name: "with only prometheus namespace",
			config: &KafkaConfig{
				Enabled: true,
				Brokers: []string{"localhost:9092"},
				InitialDynamicConfig: wrpkafka.DynamicConfig{
					TopicMap: []wrpkafka.TopicRoute{
						{Pattern: "*", Topic: "test-topic"},
					},
				},
				PrometheusNamespace: "custom_ns",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			publisher, err := publisherFactory(tt.config, nil)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				assert.Nil(t, publisher)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, publisher)

				// Verify it's a real wrpkafka.Publisher
				wrpPub, ok := publisher.(*wrpkafka.Publisher)
				require.True(t, ok)
				assert.Equal(t, tt.config.Brokers, wrpPub.Brokers)
				assert.Equal(t, tt.config.MaxBufferedRecords, wrpPub.MaxBufferedRecords)
				assert.Equal(t, tt.config.MaxBufferedBytes, wrpPub.MaxBufferedBytes)
				assert.Equal(t, tt.config.MaxRetries, wrpPub.MaxRetries)
				assert.Equal(t, tt.config.RequestTimeout, wrpPub.RequestTimeout)

				// Verify prometheus namespace and subsystem
				assert.Equal(t, tt.config.PrometheusNamespace, wrpPub.PrometheusNamespace,
					"PrometheusNamespace should be passed to wrpkafka.Publisher")
				assert.Equal(t, tt.config.PrometheusSubsystem, wrpPub.PrometheusSubsystem,
					"PrometheusSubsystem should be passed to wrpkafka.Publisher")

				// Verify topic map is empty if not provided
				if len(tt.config.InitialDynamicConfig.TopicMap) == 0 {
					require.Len(t, wrpPub.InitialDynamicConfig.TopicMap, 0)
				}

				// Verify TLS config
				if tt.config.TLS.Enabled {
					assert.NotNil(t, wrpPub.TLS)
					assert.Equal(t, tt.config.TLS.InsecureSkipVerify, wrpPub.TLS.InsecureSkipVerify)
				}

				// Verify SASL config
				if tt.config.SASL.Mechanism != "" && !tt.expectError {
					assert.NotNil(t, wrpPub.SASL)
				}
			}
		})
	}
}

// TestKafkaPublisher_Lifecycle tests full start/publish/stop lifecycle
func TestKafkaPublisher_Lifecycle(t *testing.T) {
	mockPub := new(mockWrpKafkaPublisher)

	// Setup mock expectations for full lifecycle
	mockPub.On("Start").Return(nil).Once()
	mockPub.On("AddPublishEventListener", mock.Anything).Return(nil).Once()
	mockPub.On("Produce", mock.Anything, mock.AnythingOfType("*wrp.Message")).
		Return(wrpkafka.Accepted, nil).Times(3)
	mockPub.On("Stop", mock.Anything).Return().Once()

	config := &KafkaConfig{
		Enabled: true,
		InitialDynamicConfig: wrpkafka.DynamicConfig{
			TopicMap: []wrpkafka.TopicRoute{
				{Pattern: "*", Topic: "test-topic"},
			},
		},
		Brokers: []string{"localhost:9092"},
	}

	om, err := NewTestOutboundMeasures()
	require.NoError(t, err)

	kp := &kafkaPublisher{
		config: config,
		logger: zap.NewNop(),
		publisherFactory: func(c *KafkaConfig, promReg prometheus.Registerer) (wrpKafkaPublisher, error) {
			return mockPub, nil
		},
		metrics: &om,
	}

	// Start
	err = kp.Start()
	require.NoError(t, err)
	assert.True(t, kp.started)

	// Publish multiple messages
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		msg := &wrp.Message{
			Type:        wrp.SimpleEventMessageType,
			Source:      "test",
			Destination: "mac:112233445566",
		}
		err = kp.Publish(ctx, msg)
		require.NoError(t, err)
	}

	// Stop
	err = kp.Stop(ctx)
	require.NoError(t, err)
	assert.False(t, kp.started)

	mockPub.AssertExpectations(t)
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

func TestNewKafkaPublisher(t *testing.T) {
	tests := []struct {
		name                     string
		config                   map[string]interface{}
		expectError              bool
		expectNoop               bool
		expectedPrometheusNS     string
		expectedPrometheusSubsys string
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
		{
			name: "With prometheus config in metricsOptions",
			config: map[string]interface{}{
				"kafka": map[string]interface{}{
					"enabled": true,
					"brokers": []string{"localhost:9092"},
				},
				"metric": map[string]interface{}{
					"metricsOptions": map[string]interface{}{
						"namespace": "xmidt",
						"subsystem": "talaria",
					},
				},
			},
			expectError:              false,
			expectNoop:               false,
			expectedPrometheusNS:     "xmidt",
			expectedPrometheusSubsys: "talaria",
		},
		{
			name: "With only namespace in metricsOptions",
			config: map[string]interface{}{
				"kafka": map[string]interface{}{
					"enabled": true,
					"brokers": []string{"localhost:9092"},
				},
				"metric": map[string]interface{}{
					"metricsOptions": map[string]interface{}{
						"namespace": "custom_namespace",
					},
				},
			},
			expectError:              false,
			expectNoop:               false,
			expectedPrometheusNS:     "custom_namespace",
			expectedPrometheusSubsys: "",
		},
		{
			name: "With only subsystem in metricsOptions",
			config: map[string]interface{}{
				"kafka": map[string]interface{}{
					"enabled": true,
					"brokers": []string{"localhost:9092"},
				},
				"metric": map[string]interface{}{
					"metricsOptions": map[string]interface{}{
						"subsystem": "custom_subsystem",
					},
				},
			},
			expectError:              false,
			expectNoop:               false,
			expectedPrometheusNS:     "",
			expectedPrometheusSubsys: "custom_subsystem",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := viper.New()
			for key, value := range tt.config {
				v.Set(key, value)
			}

			logger := zap.NewNop()
			publisher, err := NewKafkaPublisher(logger, v, nil, nil)

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
				kp, ok := publisher.(*kafkaPublisher)
				assert.True(t, ok, "Expected kafkaPublisher")

				// Verify prometheus namespace and subsystem if specified
				if tt.expectedPrometheusNS != "" || tt.expectedPrometheusSubsys != "" {
					assert.Equal(t, tt.expectedPrometheusNS, kp.config.PrometheusNamespace,
						"PrometheusNamespace should match expected value")
					assert.Equal(t, tt.expectedPrometheusSubsys, kp.config.PrometheusSubsystem,
						"PrometheusSubsystem should match expected value")
				}
			}
		})
	}
}

func TestKafkaPublisher_NotStarted(t *testing.T) {
	v := viper.New()
	v.Set("kafka.enabled", true)
	v.Set("kafka.brokers", []string{"localhost:9092"})
	v.Set("kafka.topic", "test-topic")

	logger := zap.NewNop()
	publisher, err := NewKafkaPublisher(logger, v, nil, nil)
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
