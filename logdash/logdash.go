package logdash

import (
	"context"
	"time"

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

		// events sends the events of [Logdash.Track] to the server, nil without an API key.
		events *asyncProcessor[eventEntry]

		// internalLogger is the logger used to log messages to the console.
		internalLogger *Logger
	}

	// Option is a function that configures a Logdash instance.
	Option func(*options)

	// options contains all the configuration options for Logdash.
	options struct {
		host           string
		apiKey         string
		app            string
		verbose        bool
		bufferSize     int
		overflowPolicy OverflowPolicy
		httpTimeout    time.Duration
		httpRetries    int
		httpRetryMin   time.Duration
		httpRetryMax   time.Duration
	}

	// OverflowPolicy defines how to handle log and event overflow.
	OverflowPolicy int
)

const (
	// OverflowPolicyDrop drops new logs and events when the internal buffer is full.
	//
	// This is the default behavior.
	OverflowPolicyDrop OverflowPolicy = iota

	// OverflowPolicyBlock blocks when the internal buffer is full.
	//
	// This is useful when you want to preserve logs and events even when the internal buffer is full.
	OverflowPolicyBlock
)

var (
	// DefaultBufferSize is the default size of the buffer for the async queue.
	DefaultBufferSize = 128
)

// WithHost sets the host for the Logdash server.
func WithHost(host string) Option {
	return func(o *options) {
		o.host = host
	}
}

// WithAPIKey sets the API key for the Logdash server.
func WithAPIKey(apiKey string) Option {
	return func(o *options) {
		o.apiKey = apiKey
	}
}

// WithApp sets the name of the app that sends the logs, metrics and events, for example "api" or "desktop".
//
// All apps of a domain share one domain key, and the app name tells them apart in the dashboard.
// Older service keys ignore it.
func WithApp(app string) Option {
	return func(o *options) {
		o.app = app
	}
}

// WithVerbose enables verbose logging.
//
// This is useful for debugging, showing internal logs and changes in the metrics.
func WithVerbose() Option {
	return func(o *options) {
		o.verbose = true
	}
}

// WithBufferSize sets the size of the buffer for the async queue.
func WithBufferSize(size int) Option {
	return func(o *options) {
		o.bufferSize = size
	}
}

// WithOverflowPolicy sets how to handle log and event overflow.
func WithOverflowPolicy(policy OverflowPolicy) Option {
	return func(o *options) {
		o.overflowPolicy = policy
	}
}

// WithHTTPTimeout sets the timeout for HTTP requests.
func WithHTTPTimeout(timeout time.Duration) Option {
	return func(o *options) {
		o.httpTimeout = timeout
	}
}

// WithHTTPRetries sets the number of retries for HTTP requests.
func WithHTTPRetries(retries int) Option {
	return func(o *options) {
		o.httpRetries = retries
	}
}

// WithHTTPRetryMin sets the minimum duration for HTTP retries.
func WithHTTPRetryMin(min time.Duration) Option {
	return func(o *options) {
		o.httpRetryMin = min
	}
}

// WithHTTPRetryMax sets the maximum duration for HTTP retries.
func WithHTTPRetryMax(max time.Duration) Option {
	return func(o *options) {
		o.httpRetryMax = max
	}
}

// New creates a new Logdash instance with the given options.
//
// By default, the Logdash will use the Logdash API at https://api.logdash.io.
//
// If no API key is provided, the Logdash will not send any logs, metrics or events to the server.
// Logging to the console is always enabled.
//
// The default buffer size is 128 (see: [DefaultBufferSize]).
//
// The default overflow policy is [OverflowPolicyDrop], to avoid blocking the logging thread.
// For preserving logs in case of overflow, use [WithOverflowPolicy] to set [OverflowPolicyBlock].
//
// The default HTTP settings are:
//   - timeout: 5 seconds (see: [WithHTTPTimeout]).
//   - retries: 3 (see: [WithHTTPRetries]).
//   - retry minimum interval: 1 second (see: [WithHTTPRetryMin]).
//   - retry maximum interval: 30 seconds (see: [WithHTTPRetryMax]).
func New(opts ...Option) *Logdash {
	o := &options{
		host:           "https://api.logdash.io",
		bufferSize:     DefaultBufferSize,
		overflowPolicy: OverflowPolicyDrop,
		httpTimeout:    5 * time.Second,
		httpRetries:    3,
		httpRetryMin:   1 * time.Second,
		httpRetryMax:   30 * time.Second,
	}

	for _, opt := range opts {
		opt(o)
	}

	ld := &Logdash{}
	ld.setup(o)
	return ld
}

func (ld *Logdash) setup(o *options) {
	ld.setupInternalLogger(o)
	ld.setupLogger(o)
	ld.setupMetrics(o)
	ld.setupEvents(o)
}

func (ld *Logdash) setupInternalLogger(o *options) {
	if o.verbose {
		ld.internalLogger = newLogger(newConsoleLogger())
	} else {
		ld.internalLogger = newLogger(newNoopLogger())
	}
}

func (ld *Logdash) setupLogger(o *options) {
	if o.apiKey != "" {
		ld.internalLogger.VerboseF("Creating Logger with host %s", o.host)
		httpLogger := newHTTPLogger(o, ld.internalLogger, o.bufferSize)
		httpLogger.SetOverflowPolicy(o.overflowPolicy)
		ld.Logger = newLogger(
			newConsoleLogger(),
			httpLogger,
		)
	} else {
		ld.internalLogger.Warn("No API key provided, using local logger only")
		ld.Logger = newLogger(newConsoleLogger())
	}
}

func (ld *Logdash) setupMetrics(o *options) {
	var innerMetrics Metrics

	if o.apiKey != "" {
		ld.internalLogger.VerboseF("Creating Metrics with host %s", o.host)
		httpMetrics := newHTTPMetrics(o, ld.internalLogger)
		innerMetrics = httpMetrics
	} else {
		ld.internalLogger.Warn("No API key provided, using noop metrics")
		innerMetrics = noopMetrics{}
	}

	ld.Metrics = newVerboseLogMetricsWrapper(ld.internalLogger, innerMetrics)
}

// Shutdown stops accepting logs, metrics and events, and waits until the queued ones are sent or ctx is done.
func (ld *Logdash) Shutdown(ctx context.Context) error {
	errg, _ := errgroup.WithContext(ctx)
	errg.Go(func() error {
		return ld.Logger.Shutdown(ctx)
	})
	errg.Go(func() error {
		return ld.Metrics.Shutdown(ctx)
	})
	if ld.events != nil {
		errg.Go(func() error {
			return ld.events.Shutdown(ctx)
		})
	}
	return errg.Wait()
}

// Close stops accepting logs, metrics and events, without waiting for the queued ones to be sent.
func (ld *Logdash) Close() error {
	errg, _ := errgroup.WithContext(context.Background())
	errg.Go(ld.Logger.Close)
	errg.Go(ld.Metrics.Close)
	if ld.events != nil {
		errg.Go(ld.events.Close)
	}
	return errg.Wait()
}
