package logdash

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// syncLogger defines the internal interface for synchronous logging.
type syncLogger interface {
	resourceManager
	// syncLog logs a message with the given timestamp, level and message.
	syncLog(timestamp time.Time, level logLevel, message string)
}

// Logger is a struct that provides logging functionality.
//
// This is created internally as a part of the [Logdash] object and accessed via the [Logdash.Logger] field.
type Logger struct {
	loggers []syncLogger
}

// newLogger creates a new Logger instance with the given syncLoggers.
func newLogger(loggers ...syncLogger) *Logger {
	return &Logger{
		loggers: loggers,
	}
}

// Error logs an error message.
func (l *Logger) Error(args ...any) {
	l.log(logLevelError, args...)
}

// ErrorF logs a formatted error message.
func (l *Logger) ErrorF(format string, args ...any) {
	l.log(logLevelError, fmt.Sprintf(format, args...))
}

// Warn logs a warning message.
func (l *Logger) Warn(args ...any) {
	l.log(logLevelWarn, args...)
}

// WarnF logs a formatted warning message.
func (l *Logger) WarnF(format string, args ...any) {
	l.log(logLevelWarn, fmt.Sprintf(format, args...))
}

// Info logs an informational message.
func (l *Logger) Info(args ...any) {
	l.log(logLevelInfo, args...)
}

// InfoF logs a formatted informational message.
func (l *Logger) InfoF(format string, args ...any) {
	l.log(logLevelInfo, fmt.Sprintf(format, args...))
}

// Log is an alias for Info.
func (l *Logger) Log(args ...any) {
	l.Info(args...)
}

// LogF is an alias for InfoF.
func (l *Logger) LogF(format string, args ...any) {
	l.InfoF(format, args...)
}

// HTTP logs an HTTP-related message.
func (l *Logger) HTTP(args ...any) {
	l.log(logLevelHTTP, args...)
}

// HTTPF logs a formatted HTTP-related message.
func (l *Logger) HTTPF(format string, args ...any) {
	l.log(logLevelHTTP, fmt.Sprintf(format, args...))
}

// Verbose logs a verbose message.
func (l *Logger) Verbose(args ...any) {
	l.log(logLevelVerbose, args...)
}

// VerboseF logs a formatted verbose message.
func (l *Logger) VerboseF(format string, args ...any) {
	l.log(logLevelVerbose, fmt.Sprintf(format, args...))
}

// Debug logs a debug message.
func (l *Logger) Debug(args ...any) {
	l.log(logLevelDebug, args...)
}

// DebugF logs a formatted debug message.
func (l *Logger) DebugF(format string, args ...any) {
	l.log(logLevelDebug, fmt.Sprintf(format, args...))
}

// Silly logs a silly message (lowest priority).
func (l *Logger) Silly(args ...any) {
	l.log(logLevelSilly, args...)
}

// SillyF logs a formatted silly message (lowest priority).
func (l *Logger) SillyF(format string, args ...any) {
	l.log(logLevelSilly, fmt.Sprintf(format, args...))
}

// log is the common implementation for all logging methods.
func (l *Logger) log(level logLevel, args ...any) {
	timestamp := time.Now()
	message := formatMessage(args...)

	for _, logger := range l.loggers {
		logger.syncLog(timestamp, level, message)
	}
}

func (l *Logger) logWithAttrs(timestamp time.Time, level logLevel, attrs []string) {
	message := strings.Join(attrs, " ")
	for _, logger := range l.loggers {
		logger.syncLog(timestamp, level, message)
	}
}

// formatMessage formats the log message arguments into a single string.
func formatMessage(args ...any) string {
	strArgs := make([]string, len(args))
	for i, arg := range args {
		strArgs[i] = fmt.Sprint(arg)
	}
	return strings.Join(strArgs, " ")
}

func (l *Logger) Shutdown(ctx context.Context) error {
	var errs []error
	for _, logger := range l.loggers {
		err := logger.Shutdown(ctx)
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (l *Logger) Close() error {
	var errs []error
	for _, logger := range l.loggers {
		err := logger.Close()
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
