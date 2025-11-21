<-- SPDX-FileCopyrightText: 2025 Comcast Cable Communications Management, LLC
SPDX-License-Identifier: Apache-2.0 -->

# Kafka Publisher Quick Reference

## Quick Start

### 1. Minimum Configuration

Add to `talaria.yaml`:

```yaml
kafka:
  enabled: true
  topic: "wrp-events"
  brokers:
    - "localhost:9092"
```

### 2. With TLS and SASL

```yaml
kafka:
  enabled: true
  topic: "wrp-events"
  brokers:
    - "kafka-1.example.com:9093"
    - "kafka-2.example.com:9093"
  tls:
    enabled: true
    insecureSkipVerify: false
  sasl:
    mechanism: "SCRAM-SHA-512"
    username: "talaria-producer"
    password: "${KAFKA_PASSWORD}"  # Use environment variable
  maxBufferedRecords: 10000
  maxBufferedBytes: 104857600
  maxRetries: 3
  requestTimeout: "30s"
```

## Configuration Reference

| Property | Type | Default | Description |
|----------|------|---------|-------------|
| `kafka.enabled` | bool | `false` | Enable Kafka publisher |
| `kafka.topic` | string | *required* | Kafka topic for all messages |
| `kafka.brokers` | []string | *required* | Broker addresses (host:port) |
| `kafka.maxBufferedRecords` | int | `10000` | Max records in buffer |
| `kafka.maxBufferedBytes` | int | `104857600` | Max bytes in buffer (100MB) |
| `kafka.maxRetries` | int | `3` | Max retry attempts |
| `kafka.requestTimeout` | duration | `30s` | Request timeout |
| `kafka.tls.enabled` | bool | `false` | Enable TLS |
| `kafka.tls.insecureSkipVerify` | bool | `false` | Skip cert verification |
| `kafka.sasl.mechanism` | string | `""` | PLAIN, SCRAM-SHA-256, SCRAM-SHA-512 |
| `kafka.sasl.username` | string | `""` | SASL username |
| `kafka.sasl.password` | string | `""` | SASL password |

## Testing

### Check if Kafka is Enabled

Look for this log message on startup:
```
{"level":"info","msg":"Kafka publisher started and enabled"}
```

### Verify Messages in Kafka

```bash
# Using kafka-console-consumer
kafka-console-consumer \
  --bootstrap-server localhost:9092 \
  --topic wrp-events \
  --from-beginning

# Using kafkacat
kafkacat -C -b localhost:9092 -t wrp-events
```

### Test with Docker

```bash
# Start Kafka
docker run -d --name kafka -p 9092:9092 apache/kafka:latest

# Update talaria.yaml
kafka:
  enabled: true
  topic: "wrp-events"
  brokers: ["localhost:9092"]

# Start Talaria
./talaria -f talaria.yaml

# Monitor Kafka topic
docker exec kafka /opt/kafka/bin/kafka-console-consumer.sh \
  --bootstrap-server localhost:9092 \
  --topic wrp-events \
  --from-beginning
```

## Message Types Published

| Event Type | When Published | WRP Type |
|------------|----------------|----------|
| Connect | Device connects | SimpleEventMessageType |
| Disconnect | Device disconnects | SimpleEventMessageType |
| MessageReceived | Device sends message with `event:` scheme | Varies |

## QoS Levels

| WRP QoS Range | Behavior | Use Case |
|---------------|----------|----------|
| 0-24 | Fire-and-forget | Non-critical events |
| 25-74 | Async with retries | Standard messages |
| 75-99 | Synchronous with ack | Critical messages |

## Common Issues

### Issue: Kafka publisher not starting

**Symptoms**: No log messages about Kafka
**Solutions**:
- Check `kafka.enabled` is `true`
- Verify `kafka.brokers` and `kafka.topic` are set
- Review logs for configuration errors

### Issue: Messages not appearing in Kafka

**Symptoms**: Devices connected but no Kafka messages
**Solutions**:
- Verify Kafka brokers are reachable
- Check network/firewall rules
- Ensure topic exists or auto-creation is enabled
- Review Talaria logs for publish errors

### Issue: Authentication failures

**Symptoms**: Connection errors in logs
**Solutions**:
- Verify SASL mechanism matches broker config
- Check username/password are correct
- Ensure broker has SASL enabled

### Issue: TLS connection errors

**Symptoms**: TLS handshake failures
**Solutions**:
- Verify TLS is enabled on brokers
- Check certificate validity
- Ensure using correct port (usually 9093 for TLS)
- Try `insecureSkipVerify: true` for debugging (dev only!)

## Log Messages

### Success Messages
```json
{"level":"info","msg":"Creating Kafka publisher","brokers":["localhost:9092"],"topic":"wrp-events"}
{"level":"info","msg":"Kafka publisher started successfully"}
{"level":"debug","msg":"Published message to Kafka","destination":"event:...","outcome":"Success"}
```

### Error Messages
```json
{"level":"error","msg":"failed to create kafka publisher: kafka.topic is required"}
{"level":"error","msg":"Failed to publish message to Kafka","destination":"...","error":"..."}
```

## Environment Variables

Use environment variables for sensitive values:

```yaml
kafka:
  enabled: true
  topic: "${KAFKA_TOPIC:wrp-events}"  # Default to wrp-events
  brokers:
    - "${KAFKA_BROKER_1:localhost:9092}"
  sasl:
    username: "${KAFKA_USERNAME}"
    password: "${KAFKA_PASSWORD}"
```

## Performance Tuning

### High Throughput
```yaml
kafka:
  maxBufferedRecords: 50000
  maxBufferedBytes: 524288000  # 500MB
  requestTimeout: "60s"
  initialDynamicConfig:
    compression: "snappy"  # or "lz4", "zstd"
```

### Low Latency
```yaml
kafka:
  maxBufferedRecords: 1000
  maxBufferedBytes: 10485760  # 10MB
  requestTimeout: "5s"
  maxRetries: 1
```

### Balanced
```yaml
kafka:
  maxBufferedRecords: 10000
  maxBufferedBytes: 104857600  # 100MB
  requestTimeout: "30s"
  maxRetries: 3
```

## Security Best Practices

1. ✅ Always use TLS in production
2. ✅ Always use SASL authentication
3. ✅ Never commit credentials to git
4. ✅ Use environment variables or secrets
5. ✅ Never use `insecureSkipVerify: true` in production
6. ✅ Rotate credentials regularly
7. ✅ Use least-privilege ACLs in Kafka

## Disabling Kafka

To disable Kafka publishing:

```yaml
kafka:
  enabled: false
```

Or simply remove the `kafka` section from the config.

## Advanced: Dynamic Configuration

The `initialDynamicConfig` allows advanced routing and configuration:

```yaml
kafka:
  enabled: true
  topic: "default-topic"  # Fallback topic
  brokers: ["localhost:9092"]
  initialDynamicConfig:
    # Route different event types to different topics
    topicMap:
      - pattern: "online"
        topic: "device-lifecycle"
      - pattern: "offline"
        topic: "device-lifecycle"
      - pattern: "device-status-*"
        topic: "device-status"
      - pattern: "*"  # Catch-all
        topic: "device-events"
    
    # Enable compression
    compression: "snappy"
    
    # Add headers to all messages
    headers:
      - "X-Producer: talaria"
      - "X-Version: v1"
```

## Monitoring Checklist

- [ ] Check Talaria logs for Kafka errors
- [ ] Monitor Kafka consumer lag
- [ ] Verify message count in topic matches expectations
- [ ] Check broker health
- [ ] Monitor disk space on Kafka brokers
- [ ] Review producer metrics (if available)

## Support

For issues or questions:

1. Check [docs/kafka-publisher.md](./docs/kafka-publisher.md)
2. Review [KAFKA_IMPLEMENTATION.md](./KAFKA_IMPLEMENTATION.md)
3. Check Talaria logs
4. Verify Kafka broker logs
5. Test connectivity with `kafka-console-producer`
