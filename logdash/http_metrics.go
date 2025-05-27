package logdash

import (
	"fmt"
	"time"
)

// httpMetrics implements Metrics interface for HTTP output.
type httpMetrics struct {
	client         *httpClient
	internalLogger *Logger
	processor      *asyncProcessor[metricEntry]
}

// metricEntry represents a single metric entry to be sent to the server.
type metricEntry struct {
	Timestamp string  `json:"timestamp"`
	Name      string  `json:"name"`
	Value     float64 `json:"value"`
	Operation string  `json:"operation"`
}

// newHTTPMetrics creates a new HTTPMetrics instance.
func newHTTPMetrics(serverURL string, apiKey string, internalLogger *Logger, bufferSize int) *httpMetrics {
	metrics := &httpMetrics{
		client:         newHTTPClient(serverURL, apiKey),
		internalLogger: internalLogger,
	}

	// Create async processor for metrics
	metrics.processor = newAsyncProcessor(
		bufferSize,
		func(entry metricEntry) error {
			return metrics.client.sendData("/metrics", entry)
		},
		func(err error) {
			if err == errChannelOverflow {
				metrics.internalLogger.Error("Metric dropped due to channel overflow")
			} else {
				metrics.internalLogger.Error(fmt.Sprintf("Failed to send metric: %v", err))
			}
		},
	)

	return metrics
}

func (m *httpMetrics) sendOperation(name string, value float64, operation string) {
	entry := metricEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Name:      name,
		Value:     value,
		Operation: operation,
	}

	m.processor.send(entry)
}

// Set sets a metric to an absolute value.
func (m *httpMetrics) Set(name string, value float64) {
	m.sendOperation(name, value, "set")
}

// Mutate changes a metric by a relative value.
func (m *httpMetrics) Mutate(name string, value float64) {
	m.sendOperation(name, value, "change")
}

// Close stops the background worker and closes the metrics
func (m *httpMetrics) Close() {
	m.processor.Close()
}

// SetOverflowPolicy sets the overflow policy for the metrics
func (m *httpMetrics) SetOverflowPolicy(policy OverflowPolicy) {
	m.processor.SetOverflowPolicy(policy)
}
