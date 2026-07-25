package httpx

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDo_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, "ok")
	}))
	t.Cleanup(srv.Close)

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	resp, err := Do(srv.Client(), req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "ok" {
		t.Errorf("body = %q, want %q", body, "ok")
	}
}

func TestDo_NilClientUsesDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	resp, err := Do(nil, req)
	if err != nil {
		t.Fatalf("Do(nil client): %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusNoContent)
	}
}

func TestDo_StatusError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, "upstream failure")
	}))
	t.Cleanup(srv.Close)

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	resp, err := Do(srv.Client(), req)
	if err == nil {
		resp.Body.Close()
		t.Fatal("expected error for HTTP 502")
	}

	var se *StatusError
	if !errors.As(err, &se) {
		t.Fatalf("error = %T, want *StatusError", err)
	}
	if se.StatusCode != http.StatusBadGateway {
		t.Errorf("StatusCode = %d, want %d", se.StatusCode, http.StatusBadGateway)
	}
	if string(se.Body) != "upstream failure" {
		t.Errorf("Body = %q, want %q", se.Body, "upstream failure")
	}
}

func TestStatusError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  *StatusError
		want string
	}{
		{"with body", &StatusError{StatusCode: 404, Body: []byte("not found")}, "HTTP 404: not found"},
		{"trims body", &StatusError{StatusCode: 500, Body: []byte("  boom\n")}, "HTTP 500: boom"},
		{"empty body", &StatusError{StatusCode: 503, Body: nil}, "HTTP 503"},
		{"sanitizes escapes", &StatusError{StatusCode: 500, Body: []byte("bad\x1b[31m\x1b]52;c;eVil\x07")}, "HTTP 500: bad�[31m�]52;c;eVil�"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDo_StatusErrorBodyIsCapped(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write(make([]byte, MaxErrorBody+1024))
	}))
	t.Cleanup(srv.Close)

	req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	_, err = Do(srv.Client(), req)
	var se *StatusError
	if !errors.As(err, &se) {
		t.Fatalf("error = %T, want *StatusError", err)
	}
	if len(se.Body) != MaxErrorBody {
		t.Errorf("Body length = %d, want capped at %d", len(se.Body), MaxErrorBody)
	}
}
