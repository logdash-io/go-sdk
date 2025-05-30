package logdash_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/logdash-io/go-sdk/logdash"
	"github.com/stretchr/testify/assert"
)

type (
	requestAndBody struct {
		request      *http.Request
		body         []byte
		timeReceived time.Time
	}

	requestsCollector struct {
		requests []requestAndBody
		mu       sync.Mutex
	}
)

func (c *requestsCollector) add(t *testing.T, request *http.Request) {
	body, err := io.ReadAll(request.Body)
	assert.NoError(t, err)

	c.mu.Lock()
	defer c.mu.Unlock()
	c.requests = append(c.requests, requestAndBody{request: request, body: body, timeReceived: time.Now()})
}

func assertRequestAndBody(t *testing.T, rb requestAndBody, expectedMethod, expectedPath, expectedAPIKey string, expectedBody map[string]any, beforeRequest time.Time) map[string]any {
	t.Helper()
	assert.Equal(t, expectedMethod, rb.request.Method)
	assert.Equal(t, expectedPath, rb.request.URL.Path)
	assert.Equal(t, expectedAPIKey, rb.request.Header.Get("project-api-key"))

	var actualBody map[string]any
	err := json.Unmarshal(rb.body, &actualBody)
	assert.NoError(t, err)

	for key, expectedValue := range expectedBody {
		if key == "createdAt" || key == "timestamp" {
			// Handle timestamp validation
			timestamp, err := time.Parse(time.RFC3339Nano, actualBody[key].(string))
			assert.NoError(t, err)
			assert.WithinRange(t, timestamp, beforeRequest, rb.timeReceived)
			continue
		}

		if expectedValue == nil {
			// If expected value is nil, only check if the field exists
			assert.Contains(t, actualBody, key, "field %s should exist", key)
			continue
		}

		assert.Equal(t, expectedValue, actualBody[key], "body field %s mismatch", key)
	}

	return actualBody
}

func TestLogdashLoggerInfoOneLog(t *testing.T) {
	t.Run("should send info log to the server", func(t *testing.T) {
		// GIVEN
		requestsCollector := &requestsCollector{}

		httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer r.Body.Close()
			w.WriteHeader(http.StatusOK)

			requestsCollector.add(t, r)
		}))

		defer httpServer.Close()

		// WHEN
		ld := logdash.New(logdash.LogdashConfig{
			Host:    httpServer.URL,
			APIKey:  "test-api-key",
			Verbose: true,
		})

		beforeLogSent := time.Now()
		ld.Logger.Info("Hello, World!")
		err := ld.Shutdown(context.Background())

		// THEN
		assert.NoError(t, err)

		assert.Len(t, requestsCollector.requests, 1)
		r := requestsCollector.requests[0]

		expectedBody := map[string]any{
			"level":          "info",
			"message":        "Hello, World!",
			"createdAt":      nil, // Will be validated as timestamp
			"sequenceNumber": nil, // Will only check if field exists
		}
		assertRequestAndBody(t, r, http.MethodPost, "/logs", "test-api-key", expectedBody, beforeLogSent)
	})
}

func TestLogdashMetricSetMetric(t *testing.T) {
	t.Run("should send metric to the server", func(t *testing.T) {
		// GIVEN
		requestsCollector := &requestsCollector{}

		httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer r.Body.Close()
			w.WriteHeader(http.StatusOK)

			requestsCollector.add(t, r)
		}))

		defer httpServer.Close()

		// WHEN
		ld := logdash.New(logdash.LogdashConfig{
			Host:    httpServer.URL,
			APIKey:  "test-api-key",
			Verbose: true,
		})

		beforeMetricSent := time.Now()
		ld.Metrics.Set("test-metric", 42)
		err := ld.Shutdown(context.Background())

		// THEN
		assert.NoError(t, err)

		assert.Len(t, requestsCollector.requests, 1)
		r := requestsCollector.requests[0]

		expectedBody := map[string]any{
			"name":      "test-metric",
			"value":     float64(42),
			"operation": "set",
			"timestamp": nil, // Will be validated as timestamp
		}
		assertRequestAndBody(t, r, http.MethodPut, "/metrics", "test-api-key", expectedBody, beforeMetricSent)
	})
}
