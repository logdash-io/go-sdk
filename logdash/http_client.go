package logdash

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/hashicorp/go-retryablehttp"
)

// httpClient is a common HTTP client for sending data to the server.
type httpClient struct {
	client    *retryablehttp.Client
	serverURL string
	apiKey    string
}

type retryLogger struct {
	internalLogger *Logger
}

func (l *retryLogger) Printf(format string, v ...interface{}) {
	l.internalLogger.VerboseF(format, v...)
}

// newHTTPClient creates a new HTTP client instance.
func newHTTPClient(serverURL string, apiKey string, internalLogger *Logger) *httpClient {
	retryhttpClient := retryablehttp.NewClient()
	retryhttpClient.Logger = &retryLogger{
		internalLogger: internalLogger,
	}
	retryhttpClient.RetryMax = 3
	retryhttpClient.RetryWaitMin = 1 * time.Second
	retryhttpClient.RetryWaitMax = 30 * time.Second
	retryhttpClient.HTTPClient.Timeout = 5 * time.Second

	return &httpClient{
		client:    retryhttpClient,
		serverURL: serverURL,
		apiKey:    apiKey,
	}
}

// sendData sends data to the server at the specified endpoint.
func (c *httpClient) sendData(endpoint string, method string, data any) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal: %w", err)
	}

	req, err := retryablehttp.NewRequest(method, c.serverURL+endpoint, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("project-api-key", c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send: %w", err)
	}
	defer resp.Body.Close()

	// Allow reuse connection
	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return fmt.Errorf("server returned error status: %d, body: %s", resp.StatusCode, string(respBody))
	}

	return nil
}
