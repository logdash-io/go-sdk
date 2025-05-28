package logdash_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/logdash-io/go-sdk/logdash"
	"github.com/stretchr/testify/assert"
)

func TestLogdashLogger(t *testing.T) {
	t.Run("should send info log to the server", func(t *testing.T) {
		requests := make([]*http.Request, 0)
		bodies := make([][]byte, 0)
		httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests = append(requests, r)
			body, err := io.ReadAll(r.Body)
			assert.NoError(t, err)
			defer r.Body.Close()
			bodies = append(bodies, body)
			w.WriteHeader(http.StatusOK)
		}))

		defer httpServer.Close()

		ld := logdash.New(logdash.LogdashConfig{
			Host:    httpServer.URL,
			APIKey:  "test-api-key",
			Verbose: true,
		})
		ld.Logger.Info("Hello, World!")
		err := ld.Shutdown(context.Background())
		assert.NoError(t, err)

		assert.Equal(t, 1, len(requests))
		r := requests[0]
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/logs", r.URL.Path)
		assert.Equal(t, "test-api-key", r.Header.Get("project-api-key"))
		body := bodies[0]
		var logEntry map[string]any
		err = json.Unmarshal(body, &logEntry)
		assert.NoError(t, err)
		assert.Equal(t, "info", logEntry["level"])
		assert.Equal(t, "Hello, World!", logEntry["message"])
		assert.Contains(t, logEntry, "createdAt")
		assert.Contains(t, logEntry, "sequenceNumber")

		timestamp, err := time.Parse(time.RFC3339Nano, logEntry["createdAt"].(string))
		assert.NoError(t, err)
		assert.True(t, timestamp.Before(time.Now()))
	})
}
