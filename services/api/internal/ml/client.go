package ml

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"log/slog"
)

// Client communicates with the ML Service.
type Client struct {
	baseURL    string
	httpClient *http.Client

	mu            sync.Mutex
	failCount     int
	lastFail      time.Time
	circuitOpen   bool
	circuitOpenAt time.Time
}

const (
	maxFailures    = 3
	failWindow     = 60 * time.Second
	circuitTimeout = 5 * time.Minute
	requestTimeout = 30 * time.Second
)

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: requestTimeout,
		},
	}
}

// Available returns true if the ML service circuit breaker is closed.
func (c *Client) Available() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.circuitOpen {
		return true
	}
	if time.Since(c.circuitOpenAt) > circuitTimeout {
		c.circuitOpen = false
		c.failCount = 0
		slog.Info("ml circuit breaker reset (half-open)")
		return true
	}
	return false
}

func (c *Client) recordSuccess() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.failCount = 0
	c.circuitOpen = false
}

func (c *Client) recordFailure() {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	if time.Since(c.lastFail) > failWindow {
		c.failCount = 1
	} else {
		c.failCount++
	}
	c.lastFail = now
	if c.failCount >= maxFailures {
		c.circuitOpen = true
		c.circuitOpenAt = now
		slog.Warn("ml circuit breaker opened", "failures", c.failCount)
	}
}

// Post sends a JSON request to the ML service.
func (c *Client) Post(ctx context.Context, path string, body any) (json.RawMessage, error) {
	if !c.Available() {
		return nil, fmt.Errorf("ml service unavailable (circuit open)")
	}

	var reqBody io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		reqBody = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, reqBody)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.recordFailure()
		return nil, fmt.Errorf("ml service request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		c.recordFailure()
		return nil, fmt.Errorf("read ml response: %w", err)
	}

	if resp.StatusCode >= 500 {
		c.recordFailure()
		return nil, fmt.Errorf("ml service error %d: %s", resp.StatusCode, string(respBody))
	}

	c.recordSuccess()
	return json.RawMessage(respBody), nil
}

// Health checks if the ML service is reachable.
func (c *Client) Health(ctx context.Context) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/health", nil)
	if err != nil {
		return false, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.recordFailure()
		return false, err
	}
	defer resp.Body.Close()
	c.recordSuccess()
	return resp.StatusCode == 200, nil
}
