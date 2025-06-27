package logdash

// Metrics defines the interface for metrics functionality.
//
// This is created internally as a part of the [Logdash] object and accessed via the [Logdash.Metrics] field.
type Metrics interface {
	resourceManager

	// Set sets a metric to an absolute value.
	Set(name string, value float64)

	// Mutate changes a metric by a relative value.
	Mutate(name string, value float64)
}
