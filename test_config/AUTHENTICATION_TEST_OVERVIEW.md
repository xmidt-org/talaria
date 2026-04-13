<!-- SPDX-FileCopyrightText: 2025 Comcast Cable Communications Management, LLC -->
<!-- SPDX-License-Identifier: Apache-2.0 -->

# Authentication Testing Overview

This document describes the authentication test suite for Talaria's four main endpoints.

### Performance Optimization (In Progress)
- **Current state**: Still excessively long for CI/CD environments - over 10 minutes
- **Additional improvements needed**:
  - Further container startup optimization
  - Parallel test execution where safe
  - More granular service selection
  - Configurable wait times

### Authentication Features (In Progress)
- **Current state**: The authentication by endpoint feature is still under development
  - Expected results for these tests will need to be updated
  - This is an early check-in to allow early testing


## Target Endpoints

The test suite covers authentication for these four endpoints:

1. **GET `/api/v2/devices`** - API endpoint for listing connected devices
2. **GET `/api/v2/device/{deviceID}/stat`** - API endpoint for device statistics  
3. **POST `/api/v2/device/send`** - API endpoint for sending messages to devices
4. **WebSocket `/api/v2/device`** - Device connection endpoint for WebSocket connections

## Current Test Coverage

### Core Authentication Tests

#### `TestGetDevices_Auth`

- **Purpose**: Validates authentication for device listing endpoint
- **Scenarios**:
  - Valid basic auth credentials → 200 OK
  - Invalid basic auth credentials → 401 Unauthorized
  - No auth credentials → 401 Unauthorized
  - Valid API JWT token → 200 OK
  - Device JWT on API endpoint → Currently succeeds (will be 401 when 2-validator support added)

#### `TestGetDeviceStat_Auth`  

- **Purpose**: Validates authentication for device statistics endpoint
- **Scenarios**:
  - Valid basic auth credentials → 200 OK
  - Invalid basic auth credentials → 401 Unauthorized  
  - No auth credentials → 401 Unauthorized
  - Valid API JWT token → 200 OK

#### `TestPostDeviceSend_Auth`

- **Purpose**: Validates authentication for message sending endpoint
- **Scenarios**:
  - Valid basic auth credentials with proper WRP payload → 200 OK
  - Invalid basic auth credentials → 401 Unauthorized
  - No auth credentials → 401 Unauthorized  
  - Valid API JWT token → 200 OK

#### `TestDeviceConnect_Auth`

- **Purpose**: Validates authentication for WebSocket device connections
- **Scenarios**:
  - Valid Device JWT token → 101 Switching Protocols
  - Invalid JWT token → 401 Unauthorized
  - No auth credentials → May succeed (depends on failOpen config)

#### `TestExpiredJWT` (Modified for reliability)

- **Purpose**: Demonstrates expired token handling workflow  
- **Note**: Uses standard tokens with simulated expiration timing rather than actual expired tokens for test reliability

#### `TestTrustedVsUntrustedJWT` (Currently disabled)

- **Purpose**: Will test JWT issuer trust validation when Talaria supports multiple validators
- **Status**: Skipped until Talaria implements separate device vs API JWT validators

### Infrastructure Tests

#### `TestMultipleThemisInstances`

- **Purpose**: Tests multiple JWT issuer configurations
- **Features**:
  - Single Themis instance (default behavior)
  - Multiple Themis instances with different capabilities
  - JWT token generation from specific instances
  - Verification that different instances produce different tokens

#### `TestHelloWorld`

- **Purpose**: Basic integration test verifying test fixture setup
- **Validates**: Test environment works with simple authenticated API call

#### `TestSetupOptions`

- **Purpose**: Demonstrates flexible service startup configurations
- **Covers**: API-only services, Themis-only, full stack variations
- **Options implemented**:
  - `WithFullStack()` - All services required by tests (currently Kafka + Themis + Caduceus + xmidt-agent)
  - `WithAPIServices()` - Auth + monitoring only (Themis + Caduceus)
  - `WithKafka()`, `WithThemis()`, `WithCaduceus()`, `WithXmidtAgent()` - Individual services
  - `WithoutKafka()`, `WithoutThemis()` - Selective exclusion from presets

## Authentication Methods Tested

### Basic Authentication

- Username/password combinations
- Valid credentials: `user:pass`
- Invalid credentials: `wrong:wrong`
- Missing credentials: empty values

### JWT Token Authentication  

- Bearer token format: `Authorization: Bearer <jwt>`
- Multiple JWT issuer sources (Themis instances)
- Token validation against different endpoints

### Mock TR-181 Parameters

- **File**: `test_config/mock_tr181.json` 
- **Content**: 75 abstracted device parameters
- **Format**: Generic property names (`Device.Info.Property1`, `Device.Service1.Instance1.Enable`)
- **Values**: Generic test data (`TestVendor`, `TestModel`, `interface0`, `192.168.1.1`)

### Configuration Files

- **Talaria**: `test_config/talaria_integration_template.yaml` - Template with placeholders
- **Themis**: Multiple configurations (`themis.yaml`, `themis_read_only.yaml`, etc.)
- **xmidt-agent**: `test_config/xmidt-agent.yaml` - Generic test device configuration

## TODO: Next Tests

- [ ] **Invalid JWT Tokens**
  - NBF (Not Before) value is honored
  - Partner ID is enforced

- [ ] **Token Handling**
  - No sensitive data in logs/obfuscated
  - No sensitive data in error messages

- [ ] **Malformed JWT Tokens**
  - Invalid base64 encoding
  - Missing JWT segments (header/payload/signature)  
  - Invalid JSON in JWT payload
  - Corrupted signature

## Possible Future Tests when support is added

- [ ] **JWT Claim Validation (not supported)**
  - Wrong audience (`aud` claim) 
  - Wrong issuer (`iss` claim)
  - Missing required claims (`sub`, `exp`, `iat`)
  - Invalid claim data types

- [ ] **Permission/Capability Testing (not supported)**
  - JWT with insufficient capabilities for endpoint access
  - Role-based access control validation
  - Partner ID restrictions
  - Device-specific access controls

- [ ] **Rate Limiting Authentication (not supported)**
  - Multiple failed auth attempts
  - Brute force protection
  - Authentication throttling