# Build stage
FROM golang:1.25-alpine AS builder

# Install ca-certificates for HTTPS requests
RUN apk add --no-cache ca-certificates git tzdata

WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download && go mod verify

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build \
    -a -installsuffix cgo \
    -ldflags='-w -s -extldflags "-static"' \
    -o strangle-bot \
    cmd/bot/main.go

# Runtime stage
FROM scratch

# Import ca-certificates from builder
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Import timezone data
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Copy the binary
COPY --from=builder /app/strangle-bot /strangle-bot

# Copy config template (user should mount actual config)
COPY --from=builder /app/config.yaml.example /config.yaml.example

# Run as non-root user
USER 65534:65534

EXPOSE 8080

ENTRYPOINT ["/strangle-bot"]