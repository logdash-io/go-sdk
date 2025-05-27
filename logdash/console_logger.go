package logdash

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gookit/color"
)

// consoleLogger implements syncLogger interface for console output.
type consoleLogger struct {
	// mu is used to ensure the log message is printed as a single line
	mu sync.Mutex
}

var (
	levelColors = map[logLevel]color.RGBColor{
		logLevelError:   color.RGB(231, 0, 11),  // Red
		logLevelWarn:    color.RGB(254, 154, 0), // Orange
		logLevelInfo:    color.RGB(21, 93, 252), // Blue
		logLevelHTTP:    color.RGB(0, 166, 166), // Teal
		logLevelVerbose: color.RGB(0, 166, 0),   // Green
		logLevelDebug:   color.RGB(0, 166, 62),  // Light Green
		logLevelSilly:   color.RGB(80, 80, 80),  // Gray
	}

	timestampColor = color.RGB(150, 150, 150)
)

// newConsoleLogger creates a new ConsoleLogger instance.
func newConsoleLogger() *consoleLogger {
	return &consoleLogger{}
}

const (
	// For console output, we use ISO 8601, fractional seconds with trailing zeros, no timezone info
	timestampFormat = "2006-01-02T15:04:05.0000000"
)

// syncLog implements the syncLogger interface.
func (l *consoleLogger) syncLog(timestamp time.Time, level logLevel, message string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	timestampColor.Printf("[%s] ", timestamp.Format(timestampFormat))
	levelColors[level].Print(strings.ToUpper(string(level)))
	fmt.Println("", message)
}
