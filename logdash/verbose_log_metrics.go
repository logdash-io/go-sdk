package logdash

import "context"

type verboseLogMetricsWrapper struct {
	logger  *Logger
	metrics Metrics
}

func newVerboseLogMetricsWrapper(logger *Logger, metrics Metrics) *verboseLogMetricsWrapper {
	return &verboseLogMetricsWrapper{
		logger:  logger,
		metrics: metrics,
	}
}

func (v *verboseLogMetricsWrapper) Set(name string, value float64) {
	v.logger.VerboseF("Setting metric %s to %f", name, value)
	v.metrics.Set(name, value)
}

func (v *verboseLogMetricsWrapper) Mutate(name string, value float64) {
	v.logger.VerboseF("Mutating metric %s by %f", name, value)
	v.metrics.Mutate(name, value)
}

func (v *verboseLogMetricsWrapper) Shutdown(ctx context.Context) error {
	return v.metrics.Shutdown(ctx)
}

func (v *verboseLogMetricsWrapper) Close() error {
	return v.metrics.Close()
}
