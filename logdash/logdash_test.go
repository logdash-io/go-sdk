package logdash_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/logdash-io/go-sdk/logdash"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

// sendLogsMetricAndEvent logs through the SDK logger and a slog group, sets a metric, tracks an event,
// and returns the JSON bodies the server received, by path.
func sendLogsMetricAndEvent(t *testing.T, opts ...logdash.Option) map[string][]map[string]any {
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
	ld.Track("agent_run", nil)
	assert.NoError(t, ld.Shutdown(context.Background()))

	bodies := make(map[string][]map[string]any)
	for _, r := range requestsCollector.requests {
		var body map[string]any
		assert.NoError(t, json.Unmarshal(r.body, &body))
		bodies[r.request.URL.Path] = append(bodies[r.request.URL.Path], body)
	}
	assert.Len(t, bodies["/logs"], 2)
	assert.Len(t, bodies["/metrics"], 1)
	assert.Len(t, bodies["/events"], 1)

	return bodies
}

func TestLogdashApp(t *testing.T) {
	t.Run("should send app with every log, metric and event when set", func(t *testing.T) {
		for path, bodies := range sendLogsMetricAndEvent(t, logdash.WithApp("api")) {
			for _, body := range bodies {
				assert.Equal(t, "api", body["app"], path)
			}
		}
	})

	t.Run("should not send app key when not set", func(t *testing.T) {
		for path, bodies := range sendLogsMetricAndEvent(t) {
			for _, body := range bodies {
				assert.NotContains(t, body, "app", path)
			}
		}
	})
}

// uuidV4 matches the event ids the API accepts.
var uuidV4 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// newTestServer starts a server that collects every request and answers with statuses in turn,
// repeating the last one, or with 202 Accepted when there are none.
func newTestServer(t *testing.T, statuses ...int) (*httptest.Server, *requestsCollector) {
	t.Helper()
	if len(statuses) == 0 {
		statuses = []int{http.StatusAccepted}
	}

	requestsCollector := &requestsCollector{}
	var calls atomic.Int32
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		requestsCollector.add(t, r)
		w.WriteHeader(statuses[min(int(calls.Add(1)), len(statuses))-1])
	}))
	t.Cleanup(httpServer.Close)

	return httpServer, requestsCollector
}

// captureStdout returns what f prints to the standard output, where the console logger prints messages.
func captureStdout(t *testing.T, f func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	require.NoError(t, err)
	stdout := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = stdout }()

	f()

	require.NoError(t, w.Close())
	output, err := io.ReadAll(r)
	require.NoError(t, err)
	return string(output)
}

func TestLogdashTrack(t *testing.T) {
	t.Run("should send event to the server", func(t *testing.T) {
		// GIVEN
		httpServer, requestsCollector := newTestServer(t)

		// WHEN
		ld := logdash.New(
			logdash.WithHost(httpServer.URL),
			logdash.WithAPIKey("test-api-key"),
			logdash.WithVerbose(),
		)
		ld.Track("agent_run", map[string]any{"agent": "support", "steps": 4, "success": true}, logdash.WithUserID("42"))
		err := ld.Shutdown(context.Background())

		// THEN
		assert.NoError(t, err)

		require.Len(t, requestsCollector.requests, 1)
		r := requestsCollector.requests[0]
		assert.Equal(t, "application/json", r.request.Header.Get("content-type"))

		body := assertRequestAndBody(t, r, http.MethodPost, "/events", "test-api-key", nil, time.Time{})
		assert.Regexp(t, uuidV4, body["id"])
		delete(body, "id")
		// the API rejects unknown fields with HTTP 400, so send exactly these
		assert.Equal(t, map[string]any{
			"name":   "agent_run",
			"props":  map[string]any{"agent": "support", "steps": float64(4), "success": true},
			"userId": "42",
		}, body)
	})

	t.Run("should leave out props and user ID when not set", func(t *testing.T) {
		// GIVEN
		httpServer, requestsCollector := newTestServer(t)

		// WHEN
		ld := logdash.New(
			logdash.WithHost(httpServer.URL),
			logdash.WithAPIKey("test-api-key"),
		)
		ld.Track("agent_run", nil)
		err := ld.Shutdown(context.Background())

		// THEN
		assert.NoError(t, err)

		require.Len(t, requestsCollector.requests, 1)
		var body map[string]any
		require.NoError(t, json.Unmarshal(requestsCollector.requests[0].body, &body))
		assert.Regexp(t, uuidV4, body["id"])
		delete(body, "id")
		assert.Equal(t, map[string]any{"name": "agent_run"}, body)
	})

	t.Run("should drop event with invalid name or user ID with a warning", func(t *testing.T) {
		// GIVEN
		httpServer, requestsCollector := newTestServer(t)
		ld := logdash.New(
			logdash.WithHost(httpServer.URL),
			logdash.WithAPIKey("test-api-key"),
			logdash.WithVerbose(),
		)
		invalidNames := []string{
			"", "Agent_run", "agent run", "agent-run", "9lives", "agent_run\n", strings.Repeat("a", 65),
			"pageview", "pageleave", "browser_error",
		}

		// WHEN
		output := captureStdout(t, func() {
			for _, name := range invalidNames {
				ld.Track(name, nil)
			}
			// stands in for an email address, which the API rejects as a user ID
			ld.Track("agent_run", nil, logdash.WithUserID("user@42"))
		})
		// a valid event, to show the others were dropped and not lost on the way
		ld.Track("agent_run", nil)
		err := ld.Shutdown(context.Background())

		// THEN
		assert.NoError(t, err)
		assert.Equal(t, len(invalidNames)+1, strings.Count(output, "Dropping event"), output)
		assert.NotContains(t, output, "user@42")

		require.Len(t, requestsCollector.requests, 1)
		assertRequestAndBody(t, requestsCollector.requests[0], http.MethodPost, "/events", "test-api-key", map[string]any{"name": "agent_run"}, time.Time{})
	})

	t.Run("should send props as they were on Track, without the ones JSON cannot encode", func(t *testing.T) {
		// GIVEN
		httpServer, requestsCollector := newTestServer(t)
		ld := logdash.New(
			logdash.WithHost(httpServer.URL),
			logdash.WithAPIKey("test-api-key"),
		)

		// WHEN
		props := map[string]any{"agent": "support", "ratio": math.NaN()}
		ld.Track("agent_run", props)
		// the caller reuses its map while the event waits to be sent
		props["agent"] = "sales"
		err := ld.Shutdown(context.Background())

		// THEN
		assert.NoError(t, err)

		require.Len(t, requestsCollector.requests, 1)
		assertRequestAndBody(t, requestsCollector.requests[0], http.MethodPost, "/events", "test-api-key", map[string]any{
			"props": map[string]any{"agent": "support"},
		}, time.Time{})
	})

	t.Run("should send queued events on shutdown", func(t *testing.T) {
		// GIVEN
		requestsCollector := &requestsCollector{}

		httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer r.Body.Close()
			// a slow server, so the events are still queued when Shutdown starts
			time.Sleep(10 * time.Millisecond)
			requestsCollector.add(t, r)
			w.WriteHeader(http.StatusAccepted)
		}))

		defer httpServer.Close()

		// WHEN
		ld := logdash.New(
			logdash.WithHost(httpServer.URL),
			logdash.WithAPIKey("test-api-key"),
		)
		const events = 20
		for range events {
			ld.Track("agent_run", nil)
		}
		err := ld.Shutdown(context.Background())

		// THEN
		assert.NoError(t, err)

		ids := make(map[any]bool)
		for _, r := range requestsCollector.requests {
			body := assertRequestAndBody(t, r, http.MethodPost, "/events", "test-api-key", map[string]any{"name": "agent_run"}, time.Time{})
			ids[body["id"]] = true
		}
		assert.Len(t, ids, events, "every event is sent once, with its own id")
	})

	t.Run("should retry failed event with the same id", func(t *testing.T) {
		// GIVEN
		httpServer, requestsCollector := newTestServer(t, http.StatusServiceUnavailable, http.StatusAccepted)

		// WHEN
		ld := logdash.New(
			logdash.WithHost(httpServer.URL),
			logdash.WithAPIKey("test-api-key"),
			// the default number of retries, without waiting long between them
			logdash.WithHTTPRetryMin(time.Millisecond),
			logdash.WithHTTPRetryMax(time.Millisecond),
		)
		ld.Track("agent_run", nil)
		err := ld.Shutdown(context.Background())

		// THEN
		assert.NoError(t, err)

		require.Len(t, requestsCollector.requests, 2)
		assert.Equal(t, string(requestsCollector.requests[0].body), string(requestsCollector.requests[1].body))
	})

	t.Run("should not retry event over the hourly limit", func(t *testing.T) {
		// GIVEN
		httpServer, requestsCollector := newTestServer(t, http.StatusTooManyRequests)

		// WHEN
		ld := logdash.New(
			logdash.WithHost(httpServer.URL),
			logdash.WithAPIKey("test-api-key"),
			logdash.WithHTTPRetryMin(time.Millisecond),
			logdash.WithHTTPRetryMax(time.Millisecond),
		)
		ld.Track("agent_run", nil)
		err := ld.Shutdown(context.Background())

		// THEN
		assert.NoError(t, err)
		assert.Len(t, requestsCollector.requests, 1)
	})

	t.Run("should print event to the console only without API key", func(t *testing.T) {
		// GIVEN
		httpServer, requestsCollector := newTestServer(t)
		ld := logdash.New(logdash.WithHost(httpServer.URL))

		// WHEN
		output := captureStdout(t, func() {
			ld.Track("agent_run", map[string]any{"agent": "support"}, logdash.WithUserID("42"))
		})
		err := ld.Shutdown(context.Background())

		// THEN
		assert.NoError(t, err)
		assert.Contains(t, output, `Event {"name":"agent_run","props":{"agent":"support"},"userId":"42"}`)
		assert.Empty(t, requestsCollector.requests)
	})
}

func TestLogdashAfterShutdown(t *testing.T) {
	t.Run("should not block logs and events after the second shutdown", func(t *testing.T) {
		// GIVEN
		httpServer, requestsCollector := newTestServer(t)
		ld := logdash.New(
			logdash.WithHost(httpServer.URL),
			logdash.WithAPIKey("test-api-key"),
			logdash.WithOverflowPolicy(logdash.OverflowPolicyBlock),
		)
		assert.NoError(t, ld.Shutdown(context.Background()))
		assert.ErrorIs(t, ld.Shutdown(context.Background()), logdash.ErrAlreadyClosed)

		// WHEN
		done := make(chan struct{})
		go func() {
			defer close(done)
			ld.Logger.Info("Hello, World!")
			ld.Track("agent_run", nil)
		}()

		// THEN
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("logging after shutdown blocks")
		}
		assert.Empty(t, requestsCollector.requests)
	})
}

// sendSlogLogs logs with a slog.Logger on a [logdash.SlogTextHandler] and returns the log messages the server received.
func sendSlogLogs(t *testing.T, opts slog.HandlerOptions, log func(logger *slog.Logger)) []any {
	t.Helper()

	httpServer, requestsCollector := newTestServer(t)
	ld := logdash.New(
		logdash.WithHost(httpServer.URL),
		logdash.WithAPIKey("test-api-key"),
	)
	log(slog.New(logdash.NewSlogTextHandler(ld.Logger, opts)))
	assert.NoError(t, ld.Shutdown(context.Background()))

	var messages []any
	for _, r := range requestsCollector.requests {
		var body map[string]any
		assert.NoError(t, json.Unmarshal(r.body, &body))
		messages = append(messages, body["message"])
	}
	return messages
}

func TestSlogTextHandler(t *testing.T) {
	t.Run("should log group attrs as rewritten by ReplaceAttr", func(t *testing.T) {
		messages := sendSlogLogs(t, slog.HandlerOptions{
			ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
				if a.Key == "token" {
					a.Value = slog.StringValue("********")
				}
				return a
			},
		}, func(logger *slog.Logger) {
			logger.Info("login",
				slog.Group("auth", "user", "john", "token", "1234567890", slog.Group("empty")),
				slog.Group("", "inline", true),
				"status", 200,
			)
		})

		assert.Equal(t, []any{`"login" auth.user="john" auth.token="********" inline=true status=200`}, messages)
	})

	t.Run("should log info and above without a level", func(t *testing.T) {
		messages := sendSlogLogs(t, slog.HandlerOptions{}, func(logger *slog.Logger) {
			logger.Debug("debug")
			logger.Info("info")
		})

		assert.Equal(t, []any{`"info"`}, messages)
	})
}
