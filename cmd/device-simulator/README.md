# Device Simulator

A simple Go program that simulates a device connecting to Talaria via WebSocket.

## Features

- Requests a JWT token from Themis
- Establishes a WebSocket connection to Talaria with proper authentication
- Sends periodic pings to keep the connection alive
- Listens for messages from Talaria
- Gracefully handles shutdown

## Usage

### Build

```bash
cd cmd/device-simulator
go build -o device-simulator .
```

### Run with defaults

```bash
./device-simulator
```

This will connect with:
- Themis URL: `http://localhost:6500/issue`
- Talaria URL: `ws://localhost:6200/api/v2/device`
- Device ID: `mac:112233445566`
- Serial: `1800deadbeef`

### Run with custom parameters

```bash
./device-simulator \
  -themis "http://localhost:6500/issue" \
  -talaria "ws://localhost:6200/api/v2/device" \
  -device-id "mac:aabbccddeeff" \
  -serial "1234567890ab" \
  -ping-interval 30s
```

### Command-line flags

- `-themis`: Themis URL for JWT token (default: `http://localhost:6500/issue`)
- `-talaria`: Talaria WebSocket URL (default: `ws://localhost:6200/api/v2/device`)
- `-device-id`: Device ID/MAC address (default: `mac:112233445566`)
- `-serial`: Device serial number (default: `1800deadbeef`)
- `-ping-interval`: Interval for sending pings (default: `30s`)

## Example Output

```
2025/12/03 10:00:00 Device Simulator Starting...
2025/12/03 10:00:00 Device ID: mac:112233445566
2025/12/03 10:00:00 Serial: 1800deadbeef
2025/12/03 10:00:00 Themis URL: http://localhost:6500/issue
2025/12/03 10:00:00 Talaria URL: ws://localhost:6200/api/v2/device
2025/12/03 10:00:00 Requesting JWT token from Themis at http://localhost:6500/issue
2025/12/03 10:00:00 ✓ Successfully obtained JWT token (expires at 2025-12-04 10:00:00 +0000 UTC)
2025/12/03 10:00:00 Token: eyJhbGciOiJSUzI1NiIsImtpZCI6ImRldmVsb3BtZW50Iiwid...
2025/12/03 10:00:00 Connecting to Talaria at ws://localhost:6200/api/v2/device
2025/12/03 10:00:00 ✓ Successfully connected to Talaria!
2025/12/03 10:00:00 Connection established with device ID: mac:112233445566
2025/12/03 10:00:00 Device simulator running. Press Ctrl+C to exit.
2025/12/03 10:00:30 Sent ping
2025/12/03 10:01:00 Sent ping
```

## Integration with Tests

You can use this simulator in integration tests instead of testcontainers:

```go
cmd := exec.Command("./device-simulator", 
    "-themis", themisURL,
    "-talaria", "ws://localhost:6200/api/v2/device",
    "-device-id", "mac:4ca161000109",
)
cmd.Start()
defer cmd.Process.Kill()
```

## Stopping the Simulator

Press `Ctrl+C` to gracefully shutdown the simulator. It will send a proper WebSocket close message before exiting.
