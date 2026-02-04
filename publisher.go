// SPDX-FileCopyrightText: 2017 Comcast Cable Communications Management, LLC
// SPDX-License-Identifier: Apache-2.0
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/spf13/viper"
	"github.com/twmb/franz-go/pkg/sasl"
	"github.com/twmb/franz-go/pkg/sasl/plain"
	"github.com/twmb/franz-go/pkg/sasl/scram"
	"github.com/xmidt-org/wrp-go/v3"
	wrpv5 "github.com/xmidt-org/wrp-go/v5"
	"github.com/xmidt-org/wrpkafka"
	"go.uber.org/zap"
)

const (
	// KafkaConfigKey is the key in the Viper config for Kafka publisher configuration
	KafkaConfigKey = "kafka"
)

var (
	// ErrKafkaNotConfigured is returned when Kafka is not configured
	ErrKafkaNotConfigured = errors.New("kafka publisher not configured")
	// ErrKafkaAlreadyStarted is returned when attempting to start an already started publisher
	ErrKafkaAlreadyStarted = errors.New("kafka publisher already started")
	// ErrKafkaNotStarted is returned when attempting to publish before starting
	ErrKafkaNotStarted = errors.New("kafka publisher not started")
)

// KafkaTLSConfig configures TLS for Kafka connections
type KafkaTLSConfig struct {
	// Enabled determines whether TLS is enabled
	Enabled bool `json:"enabled" yaml:"enabled"`
	// InsecureSkipVerify controls whether a client verifies the server's certificate chain
	InsecureSkipVerify bool `json:"insecureSkipVerify" yaml:"insecureSkipVerify"`
	// CertFile is the path to the client certificate file
	CertFile string `json:"certFile" yaml:"certFile"`
	// KeyFile is the path to the client key file
	KeyFile string `json:"keyFile" yaml:"keyFile"`
	// CAFile is the path to the CA certificate file
	CAFile string `json:"caFile" yaml:"caFile"`
}

// KafkaSASLConfig configures SASL authentication for Kafka
type KafkaSASLConfig struct {
	// Mechanism is the SASL mechanism to use (PLAIN, SCRAM-SHA-256, SCRAM-SHA-512)
	Mechanism string `json:"mechanism" yaml:"mechanism"`
	// Username is the SASL username
	Username string `json:"username" yaml:"username"`
	// Password is the SASL password
	Password string `json:"password" yaml:"password"`
}

// KafkaConfig holds the configuration for the Kafka publisher
type KafkaConfig struct {
	// Enabled determines whether the Kafka publisher is enabled
	Enabled bool `json:"enabled" yaml:"enabled"`
	// Enabled determines whether the Kafka publisher is enabled
	AllowAutoTopicCreation bool `json:"allow_auto_topic_creation"`
	// Brokers is the list of Kafka broker addresses
	Brokers []string `json:"brokers" yaml:"brokers"`
	// TLS configures TLS for Kafka connections
	TLS KafkaTLSConfig `json:"tls" yaml:"tls"`
	// SASL configures SASL authentication
	SASL KafkaSASLConfig `json:"sasl" yaml:"sasl"`
	// MaxBufferedRecords is the maximum number of records to buffer
	MaxBufferedRecords int `json:"maxBufferedRecords" yaml:"maxBufferedRecords"`
	// MaxBufferedBytes is the maximum number of bytes to buffer
	MaxBufferedBytes int `json:"maxBufferedBytes" yaml:"maxBufferedBytes"`
	// MaxRetries is the maximum number of retries for failed produce requests
	MaxRetries int `json:"maxRetries" yaml:"maxRetries"`
	// RequestTimeout is the timeout for produce requests
	RequestTimeout time.Duration `json:"requestTimeout" yaml:"requestTimeout"`
	// InitialDynamicConfig is the initial dynamic configuration for wrpkafka
	InitialDynamicConfig wrpkafka.DynamicConfig `json:"initialDynamicConfig" yaml:"initialDynamicConfig"`
}

// Publisher is an interface for publishing WRP messages to Kafka
type Publisher interface {
	// Start initializes and starts the Kafka publisher
	Start() error
	// Stop gracefully shuts down the Kafka publisher
	Stop(ctx context.Context) error
	// Publish sends a WRP message to Kafka
	Publish(ctx context.Context, msg *wrp.Message) error
	// IsEnabled returns true if the publisher is enabled
	IsEnabled() bool
}

// wrpKafkaPublisher is an interface that wraps wrpkafka.Publisher methods we need
// This allows us to mock the wrpkafka.Publisher for testing
type wrpKafkaPublisher interface {
	Start() error
	Stop(ctx context.Context)
	Produce(ctx context.Context, msg *wrpv5.Message) (wrpkafka.Outcome, error)
	AddPublishEventListener(fn func(*wrpkafka.PublishEvent)) func()
	BufferedRecords() (currentRecords, maxRecords int, currentBytes, maxBytes int64)
}

// kafkaPublisher implements the Publisher interface using wrpkafka
type kafkaPublisher struct {
	config           *KafkaConfig
	logger           *zap.Logger
	publisher        wrpKafkaPublisher
	started          bool
	publisherFactory func(*KafkaConfig) (wrpKafkaPublisher, error) // for testing
	metrics          *OutboundMeasures                             // for Prometheus metrics
}

// NewKafkaPublisher creates a new Kafka publisher from Viper configuration
func NewKafkaPublisher(logger *zap.Logger, v *viper.Viper, om *OutboundMeasures) (Publisher, error) {
	if v == nil {
		return nil, errors.New("viper config is required")
	}

	// Get Kafka config from viper
	kafkaV := v.Sub(KafkaConfigKey)
	if kafkaV == nil {
		logger.Info("Kafka publisher not configured, using no-op publisher")
		return &noopPublisher{}, nil
	}

	var config KafkaConfig
	if err := kafkaV.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal kafka config: %w", err)
	}

	if !config.Enabled {
		logger.Info("Kafka publisher disabled in configuration")
		return &noopPublisher{}, nil
	}

	// Validate required configuration
	if len(config.Brokers) == 0 {
		return nil, errors.New("kafka.brokers is required")
	}

	logger.Info("Creating Kafka publisher",
		zap.Strings("brokers", config.Brokers),
		zap.Bool("allowAutoTopicCreation", config.AllowAutoTopicCreation),
		zap.Int("maxBufferedRecords", config.MaxBufferedRecords),
		zap.Int("maxBufferedBytes", config.MaxBufferedBytes),
		zap.Int("maxRetries", config.MaxRetries),
		zap.Duration("requestTimeout", config.RequestTimeout),
		zap.Any("dynamicConfig", config.InitialDynamicConfig),
	)

	return &kafkaPublisher{
		config:           &config,
		logger:           logger,
		publisherFactory: publisherFactory,
		metrics:          om,
	}, nil
}

// publisherFactory creates a real wrpkafka.Publisher
func publisherFactory(config *KafkaConfig) (wrpKafkaPublisher, error) {

	// Create wrpkafka publisher
	publisher := &wrpkafka.Publisher{
		Brokers:                config.Brokers,
		MaxBufferedRecords:     config.MaxBufferedRecords,
		MaxBufferedBytes:       config.MaxBufferedBytes,
		MaxRetries:             config.MaxRetries,
		RequestTimeout:         config.RequestTimeout,
		InitialDynamicConfig:   config.InitialDynamicConfig,
		AllowAutoTopicCreation: config.AllowAutoTopicCreation,
	}

	// Configure TLS if enabled
	if config.TLS.Enabled {
		publisher.TLS = &tls.Config{
			//nolint:gosec
			InsecureSkipVerify: config.TLS.InsecureSkipVerify,
		}
		caCertPool := x509.NewCertPool()
		if !config.TLS.InsecureSkipVerify {
			caCertPEM, err := os.ReadFile(config.TLS.CAFile)
			if err != nil {
				return nil, fmt.Errorf("failed to read CA certificate file: %w", err)
			}
			if !caCertPool.AppendCertsFromPEM(caCertPEM) {
				return nil, fmt.Errorf("failed to append CA certificate to pool")
			}
			publisher.TLS.RootCAs = caCertPool
		}
		if config.TLS.CertFile != "" && config.TLS.KeyFile != "" {
			cert, err := tls.LoadX509KeyPair(config.TLS.CertFile, config.TLS.KeyFile)
			if err != nil {
				return nil, fmt.Errorf("failed to load client certificate/key: %w", err)
			}
			publisher.TLS.Certificates = []tls.Certificate{cert}
		}
	}

	// Configure SASL if mechanism is set
	if config.SASL.Mechanism != "" {
		var mechanism sasl.Mechanism

		switch config.SASL.Mechanism {
		case "PLAIN":
			mechanism = plain.Auth{
				User: config.SASL.Username,
				Pass: config.SASL.Password,
			}.AsMechanism()
		case "SCRAM-SHA-256":
			mechanism = scram.Auth{
				User: config.SASL.Username,
				Pass: config.SASL.Password,
			}.AsSha256Mechanism()
		case "SCRAM-SHA-512":
			mechanism = scram.Auth{
				User: config.SASL.Username,
				Pass: config.SASL.Password,
			}.AsSha512Mechanism()
		default:
			return nil, fmt.Errorf("unsupported SASL mechanism: %s", config.SASL.Mechanism)
		}

		publisher.SASL = mechanism
	}

	return publisher, nil
}

// Start initializes and starts the Kafka publisher
func (k *kafkaPublisher) Start() error {
	if k.started {
		return ErrKafkaAlreadyStarted
	}

	// Create the wrpkafka publisher using the factory
	publisher, err := k.publisherFactory(k.config)
	if err != nil {
		return fmt.Errorf("failed to create wrpkafka publisher: %w", err)
	}

	// Configure Prometheus metrics via event listeners (if metrics provided)
	if k.metrics != nil {
		// Add event listener for all publish events (success and failure)
		publisher.AddPublishEventListener(func(event *wrpkafka.PublishEvent) {
			labels := prometheus.Labels{
				eventTypeLabel:          event.EventType,
				topicLabel:              event.Topic,
				topicShardStrategyLabel: event.TopicShardStrategy,
			}

			// Record latency for all events
			k.metrics.KafkaPublishLatency.With(labels).Observe(event.Duration.Seconds())

			if event.Error != nil {
				// Record error
				errorLabels := prometheus.Labels{
					eventTypeLabel:          event.EventType,
					topicLabel:              event.Topic,
					topicShardStrategyLabel: event.TopicShardStrategy,
					errorTypeLabel:          event.ErrorType,
				}
				k.metrics.KafkaPublishErrors.With(errorLabels).Inc()
			} else {
				// Record success
				k.metrics.KafkaPublished.With(labels).Inc()
			}
		})

		// Set up buffer utilization gauge (only if MaxBufferedRecords is configured)
		// find a better way - set up GaugeFunc with publisher instance and touchstone
		if k.config.MaxBufferedRecords > 0 {
			k.metrics.KafkaBufferUtilization = prometheus.NewGaugeFunc(
				prometheus.GaugeOpts{
					Name: KafkaBufferUtilizationGauge,
					Help: "Kafka buffer utilization (0.0-1.0)",
				},
				func() float64 {
					current, max, _, _ := publisher.BufferedRecords()
					if max == 0 {
						return 0.0
					}
					return float64(current) / float64(max)
				},
			)
			// Note: The gauge will be automatically scraped by Prometheus
			// but we need to register it with the Prometheus registry
			prometheus.MustRegister(k.metrics.KafkaBufferUtilization)
		}
	}

	// Start the publisher
	if err := publisher.Start(); err != nil {
		return fmt.Errorf("failed to start wrpkafka publisher: %w", err)
	}

	k.publisher = publisher
	k.started = true

	k.logger.Info("Kafka publisher started successfully")
	return nil
}

// Stop gracefully shuts down the Kafka publisher
func (k *kafkaPublisher) Stop(ctx context.Context) error {
	if !k.started || k.publisher == nil {
		return nil
	}

	k.logger.Info("Stopping Kafka publisher")
	k.publisher.Stop(ctx)
	k.started = false
	k.logger.Info("Kafka publisher stopped")
	return nil
}

// Publish sends a WRP message to Kafka
func (k *kafkaPublisher) Publish(ctx context.Context, msg *wrp.Message) error {
	if !k.started {
		k.logger.Debug("Kafka is not started, cannot publish message")
		return ErrKafkaNotStarted
	}

	if k.publisher == nil {
		return errors.New("kafka publisher is nil")
	}

	// Convert wrp v3 message to v5 for wrpkafka
	v5msg := convertV3ToV5(msg)

	// Use wrpkafka to publish the message
	outcome, err := k.publisher.Produce(ctx, v5msg)
	if err != nil {
		k.logger.Error("Failed to publish message to Kafka",
			zap.Error(err),
			zap.String("destination", msg.Destination),
		)
		return fmt.Errorf("failed to publish to kafka: %w", err)
	}

	k.logger.Debug("Published message to Kafka",
		zap.String("destination", msg.Destination),
		zap.String("outcome", outcome.String()),
	)

	return nil
}

// convertV3ToV5 converts a wrp v3 Message to a wrp v5 Message
func convertV3ToV5(v3msg *wrp.Message) *wrpv5.Message {
	if v3msg == nil {
		return nil
	}

	v5msg := &wrpv5.Message{
		Type:                    wrpv5.MessageType(v3msg.Type),
		Source:                  v3msg.Source,
		Destination:             v3msg.Destination,
		TransactionUUID:         v3msg.TransactionUUID,
		ContentType:             v3msg.ContentType,
		Accept:                  v3msg.Accept,
		Status:                  v3msg.Status,
		RequestDeliveryResponse: v3msg.RequestDeliveryResponse,
		Headers:                 v3msg.Headers,
		Metadata:                v3msg.Metadata,
		Path:                    v3msg.Path,
		Payload:                 v3msg.Payload,
		ServiceName:             v3msg.ServiceName,
		URL:                     v3msg.URL,
		PartnerIDs:              v3msg.PartnerIDs,
		SessionID:               v3msg.SessionID,
		QualityOfService:        wrpv5.QOSValue(v3msg.QualityOfService),
	}

	return v5msg
}

// IsEnabled returns true if the publisher is enabled
func (k *kafkaPublisher) IsEnabled() bool {
	return k.config.Enabled
}

// noopPublisher is a no-op implementation of Publisher
type noopPublisher struct{}

func (n *noopPublisher) Start() error                                        { return nil }
func (n *noopPublisher) Stop(ctx context.Context) error                      { return nil }
func (n *noopPublisher) Publish(ctx context.Context, msg *wrp.Message) error { return nil }
func (n *noopPublisher) IsEnabled() bool                                     { return false }
