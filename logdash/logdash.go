package logdash

import (
	"context"

	"golang.org/x/sync/errgroup"
)

type (
	// Logdash is the main object exposing the Logdash API.
	Logdash struct {
		// Logger is the logger used to log messages to the Logdash server.
		//
		// If no API key is provided, the Logdash will not send any logs to the server.
		// Logging to the console is always enabled.
		Logger *Logger

		// Metrics is the metrics object used to track metrics.
		//
		// If no API key is provided, the Logdash will not send any metrics to the server.
		Metrics Metrics

		// internalLogger is the logger used to log messages to the console.
		internalLogger *Logger
	}

	// LogdashConfig is the configuration for the Logdash.
	LogdashConfig struct {
		// Host is the host of the Logdash server.
		Host string

		// APIKey is the API key for the Logdash server.
		// If not provided, the Logdash will not send any logs nor metrics to the server.
		APIKey string

		// Verbose is a flag to enable internal logging.
		Verbose bool

		// LogAsyncSettings is the settings for the async logger.
		//
		// If not provided, the copy of [DefaultAsyncSettings] will be used.
		LogAsyncSettings *AsyncSettings
	}

	AsyncSettings struct {
		// BufferSize is the size of the buffer for the async queue.
		BufferSize int

		// OverflowPolicy defines how to handle log overflow.
		//
		// This options is ignored for metrics.
		OverflowPolicy OverflowPolicy
	}

	// OverflowPolicy defines how to handle log overflow.
	OverflowPolicy int
)

const (
	// OverflowPolicyBlock blocks when channel is full.
	OverflowPolicyBlock OverflowPolicy = iota
	// OverflowPolicyDrop drops new logs when channel is full.
	OverflowPolicyDrop
)

var (
	// DefaultAsyncSettings is the default settings for the async operations.
	//
	// BufferSize is set to 128 and OverflowPolicy is set to OverflowPolicyDrop.
	DefaultAsyncSettings = AsyncSettings{
		BufferSize:     128,
		OverflowPolicy: OverflowPolicyDrop,
	}
)

func New(config LogdashConfig) *Logdash {
	if config.Host == "" {
		config.Host = "https://api.logdash.io"
	}

	logAsyncSettings := DefaultAsyncSettings
	if config.LogAsyncSettings != nil {
		logAsyncSettings = *config.LogAsyncSettings
	}

	ld := &Logdash{}

	ld.setupInternalLogger(config.Verbose)
	ld.setupLogger(config.Host, config.APIKey, logAsyncSettings)
	ld.setupMetrics(config.Host, config.APIKey)

	return ld
}

func (ld *Logdash) setupInternalLogger(verbose bool) {
	if verbose {
		ld.internalLogger = newLogger(newConsoleLogger())
	} else {
		ld.internalLogger = newLogger(newNoopLogger())
	}
}

func (ld *Logdash) setupLogger(host string, apiKey string, asyncSettings AsyncSettings) {
	if apiKey != "" {
		ld.internalLogger.VerboseF("Creating Logger with host %s", host)
		httpLogger := newHTTPLogger(host, apiKey, ld.internalLogger, asyncSettings.BufferSize)
		httpLogger.SetOverflowPolicy(asyncSettings.OverflowPolicy)
		ld.Logger = newLogger(
			newConsoleLogger(),
			httpLogger,
		)
	} else {
		ld.internalLogger.Warn("No API key provided, using local logger only")
		ld.Logger = newLogger(newConsoleLogger())
	}
}

func (ld *Logdash) setupMetrics(host string, apiKey string) {
	var innerMetrics Metrics

	if apiKey != "" {
		ld.internalLogger.VerboseF("Creating Metrics with host %s", host)
		httpMetrics := newHTTPMetrics(host, apiKey, ld.internalLogger)
		innerMetrics = httpMetrics
	} else {
		ld.internalLogger.Warn("No API key provided, using noop metrics")
		innerMetrics = noopMetrics{}
	}

	ld.Metrics = newVerboseLogMetricsWrapper(ld.internalLogger, innerMetrics)
}

func (ld *Logdash) Shutdown(ctx context.Context) error {
	errg, _ := errgroup.WithContext(ctx)
	errg.Go(func() error {
		return ld.Logger.Shutdown(ctx)
	})
	errg.Go(func() error {
		return ld.Metrics.Shutdown(ctx)
	})
	return errg.Wait()
}

func (ld *Logdash) Close() error {
	errg, _ := errgroup.WithContext(context.Background())
	errg.Go(ld.Logger.Close)
	errg.Go(ld.Metrics.Close)
	return errg.Wait()
}
