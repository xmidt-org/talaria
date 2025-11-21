<-- SPDX-FileCopyrightText: 2025 Comcast Cable Communications Management, LLC
SPDX-License-Identifier: Apache-2.0 -->
# Kafka Publisher Implementation Summary

## Overview

This implementation adds Kafka publishing capabilities to Talaria using the [wrpkafka](https://github.com/xmidt-org/wrpkafka) library. All WRP messages processed by Talaria can now be published to a single Kafka topic in addition to being sent to Caduceus/HTTP endpoints.

## Files Modified/Created

### New Files
1. **`publisher.go`** - Main Kafka publisher implementation
   - `Publisher` interface for publishing WRP messages
   - `kafkaPublisher` - Production implementation using wrpkafka
   - `noopPublisher` - No-op implementation when Kafka is disabled
   - `NewKafkaPublisher()` - Factory function to create publisher from Viper config
   - WRP v3 to v5 conversion logic

2. **`publisher_test.go`** - Unit tests for the publisher
   - Configuration validation tests
   - No-op publisher tests
   - WRP message conversion tests
   - Default configuration tests

3. **`docs/kafka-publisher.md`** - Comprehensive documentation
   - Feature overview
   - Configuration guide
   - Architecture diagrams
   - Troubleshooting guide
   - Security best practices

4. **`kafka-config-example.yaml`** - Example configuration file
   - Complete configuration example with all available options
   - Inline documentation for each property

### Modified Files

1. **`eventDispatcher.go`**
   - Added `kafkaPublisher` field to `eventDispatcher` struct
   - Updated `NewEventDispatcher()` to accept `kafkaPublisher` parameter
   - Integrated Kafka publishing for Connect, Disconnect, and MessageReceived events
   - Kafka publishing happens after successful HTTP delivery

2. **`outbounder.go`**
   - Added `StartWithKafka()` method to accept optional Kafka publisher
   - Maintained backward compatibility with existing `Start()` method

3. **`main.go`**
   - Updated `newDeviceManager()` to create and start Kafka publisher
   - Publisher is created from Viper configuration
   - Publisher is started before device manager initialization
   - Uses `StartWithKafka()` to inject publisher into event dispatcher

4. **`eventDispatcher_test.go`**
   - Updated all calls to `NewEventDispatcher()` to include `nil` for kafka publisher parameter

5. **`go.mod`** and **`go.sum`**
   - Added `github.com/xmidt-org/wrpkafka` dependency
   - Added `github.com/xmidt-org/wrp-go/v5` dependency  
   - Added `github.com/twmb/franz-go` transitive dependencies
   - Updated Go version to 1.25.2

## Configuration Properties

All configuration is under the `kafka` section in `talaria.yaml`:

### Required (when enabled=true)
- `enabled` - Enable/disable Kafka publisher
- `topic` - Kafka topic name for all messages
- `brokers` - List of Kafka broker addresses

### Optional
- `maxBufferedRecords` - Max records to buffer (default: 10000)
- `maxBufferedBytes` - Max bytes to buffer (default: 100MB)
- `maxRetries` - Max retry attempts (default: 3)
- `requestTimeout` - Produce request timeout (default: 30s)
- `tls.enabled` - Enable TLS encryption
- `tls.insecureSkipVerify` - Skip cert verification (dev only)
- `tls.certFile` - Client certificate path
- `tls.keyFile` - Client key path
- `tls.caFile` - CA certificate path
- `sasl.mechanism` - SASL mechanism (PLAIN, SCRAM-SHA-256, SCRAM-SHA-512)
- `sasl.username` - SASL username
- `sasl.password` - SASL password
- `initialDynamicConfig` - Advanced wrpkafka configuration

## Implementation Details

### Publisher Interface

```go
type Publisher interface {
    Start() error
    Stop(ctx context.Context) error
    Publish(ctx context.Context, msg *wrp.Message) error
    IsEnabled() bool
}
```

### Message Flow

1. Device connects/disconnects or sends a message
2. Event dispatcher processes the event
3. HTTP delivery to Caduceus/endpoints occurs first
4. If successful AND Kafka is enabled, message is published to Kafka
5. Kafka publish errors are logged but don't affect HTTP flow

### WRP Version Conversion

- Talaria internally uses WRP v3
- wrpkafka requires WRP v5
- Automatic conversion happens in `convertV3ToV5()`
- All common fields are preserved

### Error Handling

- Configuration errors prevent Talaria startup
- Publish errors are logged but non-blocking
- Failed Kafka publishes don't affect HTTP delivery
- wrpkafka handles retries based on QoS level

## Quality of Service (QoS)

wrpkafka provides three QoS levels based on the WRP `QualityOfService` field:

- **0-24** (Low): Fire-and-forget, may be dropped if buffer is full
- **25-74** (Medium): Async with retries, returns when queued
- **75-99** (High): Synchronous with broker acknowledgment

## Testing

### Unit Tests
All publisher unit tests pass:
```bash
cd /Users/mpicci200/comcast/talaria
go test -v -run TestKafka ./publisher_test.go ./publisher.go
```

### Build Verification
Project builds successfully:
```bash
cd /Users/mpicci200/comcast/talaria
go build -v .
```

## Usage Example

Add to `talaria.yaml`:

```yaml
kafka:
  enabled: true
  topic: "wrp-events"
  brokers:
    - "localhost:9092"
  maxBufferedRecords: 10000
  maxBufferedBytes: 104857600  # 100MB
  maxRetries: 3
  requestTimeout: "30s"
```

## Security Considerations

1. **TLS Encryption**: Enable `tls.enabled` in production
2. **SASL Authentication**: Configure `sasl` for authentication
3. **Credentials**: Use environment variables or secrets management
4. **Certificate Validation**: Never use `insecureSkipVerify` in production

## Integration Points

The Kafka publisher integrates at the event dispatcher level:

- **Connect Events**: Published when devices connect
- **Disconnect Events**: Published when devices disconnect
- **Message Events**: Published for all routable WRP messages with `event:` scheme

## Backward Compatibility

- Kafka publishing is optional and disabled by default
- Existing functionality is unchanged when Kafka is disabled
- No breaking changes to existing APIs or interfaces
- Tests updated to maintain compatibility

## Dependencies

### Primary
- `github.com/xmidt-org/wrpkafka` - WRP Kafka publisher (v0.0.0-20251121012331)
- `github.com/xmidt-org/wrp-go/v5` - WRP v5 protocol (v5.4.0)

### Transitive
- `github.com/twmb/franz-go` - High-performance Kafka client (v1.20.4)
- `github.com/twmb/franz-go/pkg/kmsg` - Kafka protocol messages (v1.12.0)
- `github.com/twmb/franz-go/pkg/sasl/*` - SASL authentication

## Performance

- **Non-blocking**: Kafka publishing doesn't block HTTP delivery
- **Buffering**: Configurable buffering prevents memory issues
- **Batching**: franz-go automatically batches messages
- **Compression**: Can be enabled via `initialDynamicConfig`

## Monitoring

Logs are emitted at different levels:

- **INFO**: Publisher lifecycle (start, stop, creation)
- **DEBUG**: Individual message publishes
- **ERROR**: Configuration errors, publish failures

## Future Enhancements

Potential improvements for future versions:

1. Metrics integration (Prometheus counters for success/failure)
2. Per-event-type topic routing
3. Message filtering based on device metadata  
4. Kafka headers populated with WRP metadata
5. Schema registry integration
6. Certificate file loading for TLS
7. Dynamic configuration updates via control endpoint

## Known Limitations

1. TLS certificate files not yet loaded (TODO in code)
2. All messages go to a single topic (can be extended with routing)
3. No built-in metrics (relies on wrpkafka's event system)
4. Status field is pointer type in v5 (auto-handled in conversion)

## Verification Steps

1. ✅ Code compiles successfully
2. ✅ Unit tests pass
3. ✅ Configuration parsing works
4. ✅ WRP v3 to v5 conversion works
5. ✅ No-op publisher works when disabled
6. ✅ Integration with event dispatcher
7. ✅ Backward compatibility maintained

## Deployment Checklist

Before deploying to production:

- [ ] Configure Kafka brokers
- [ ] Set appropriate topic name
- [ ] Enable TLS encryption
- [ ] Configure SASL authentication
- [ ] Set reasonable buffer limits
- [ ] Review timeout settings
- [ ] Test connectivity to Kafka cluster
- [ ] Monitor logs for errors
- [ ] Verify messages appear in Kafka
- [ ] Test failover behavior
