# Copyright (c) 2026 RuturajS (ROne). All rights reserved.
# This code belongs to the author. No modification or republication 
# is allowed without explicit permission.
# Build Stage
FROM golang:1.22-alpine AS builder

# Set working directory
WORKDIR /app

# Install build dependencies
RUN apk add --no-cache gcc muscular-dev

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
# Use -ldflags="-s -w" for a smaller binary
# CGO_ENABLED=0 ensures a static binary for Alpine/distroless
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o rone .

# Final Stage
FROM alpine:3.19

# Set working directory
WORKDIR /app

# Install runtime dependencies for the "Tools" feature
# Including basic utilities the LLM might want to use
RUN apk add --no-cache \
    bash \
    procps \
    coreutils \
    iproute2 \
    curl \
    util-linux \
    ca-certificates

# Create a non-root user for security (optional but recommended)
# Note: Tools feature might need higher privileges for some commands,
# but for basic ones, a non-root user is safer.
RUN addgroup -S rone && adduser -S rone -G rone
USER rone

# Copy the binary from the builder stage
COPY --from=builder /app/rone /app/rone

# Create a place for the config file
# Users should mount their own config.yaml here
VOLUME ["/app/config.yaml"]

# Define the entrypoint
ENTRYPOINT ["/app/rone", "start", "--foreground"]

