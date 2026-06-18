package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

// httpGet performs a GET with optional headers and returns the body bytes.
func httpGet(ctx context.Context, url string, headers map[string]string) ([]byte, error) {
	return httpGetAccept(ctx, url, headers, "application/json", 0)
}

// httpGetAccept performs a GET with a caller-selected Accept header. When
// maxBytes is positive, responses larger than maxBytes return an error.
func httpGetAccept(ctx context.Context, url string, headers map[string]string, accept string, maxBytes int64) ([]byte, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			delay := time.Duration(attempt*2) * time.Second
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(delay):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, err
		}
		if accept != "" {
			req.Header.Set("Accept", accept)
		}
		req.Header.Set("User-Agent", "literature-mcp/1.0")
		for k, v := range headers {
			req.Header.Set(k, v)
		}

		body, err := httpDoLimited(req, maxBytes)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !isRetriableHTTPError(err) {
			break
		}
	}
	return nil, lastErr
}

// httpPostJSON performs a POST with a JSON body and optional headers.
func httpPostJSON(ctx context.Context, url string, body any, headers map[string]string) ([]byte, error) {
	buf, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "literature-mcp/1.0")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	return httpDo(req)
}

func httpDo(req *http.Request) ([]byte, error) {
	return httpDoLimited(req, 0)
}

func httpDoLimited(req *http.Request, maxBytes int64) ([]byte, error) {
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var r io.Reader = resp.Body
	if maxBytes > 0 {
		r = io.LimitReader(resp.Body, maxBytes+1)
	}
	body, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if maxBytes > 0 && int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("response exceeds %d bytes", maxBytes)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		preview := string(body)
		if len(preview) > 300 {
			preview = preview[:300] + "..."
		}
		return nil, httpStatusError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			Preview:    preview,
		}
	}
	return body, nil
}

type httpStatusError struct {
	StatusCode int
	Status     string
	Preview    string
}

func (e httpStatusError) Error() string {
	return fmt.Sprintf("http %d %s: %s", e.StatusCode, e.Status, e.Preview)
}

func isRetriableHTTPError(err error) bool {
	if err == nil {
		return false
	}
	if e, ok := err.(httpStatusError); ok {
		return e.StatusCode == http.StatusTooManyRequests || e.StatusCode >= 500
	}
	return false
}
