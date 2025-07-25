package logdash

import (
	"context"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"
)

// httpLogger implements syncLogger interface for HTTP output.
type httpLogger struct {
	client         *httpClient
	internalLogger *Logger
	sequenceNumber atomic.Int64
	processor      *asyncProcessor[logEntry]
}

// logEntry represents a single log entry to be sent to the server.
type logEntry struct {
	CreatedAt      string `json:"createdAt"`
	Level          string `json:"level"`
	Message        string `json:"message"`
	SequenceNumber int64  `json:"sequenceNumber"`
}

// newHTTPLogger creates a new HTTPLogger instance.
func newHTTPLogger(o *options, internalLogger *Logger, bufferSize int) *httpLogger {
	logger := &httpLogger{
		client:         newHTTPClient(o, internalLogger),
		internalLogger: internalLogger,
	}

	// Create async processor for logs
	logger.processor = newAsyncProcessor(
		bufferSize,
		func(entry logEntry) error {
			return logger.client.sendData("/logs", http.MethodPost, entry)
		},
		func(entry logEntry, err error) {
			if err == errChannelOverflow {
				logger.internalLogger.Error("Log dropped due to channel overflow")
			} else {
				logger.internalLogger.Error(fmt.Sprintf("Failed to send log: %v", err))
			}
		},
	)

	return logger
}

// syncLog implements the syncLogger interface.
func (l *httpLogger) syncLog(timestamp time.Time, level logLevel, message string) {
	entry := logEntry{
		CreatedAt:      timestamp.UTC().Format(time.RFC3339Nano),
		Level:          string(level),
		Message:        message,
		SequenceNumber: l.sequenceNumber.Add(1) % (1 << 32),
	}

	l.processor.send(entry)
}

// Close stops the background worker and closes the logger.
func (l *httpLogger) Close() error {
	return l.processor.Close()
}

// Shutdown stops the background worker and closes the logger.
func (l *httpLogger) Shutdown(ctx context.Context) error {
	return l.processor.Shutdown(ctx)
}

// SetOverflowPolicy sets the overflow policy for the logger
func (l *httpLogger) SetOverflowPolicy(policy OverflowPolicy) {
	l.processor.SetOverflowPolicy(policy)
}
