# SPDX-FileCopyrightText: 2026 Comcast Cable Communications Management, LLC
# SPDX-License-Identifier: Apache-2.0
.PHONY: test test-unit test-integration test-all clean

# IMPORTANT: -tags="" is explicitly set to exclude files with build tags
# Without this flag, Go may include integration test files despite having //go:build integration tags

# Run unit tests only (excludes integration tests via build tags)
test-unit:
	@echo "Running unit tests..."
	@go test -v -tags="" -timeout=30s ./...

# Run integration tests only
test-integration:
	@echo "Running integration tests..."
	@go test -v -tags=integration -timeout=10m ./...

# Run all tests (unit + integration)
test-all: test-unit test-integration

# Default test target (unit tests only)
test: test-unit

# Clean test cache and build artifacts
clean:
	@go clean -testcache
	@rm -f talaria_test talaria *_test.yaml
	@echo "Cleaned test cache and build artifacts"

# Build the application
build:
	@go build -o talaria .

# Run linter
lint:
	@./lint.sh

# Help target
help:
	@echo "Available targets:"
	@echo "  test           - Run unit tests (default)"
	@echo "  test-unit      - Run unit tests only"
	@echo "  test-integration - Run integration tests only"
	@echo "  test-all       - Run both unit and integration tests"
	@echo "  build          - Build the talaria binary"
	@echo "  lint           - Run linter"
	@echo "  clean          - Clean test cache and artifacts"
	@echo "  help           - Show this help message"
