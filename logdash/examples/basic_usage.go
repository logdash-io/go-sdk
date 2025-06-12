package main

import (
	"context"
	"time"

	"github.com/logdash-io/go-sdk/logdash"
)

func main() {
	// Initialize logdash with your API key
	// For testing without an API key, logs will only be printed locally
	ld := logdash.New(
		logdash.WithHost("https://api.logdash.io"),
		logdash.WithAPIKey("your-api-key"),                      // Replace with your actual API key
		logdash.WithVerbose(),                                   // Enable verbose mode for development
		logdash.WithBufferSize(256),                             // Set custom buffer size
		logdash.WithOverflowPolicy(logdash.OverflowPolicyBlock), // Block when buffer is full
	)

	// Get the logger instance
	logger := ld.Logger

	// Get the metrics instance
	metrics := ld.Metrics

	// Log messages at different levels
	logger.Info("Application started")
	logger.Debug("Debug information")
	logger.Warn("Warning message")
	logger.Error("Error occurred")
	logger.HTTP("HTTP request processed")
	logger.Verbose("Verbose details")

	// Track metrics
	for i := range 5 {
		// Set absolute values
		metrics.Set("active_users", float64(100+i*10))

		// Mutate values (increment/decrement)
		metrics.Mutate("requests_count", 1)

		// Go specific: all logging methods has ...F() counterpart
		// like fmt.PrintF for fmt.Print
		logger.InfoF("Iteration %d/5 completed", i+1)
		time.Sleep(1 * time.Second)
	}

	// Go specific: Shutdown method wait for flushing all enqueued logs before closing application
	for i := range 10 {
		logger.InfoF("Fast iteration %d/10 completed", i+1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ld.Shutdown(ctx)
}
