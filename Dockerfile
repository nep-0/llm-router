# Build stage
FROM golang:1.25-alpine AS builder

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o llm-router .

# Runtime stage
FROM alpine:latest

# Install ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates

# Create non-root user
RUN addgroup -g 1000 llmrouter && \
    adduser -D -u 1000 -G llmrouter llmrouter

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/llm-router .

# Change ownership to non-root user
RUN chown -R llmrouter:llmrouter /app

# Switch to non-root user
USER llmrouter

# Expose default port
EXPOSE 8080

# Run the application
CMD ["./llm-router"]
