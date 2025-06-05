package logdash

// noopMetrics implements Metrics interface with no-op operations.
type noopMetrics struct {
	noopResourceManager
}

// Set sets a metric to an absolute value (no-op).
func (m noopMetrics) Set(name string, value float64) {}

// Mutate changes a metric by a relative value (no-op).
func (m noopMetrics) Mutate(name string, value float64) {}
