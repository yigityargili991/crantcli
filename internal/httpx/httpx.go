// Package httpx provides a small shared HTTP layer for the CRANT API clients.
// It centralizes the HTTP client/timeout and converts non-2xx responses into a
// typed StatusError so callers can wrap them with context or inspect the body.
package httpx

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"time"
)

// DefaultClient is the shared HTTP client used for all CRANT API calls.
// http.Client is safe for concurrent use, so a single instance is reused.
var DefaultClient = &http.Client{Timeout: 30 * time.Second}

// StatusError describes a non-2xx HTTP response. It captures the status code
// and the (already drained) response body so callers can wrap it with context
// or inspect it via errors.As.
type StatusError struct {
	StatusCode int
	Body       []byte
}

// Error implements the error interface.
func (e *StatusError) Error() string {
	body := bytes.TrimSpace(e.Body)
	if len(body) == 0 {
		return fmt.Sprintf("HTTP %d", e.StatusCode)
	}
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, body)
}

// Do sends req using the given client, falling back to DefaultClient when nil.
//
// When the response status is outside the 2xx range, Do drains and closes the
// body and returns a *StatusError. On success the caller owns resp.Body and is
// responsible for closing it.
func Do(client *http.Client, req *http.Request) (*http.Response, error) {
	if client == nil {
		client = DefaultClient
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, &StatusError{StatusCode: resp.StatusCode, Body: body}
	}

	return resp, nil
}
