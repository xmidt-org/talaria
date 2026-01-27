<-- SPDX-FileCopyrightText: 2025 Comcast Cable Communications Management, LLC
SPDX-License-Identifier: Apache-2.0 -->
# Kafka Publisher Integration

This document describes the Kafka publisher integration in Talaria using the [wrpkafka](https://github.com/xmidt-org/wrpkafka) library.

## Overview

The Kafka publisher allows Talaria to publish all WRP (Web Routing Protocol) messages to a single Kafka topic. This enables real-time streaming of device events, messages, and status updates to Kafka for downstream processing.

## Features

- **Single Topic Publishing**: All WRP messages are published to a single configurable Kafka topic
- **QoS Support**: Leverages wrpkafka's quality-of-service guarantees
- **TLS Support**: Optional TLS encryption for secure communication
- **SASL Authentication**: Support for PLAIN, SCRAM-SHA-256, and SCRAM-SHA-512 authentication
- **Configurable Buffering**: Control over buffered records and bytes
- **Retry Logic**: Configurable retry attempts for failed produce requests
- **Dynamic Configuration**: Runtime configuration updates via wrpkafka's dynamic config

## Configuration

The Kafka publisher is configured through the main Talaria configuration file (typically `talaria.yaml`) under the `kafka` section.

### Minimal Configuration

```yaml
kafka:
  enabled: true
  brokers:
    - "localhost:9092"
```

### Configuration Properties

| Property | Type | Required | Default | Description |
|----------|------|----------|---------|-------------|
| `enabled` | boolean | No | `false` | Whether the Kafka publisher is enabled |
| `brokers` | []string | Yes* | - | List of Kafka broker addresses |
| `maxBufferedRecords` | int | No | `10000` | Maximum number of records to buffer |
| `maxBufferedBytes` | int | No | `104857600` (100MB) | Maximum number of bytes to buffer |
| `maxRetries` | int | No | `3` | Maximum number of retries for failed requests |
| `requestTimeout` | duration | No | `30s` | Timeout for produce requests |
| `tls.enabled` | boolean | No | `false` | Whether TLS is enabled |
| `tls.insecureSkipVerify` | boolean | No | `false` | Skip TLS certificate verification (insecure) |
| `tls.certFile` | string | No | - | Path to client certificate file |
| `tls.keyFile` | string | No | - | Path to client key file |
| `tls.caFile` | string | No | - | Path to CA certificate file |
| `sasl.mechanism` | string | No | - | SASL mechanism (PLAIN, SCRAM-SHA-256, SCRAM-SHA-512) |
| `sasl.username` | string | No | - | SASL username |
| `sasl.password` | string | No | - | SASL password |
| `initialDynamicConfig` | object | No | - |  wrpkafka topic configuration |

\* Required when `enabled` is `true`

## Message Flow

1. **Device Events**: When devices connect, disconnect, or send messages, the event dispatcher creates WRP messages
2. **Event Processing**: The event dispatcher processes the event and sends it to configured endpoints (e.g., Caduceus)
3. **Kafka Publishing**: If the Kafka publisher is enabled and the event is successful, the message is asynchronously published to Kafka
4. **WRP Conversion**: Messages are converted from WRP v3 (used internally) to WRP v5 (required by wrpkafka)
5. **Delivery**: wrpkafka handles the actual delivery to Kafka with QoS guarantees

## Message Types Published to Kafka

The following WRP message types are published to Kafka:

- **Connect Events**: Published when a device connects
- **Disconnect Events**: Published when a device disconnects  
- **Device Messages**: All WRP messages received from devices with an `event:` scheme destination

## Architecture

```
┌─────────────┐
│   Device    │
└──────┬──────┘
       │ WebSocket
       ▼
┌─────────────────────┐
│  Event Dispatcher   │
│                     │
│  ┌───────────────┐ │
│  │  HTTP Client  │──────► Caduceus/Endpoints
│  └───────────────┘ │
│                     │
│  ┌───────────────┐ │
│  │ Kafka Publisher│──────► Kafka Topic
│  │   (wrpkafka)  │ │
│  └───────────────┘ │
└─────────────────────┘
```

## Implementation Details

### Publisher Interface

The `Publisher` interface provides the following methods:

```go
type Publisher interface {
    Start() error
    Stop(ctx context.Context) error
    Publish(ctx context.Context, msg *wrp.Message) error
    IsEnabled() bool
}
```

### No-Op Publisher

If Kafka is not configured or disabled, a no-op publisher is used that silently discards all messages without error.

### WRP Version Conversion

Talaria uses WRP v3 internally, but wrpkafka requires WRP v5. The publisher includes automatic conversion logic that:

- Converts message types and fields
- Transforms span format from v3 ([][]string) to v5 ([]Span)
- Preserves all metadata, headers, and payload

### Error Handling

- Configuration errors prevent Talaria from starting
- Publish errors are logged but don't block the main event flow
- Failed publishes don't affect HTTP delivery to Caduceus/endpoints
- wrpkafka handles retries internally based on QoS level

## Monitoring and Observability

The Kafka publisher logs the following events:

- **Info**: Publisher creation, start, and stop events
- **Debug**: Individual message publishes with outcomes
- **Error**: Configuration errors, connection failures, publish failures

Example log entries:

```json
{"level":"info","msg":"Creating Kafka publisher","brokers":["localhost:9092"],"topic":"wrp-events"}
{"level":"info","msg":"Kafka publisher started successfully"}
{"level":"debug","msg":"Published message to Kafka","destination":"event:device-status/...","outcome":"Success"}
{"level":"error","msg":"Failed to publish message to Kafka","destination":"...","error":"..."}
```

## Testing

### Unit Tests

Run the publisher tests:

```bash
go test -v -run TestKafka
go test -v ./publisher_test.go
```

## Troubleshooting

### Publisher doesn't start

- Check that `kafka.enabled` is set to `true`
- Verify `kafka.brokers` are configured
- Review logs for configuration errors

### Messages not appearing in Kafka

- Verify Kafka brokers are reachable
- Check firewall/network connectivity
- Review wrpkafka logs for produce errors
- Ensure the topic exists or auto-creation is enabled

### Authentication failures

- Verify SASL mechanism matches broker configuration
- Check username and password are correct
- Ensure broker has SASL enabled

### TLS connection issues

- Verify certificate files exist and are readable
- Check certificate validity and chain
- Ensure broker has TLS enabled on the correct port

## Dependencies

- [wrpkafka](https://github.com/xmidt-org/wrpkafka) - WRP Kafka publisher
- [franz-go](https://github.com/twmb/franz-go) - High-performance Kafka client (used by wrpkafka)
- [wrp-go/v5](https://github.com/xmidt-org/wrp-go) - WRP protocol implementation

## Performance Considerations

- **Buffering**: Increase `maxBufferedRecords` and `maxBufferedBytes` for higher throughput
- **Compression**: Enable compression in `initialDynamicConfig.compression` to reduce network bandwidth
- **Batching**: franz-go automatically batches messages for efficiency
- **Async Publishing**: Publishing is non-blocking and won't slow down device message processing

## Security Best Practices

1. **Use TLS** in production environments
2. **Enable SASL** authentication
3. **Don't commit** credentials to version control
4. **Use environment variables** or secrets management for sensitive values
5. **Validate certificates** (avoid `insecureSkipVerify` in production)

## Future Enhancements

Potential improvements for future versions:

- Metrics integration (publish success/failure counters)
- Per-event-type topic routing
- Message filtering based on device metadata
- Kafka headers population with WRP metadata
- Support for schema registry integration
