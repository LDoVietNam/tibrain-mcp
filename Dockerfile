# TiBrain - Central Intelligence Hub for Multi-CLI Coordination
# Multi-stage build for minimal image size

FROM golang:1.25-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

# Copy go.mod and download dependencies
COPY go.mod go.sum* ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o tibrain .

# Final stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Create data directory
RUN mkdir -p /app/data

# Copy binary from builder
COPY --from=builder /app/tibrain .
COPY --from=builder /app/config.yaml .

# Expose ports
EXPOSE 3005 1840

# Create non-root user
RUN addgroup -g 1001 -S tibrain && \
    adduser -u 1001 -S tibrain -G tibrain
USER tibrain

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --timeout=5 http://localhost:3005/health || exit 1

# Run the application
CMD ["./tibrain"]