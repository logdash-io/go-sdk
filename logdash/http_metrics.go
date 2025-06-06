package logdash

import (
	"context"
	"net/http"
	"sync"
	"time"
)

type (
	// httpMetrics implements Metrics interface for HTTP output.
	httpMetrics struct {
		client         *httpClient
		internalLogger *Logger

		// send accumulated metrics to goroutine which sends them to the server
		sendingAccumulatedChan chan metricEntry
		sendingLoopWg          sync.WaitGroup

		// send metric to goroutine which dispatches them to particular accumulator goroutines
		dispatchChan   chan metricEntry
		dispatchChanMu sync.RWMutex

		// informs about stopping the dispatcher (and all pipeline downstream)
		stoppedChan chan struct{}

		accumulatorsWg sync.WaitGroup

		stopping bool
	}

	// metricEntry represents a single metric entry to be sent to the server.
	metricEntry struct {
		Timestamp string  `json:"timestamp"`
		Name      string  `json:"name"`
		Value     float64 `json:"value"`
		Operation string  `json:"operation"`
	}
)

const (
	metricOperationSet    = "set"
	metricOperationMutate = "change"
)

// newHTTPMetrics creates a new HTTPMetrics instance.
func newHTTPMetrics(serverURL string, apiKey string, internalLogger *Logger) *httpMetrics {
	metrics := &httpMetrics{
		client:                 newHTTPClient(serverURL, apiKey),
		internalLogger:         internalLogger,
		sendingAccumulatedChan: make(chan metricEntry),
		stoppedChan:            make(chan struct{}),
		dispatchChan:           make(chan metricEntry),
	}

	metrics.sendingLoopWg.Add(1)
	go metrics.sendingLoop()
	go metrics.dispatch()

	return metrics
}

func (m *httpMetrics) dispatch() {
	defer close(m.stoppedChan)

	accumulators := make(map[string]chan metricEntry)
	for entry := range m.dispatchChan {
		if _, ok := accumulators[entry.Name]; !ok {
			accumulators[entry.Name] = make(chan metricEntry)
			m.accumulatorsWg.Add(1)
			go m.accumulate(entry.Name, accumulators[entry.Name])
		}
		accumulators[entry.Name] <- entry
	}

	// close all accumulators
	for _, c := range accumulators {
		close(c)
	}
	// wait for all accumulators to finish
	// as we want to close channel to the sending loop
	m.accumulatorsWg.Wait()

	// close channel to the sending loop
	close(m.sendingAccumulatedChan)
	// wait for the sending loop to finish: all metrics are sent
	m.sendingLoopWg.Wait()
}

func (m *httpMetrics) sendingLoop() {
	defer m.sendingLoopWg.Done()

	for entry := range m.sendingAccumulatedChan {
		if err := m.client.sendData("/metrics", http.MethodPut, entry); err != nil {
			m.internalLogger.ErrorF("Failed to send metric: %v", err)
		}
	}
}

// accumulate accumulates metrics for a given name.
// All metrics are sent to the goroutine is processed immediately:
// either sent to the sending loop or accumulated.
func (m *httpMetrics) accumulate(name string, c <-chan metricEntry) {
	defer m.accumulatorsWg.Done()

	var (
		// set to m.processChan when there is accumulated metrics to send
		// non-nil value enables sending accumulated metric
		outputChan       chan<- metricEntry
		accumulatedEntry metricEntry
	)
	accumulatedEntry.Name = name
	accumulatedEntry.Operation = metricOperationMutate

LOOP:
	for {
		select {
		case entry, ok := <-c:
			// input channel is closed
			if !ok {
				// there is no accumulated metric, we can stop the accumulator
				if outputChan == nil {
					break LOOP
				}
				// don't wait for closed input channel, because it causes spinning
				// because reading from closed channel returns zero value immediately
				c = nil
				// don't try to send nor accumulate zero value
				continue
			}
			// try send immediately only if there is no accumulated metric
			if outputChan == nil {
				select {
				case m.sendingAccumulatedChan <- entry:
					continue
				default:
				}
			}
			// accumulate metric
			accumulatedEntry.Timestamp = entry.Timestamp
			if entry.Operation == metricOperationSet {
				accumulatedEntry.Value = entry.Value
				accumulatedEntry.Operation = metricOperationSet
			} else if entry.Operation == metricOperationMutate {
				accumulatedEntry.Value += entry.Value
			}
			// enable sending accumulated metric
			if outputChan == nil {
				outputChan = m.sendingAccumulatedChan
			}

		case outputChan <- accumulatedEntry:
			m.internalLogger.VerboseF("Accumulated metrics sent: %#v", accumulatedEntry)
			outputChan = nil
			accumulatedEntry.Value = 0
			accumulatedEntry.Operation = metricOperationMutate
			if c == nil {
				break LOOP
			}

		}
	}
}

func (m *httpMetrics) sendOperation(name string, value float64, operation string) {
	entry := metricEntry{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Name:      name,
		Value:     value,
		Operation: operation,
	}

	m.dispatchChanMu.Lock()
	defer m.dispatchChanMu.Unlock()

	if m.stopping {
		m.internalLogger.VerboseF("Failed to send metric: %v", ErrAlreadyClosed)
		return
	}

	m.dispatchChan <- entry
}

// Set sets a metric to an absolute value.
func (m *httpMetrics) Set(name string, value float64) {
	m.sendOperation(name, value, metricOperationSet)
}

// Mutate changes a metric by a relative value.
func (m *httpMetrics) Mutate(name string, value float64) {
	m.sendOperation(name, value, metricOperationMutate)
}

// stopDispatcher stops the dispatcher and starts closing accumulators.
func (m *httpMetrics) stopDispatcher() (err error) {
	m.dispatchChanMu.Lock()
	defer m.dispatchChanMu.Unlock()

	if m.stopping {
		return ErrAlreadyClosed
	}

	m.stopping = true
	close(m.dispatchChan)

	return nil
}

// Close stops the background worker as soon as possible and closes the metrics.
//
// Close doesn't wait for pending metrics to be sent.
func (m *httpMetrics) Close() error {
	return m.stopDispatcher()
}

// Shutdown stops the background worker and closes the metrics.
//
// Shutdown waits for all pending metrics to be sent.
func (m *httpMetrics) Shutdown(ctx context.Context) error {
	m.internalLogger.VerboseF("Shutting down metrics")
	if err := m.stopDispatcher(); err != nil {
		return err
	}

	// wait for the process goroutine to finish
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-m.stoppedChan:
		return nil
	}
}
