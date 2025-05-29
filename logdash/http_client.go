package logdash

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/hashicorp/go-retryablehttp"
)

// httpClient is a common HTTP client for sending data to the server.
type httpClient struct {
	client    *retryablehttp.Client
	serverURL string
	apiKey    string
}

// newHTTPClient creates a new HTTP client instance.
func newHTTPClient(serverURL string, apiKey string) *httpClient {
	retryableClient := retryablehttp.NewClient()
	retryableClient.HTTPClient = &http.Client{
		Timeout: 10 * time.Second,
	}
	retryableClient.RetryMax = 3
	retryableClient.RetryWaitMin = 1 * time.Second
	retryableClient.RetryWaitMax = 30 * time.Second
	retryableClient.Backoff = retryablehttp.DefaultBackoff

	return &httpClient{
		client:    retryableClient,
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

	req, err := retryablehttp.NewRequest(method, c.serverURL+endpoint, jsonData)
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
	_, _ = io.ReadAll(resp.Body)

	if resp.StatusCode >= 400 {
		return fmt.Errorf("server returned error status: %d", resp.StatusCode)
	}

	return nil
}
