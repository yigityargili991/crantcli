// Package httpx provides a small shared HTTP layer for the CRANT API clients.
// It centralizes the HTTP client/timeout and converts non-2xx responses into a
// typed StatusError so callers can wrap them with context or inspect the body.
package httpx

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"crantcli/internal/textout"
)

// DefaultClient is the shared HTTP client used for all CRANT API calls.
// http.Client is safe for concurrent use, so a single instance is reused.
var DefaultClient = &http.Client{Timeout: 30 * time.Second}

// MaxErrorBody bounds how much of a non-2xx response body is retained for the
// StatusError message; MaxResponseBody bounds success-response reads in the
// API clients. Both exist so a hostile or broken server cannot exhaust memory.
const (
	MaxErrorBody    = 1 << 20  // 1 MiB
	MaxResponseBody = 64 << 20 // 64 MiB
)

// StatusError describes a non-2xx HTTP response. It captures the status code
// and the (already drained) response body so callers can wrap it with context
// or inspect it via errors.As.
type StatusError struct {
	StatusCode int
	Body       []byte
	hideBody   bool
}

// Error implements the error interface. The body is server-controlled, so
// control characters are neutralized before it reaches logs or a terminal.
func (e *StatusError) Error() string {
	if e.hideBody {
		return fmt.Sprintf("HTTP %d", e.StatusCode)
	}
	body := bytes.TrimSpace(e.Body)
	if len(body) == 0 {
		return fmt.Sprintf("HTTP %d", e.StatusCode)
	}
	return fmt.Sprintf("HTTP %d: %s", e.StatusCode, textout.Sanitize(string(body)))
}

func requestHasCredentials(req *http.Request) bool {
	return req.Header.Get("Authorization") != "" ||
		req.Header.Get("Proxy-Authorization") != ""
}

func redactRequestCredentials(body []byte, req *http.Request) []byte {
	redacted := bytes.Clone(body)
	for _, header := range []string{"Authorization", "Proxy-Authorization"} {
		for _, value := range req.Header.Values(header) {
			if value == "" {
				continue
			}
			redacted = bytes.ReplaceAll(redacted, []byte(value), []byte("[REDACTED]"))
			if _, credential, ok := strings.Cut(value, " "); ok && credential != "" {
				redacted = bytes.ReplaceAll(redacted, []byte(credential), []byte("[REDACTED]"))
			}
		}
	}
	return redacted
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
		body, _ := io.ReadAll(io.LimitReader(resp.Body, MaxErrorBody))
		resp.Body.Close()
		return nil, &StatusError{
			StatusCode: resp.StatusCode,
			Body:       redactRequestCredentials(body, req),
			hideBody:   requestHasCredentials(req),
		}
	}

	return resp, nil
}
