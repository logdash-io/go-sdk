package logdash

import "time"

// noopLogger implements syncLogger interface with no-op operations.
type noopLogger struct{}

// newNoopLogger creates a new NoopLogger instance.
func newNoopLogger() *noopLogger {
	return &noopLogger{}
}

// syncLog implements the syncLogger interface (no-op).
func (l *noopLogger) syncLog(timestamp time.Time, level logLevel, message string) {}
