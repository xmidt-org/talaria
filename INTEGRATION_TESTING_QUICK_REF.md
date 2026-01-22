<!-- SPDX-FileCopyrightText: 2025 Comcast Cable Communications Management, LLC -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

# Integration Test Setup - Quick Reference

## Prerequisites & Setup

### Required Dependencies
- **Go 1.19+**
- **Docker** or **Podman** (for service containers)
- **xmidt-agent** checkout in `./cmd/`

### Initial Setup
```bash
# 1. Clone xmidt-agent dependency
./test_setup/checkout_dependencies.sh

# 2. Ensure Docker/Podman is running
docker --version  # or: podman --version
```

### Running Tests
```bash
# Run all authentication integration tests
go test -v ./auth_integration_test.go ./integration_helpers_test.go -timeout=30m -tags="integration"

# Run specific test
go test -v ./auth_integration_test.go ./integration_helpers_test.go -timeout=30m -tags="integration" -run="TestGetDevices_Auth"

# Run with verbose service startup logs
go test -v ./auth_integration_test.go ./integration_helpers_test.go -timeout=30m -tags="integration" -run="TestHelloWorld"
```