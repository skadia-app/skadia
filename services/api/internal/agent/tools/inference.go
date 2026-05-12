package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"
)

var embeddingHTTPClient = &http.Client{Timeout: 10 * time.Second}

func embedText(ctx context.Context, baseURL, text string) ([]float64, error) {
	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		return nil, fmt.Errorf("ML base URL is not configured")
	}
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(map[string]string{"text": text}); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/ml/embed/text", &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := embeddingHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("embedding service returned %s", resp.Status)
	}
	var out struct {
		Embedding []float64 `json:"embedding"`
		Dim       int       `json:"dim"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	if len(out.Embedding) == 0 {
		return nil, fmt.Errorf("embedding service returned an empty vector")
	}
	if out.Dim != 0 && out.Dim != len(out.Embedding) {
		return nil, fmt.Errorf("embedding dimension mismatch: dim=%d len=%d", out.Dim, len(out.Embedding))
	}
	return out.Embedding, nil
}

func pgVectorLiteral(values []float64) (string, error) {
	if len(values) == 0 {
		return "", fmt.Errorf("vector is empty")
	}
	parts := make([]string, 0, len(values))
	for _, v := range values {
		if math.IsInf(v, 0) || math.IsNaN(v) {
			return "", fmt.Errorf("vector contains non-finite value")
		}
		parts = append(parts, fmt.Sprintf("%g", v))
	}
	return "[" + strings.Join(parts, ",") + "]", nil
}
