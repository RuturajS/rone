# Copyright (c) 2026 RuturajS (ROne). All rights reserved.
# This code belongs to the author. No modification or republication 
# is allowed without explicit permission.
# Makefile — ROne cross-platform build targets

BINARY  := rone
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build build-linux build-windows build-all clean test migrate

# Default: build for current platform
build:
	go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY) .

# Linux AMD64
build-linux:
	GOOS=linux GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY)-linux-amd64 .

# Windows AMD64
build-windows:
	GOOS=windows GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o bin/$(BINARY)-windows-amd64.exe .

# Build all targets
build-all: build-linux build-windows

# Run tests
test:
	go test ./... -v -count=1

# Run schema migration check
migrate:
	go run . migrate

# Clean build artifacts
clean:
	rm -rf bin/

# Run in foreground (dev mode)
dev:
	go run . start --foreground --log-level debug

# Lint
lint:
	golangci-lint run ./...

