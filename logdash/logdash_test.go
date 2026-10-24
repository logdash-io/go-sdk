package logdash_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
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
		if key == "createdAt" {
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

		assert.Equal(t, expectedValue, actualBody[key], "body field '%s' mismatch", key)
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
		ld := logdash.New(
			logdash.WithHost(httpServer.URL),
			logdash.WithAPIKey("test-api-key"),
			logdash.WithVerbose(),
		)

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

func TestLogdashShutdownImmediatelly(t *testing.T) {
	ld := logdash.New(
		logdash.WithHost("http://localhost:8080"),
		logdash.WithAPIKey("test-api-key"),
		logdash.WithVerbose(),
	)

	err := ld.Shutdown(context.Background())
	assert.NoError(t, err)
}

func TestLogdashMetricMetric(t *testing.T) {
	t.Run("should send one set metric command to the server", func(t *testing.T) {
		// GIVEN
		requestsCollector := &requestsCollector{}

		httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer r.Body.Close()
			w.WriteHeader(http.StatusOK)
			requestsCollector.add(t, r)
		}))

		defer httpServer.Close()

		// WHEN
		ld := logdash.New(
			logdash.WithHost(httpServer.URL),
			logdash.WithAPIKey("test-api-key"),
			logdash.WithVerbose(),
		)

		beforeMetricSent := time.Now()
		ld.Metrics.Set("test-metric", 42)
		ld.Metrics.Mutate("test-metric", 1)
		err := ld.Shutdown(context.Background())

		// THEN
		assert.NoError(t, err)

		assert.Len(t, requestsCollector.requests, 2)

		expectedBody := []map[string]any{
			{
				"name":      "test-metric",
				"value":     float64(42),
				"operation": "set",
			},
			{
				"name":      "test-metric",
				"value":     float64(1),
				"operation": "change",
			},
		}
		for i, r := range requestsCollector.requests {
			body := assertRequestAndBody(t, r, http.MethodPut, "/metrics", "test-api-key", expectedBody[i], beforeMetricSent)
			// the API rejects unknown fields with HTTP 400, so send exactly these
			assert.Equal(t, expectedBody[i], body)
		}
	})

	t.Run("should send accumulated metric command to the server", func(t *testing.T) {
		t.Run("when metric is set multiple times", func(t *testing.T) {
			// GIVEN
			requestsCollector := &requestsCollector{}

			kickServer := make(chan struct{})

			httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				<-kickServer
				defer r.Body.Close()
				w.WriteHeader(http.StatusOK)
				requestsCollector.add(t, r)
			}))

			defer httpServer.Close()

			// WHEN
			ld := logdash.New(
				logdash.WithHost(httpServer.URL),
				logdash.WithAPIKey("test-api-key"),
				logdash.WithVerbose(),
			)

			beforeMetricSent := time.Now()
			// first request is always sent immediately in test environment
			// because test HTTP server accepts connection immediately
			// and only then we can delay consecutive requests via kickServer channel
			ld.Metrics.Set("test-metric", 42)

			// this set/changes wont be sent because following set will override it
			ld.Metrics.Set("test-metric", 100)
			for range 1000 {
				ld.Metrics.Mutate("test-metric", 1)
			}
			// this set/changes will be sent
			ld.Metrics.Set("test-metric", 1)
			for range 10 {
				ld.Metrics.Mutate("test-metric", 1)
			}
			close(kickServer)
			err := ld.Shutdown(context.Background())

			// THEN
			assert.NoError(t, err)

			assert.Len(t, requestsCollector.requests, 2)

			expectedBody := []map[string]any{
				{
					"name":      "test-metric",
					"value":     float64(42),
					"operation": "set",
				},
				{
					"name":      "test-metric",
					"value":     float64(11),
					"operation": "set",
				},
			}
			for i, r := range requestsCollector.requests {
				assertRequestAndBody(t, r, http.MethodPut, "/metrics", "test-api-key", expectedBody[i], beforeMetricSent)
			}
		})
		t.Run("when metric is mutated multiple times", func(t *testing.T) {
			// GIVEN
			requestsCollector := &requestsCollector{}

			kickServer := make(chan struct{})

			httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				<-kickServer
				defer r.Body.Close()
				w.WriteHeader(http.StatusOK)
				requestsCollector.add(t, r)
			}))

			defer httpServer.Close()

			// WHEN
			ld := logdash.New(
				logdash.WithHost(httpServer.URL),
				logdash.WithAPIKey("test-api-key"),
				logdash.WithVerbose(),
			)

			beforeMetricSent := time.Now()
			// first request is always sent immediately in test environment
			// because test HTTP server accepts connection immediately
			// and only then we can delay consecutive requests via kickServer channel
			for range 1000 {
				ld.Metrics.Mutate("test-metric", 1)
			}
			close(kickServer)
			err := ld.Shutdown(context.Background())

			// THEN
			assert.NoError(t, err)

			assert.Len(t, requestsCollector.requests, 2)

			expectedBody := []map[string]any{
				{
					"name":      "test-metric",
					"value":     float64(1),
					"operation": "change",
				},
				{
					"name":      "test-metric",
					"value":     float64(999),
					"operation": "change",
				},
			}
			for i, r := range requestsCollector.requests {
				assertRequestAndBody(t, r, http.MethodPut, "/metrics", "test-api-key", expectedBody[i], beforeMetricSent)
			}
		})
	})
}

// sendLogsAndMetric logs through the SDK logger and a slog group, sets a metric,
// and returns the JSON bodies the server received, by path.
func sendLogsAndMetric(t *testing.T, opts ...logdash.Option) map[string][]map[string]any {
	t.Helper()

	requestsCollector := &requestsCollector{}

	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		w.WriteHeader(http.StatusOK)
		requestsCollector.add(t, r)
	}))

	defer httpServer.Close()

	ld := logdash.New(append([]logdash.Option{
		logdash.WithHost(httpServer.URL),
		logdash.WithAPIKey("test-api-key"),
	}, opts...)...)

	ld.Logger.Info("Hello, World!")
	// a child logger: a slog group on top of the SDK logger
	slogHandler := logdash.NewSlogTextHandler(ld.Logger, slog.HandlerOptions{Level: slog.LevelInfo})
	slog.New(slogHandler).WithGroup("auth").Info("Hello from slog!")
	ld.Metrics.Set("test-metric", 42)
	assert.NoError(t, ld.Shutdown(context.Background()))

	bodies := make(map[string][]map[string]any)
	for _, r := range requestsCollector.requests {
		var body map[string]any
		assert.NoError(t, json.Unmarshal(r.body, &body))
		bodies[r.request.URL.Path] = append(bodies[r.request.URL.Path], body)
	}
	assert.Len(t, bodies["/logs"], 2)
	assert.Len(t, bodies["/metrics"], 1)

	return bodies
}

func TestLogdashApp(t *testing.T) {
	t.Run("should send app with every log and metric when set", func(t *testing.T) {
		for path, bodies := range sendLogsAndMetric(t, logdash.WithApp("api")) {
			for _, body := range bodies {
				assert.Equal(t, "api", body["app"], path)
			}
		}
	})

	t.Run("should not send app key when not set", func(t *testing.T) {
		for path, bodies := range sendLogsAndMetric(t) {
			for _, body := range bodies {
				assert.NotContains(t, body, "app", path)
			}
		}
	})
}
