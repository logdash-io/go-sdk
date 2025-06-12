package main

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/logdash-io/go-sdk/logdash"
)

func main() {
	fmt.Println("=== LogDash SDK Demo ===")

	// Get Go version (equivalent to Python package version check)
	goVersion := runtime.Version()
	fmt.Printf("Using Go version: %s\n", goVersion)
	fmt.Println()

	// Get environment variables
	apiKey := os.Getenv("LOGDASH_API_KEY")
	logsSeed := os.Getenv("LOGS_SEED")
	if logsSeed == "" {
		logsSeed = "default"
	}
	metricsSeedStr := os.Getenv("METRICS_SEED")
	if metricsSeedStr == "" {
		metricsSeedStr = "1"
	}

	fmt.Printf("Using API Key: %s\n", apiKey)
	fmt.Printf("Using Logs Seed: %s\n", logsSeed)
	fmt.Printf("Using Metrics Seed: %s\n", metricsSeedStr)

	// Convert metrics seed to float64
	metricsSeed, err := strconv.ParseFloat(metricsSeedStr, 64)
	if err != nil {
		fmt.Printf("Error parsing metrics seed: %v\n", err)
		return
	}

	// Initialize LogDash
	ld := logdash.New(
		logdash.WithAPIKey(apiKey),
	)

	// Get the logger instance
	logger := ld.Logger

	// Get the metrics instance
	metrics := ld.Metrics

	// Log some messages with seed appended (equivalent to Python script)
	logger.Log("This is an info log", logsSeed)
	logger.Error("This is an error log", logsSeed)
	logger.Warn("This is a warning log", logsSeed)
	logger.Debug("This is a debug log", logsSeed)
	logger.HTTP("This is a http log", logsSeed)
	logger.Silly("This is a silly log", logsSeed)
	logger.Info("This is an info log", logsSeed)
	logger.Verbose("This is a verbose log", logsSeed)

	// Set and mutate metrics with seed
	metrics.Set("users", metricsSeed)
	metrics.Mutate("users", 1)

	// Shutdown properly, wait to ensure data is sent
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ld.Shutdown(ctx)
}
