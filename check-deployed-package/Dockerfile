# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum files
COPY check-deployed-package/go.mod ./
COPY check-deployed-package/go.sum* ./

# Copy the entire logdash SDK source code
COPY logdash/ /go/src/github.com/logdash-io/go-sdk/logdash/

# Copy source code
COPY check-deployed-package/check.go ./

# Tidy modules to resolve all dependencies including transitive ones
RUN go mod tidy

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o check ./check.go

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy the binary from builder stage
COPY --from=builder /app/check .

CMD ["./check"] 