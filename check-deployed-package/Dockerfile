# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Copy go.mod and source code
COPY check-deployed-package/go.mod check-deployed-package/check.go ./

# The version to check, the latest when empty: a release such as v0.1.0, or a commit
ARG LOGDASH_VERSION

# Fetch the SDK through the Go module proxy, the way users get it
RUN go get "github.com/logdash-io/go-sdk/logdash@${LOGDASH_VERSION:-latest}"

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o check ./check.go

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy the binary from builder stage
COPY --from=builder /app/check .

CMD ["./check"] 