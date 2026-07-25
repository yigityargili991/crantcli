package seatable

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A malicious or compromised server could reflect the Authorization header
// into its error body. The combined auth error must never include response
// bodies, so the API token cannot leak into logs or terminal output.
func TestExchangeToken_ErrorNeverContainsToken(t *testing.T) {
	const token = "super-secret-api-token-12345"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		// Reflect the Authorization header, as a hostile server would.
		fmt.Fprintf(w, `{"message": "rejected %s"}`, r.Header.Get("Authorization"))
	}))
	defer srv.Close()

	_, err := exchangeToken(token, srv.URL)
	if err == nil {
		t.Fatal("expected auth to fail against reflecting server")
	}
	if strings.Contains(err.Error(), token) {
		t.Fatalf("error leaks API token: %v", err)
	}
}

func TestExchangeToken_SuccessFlow1(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer good-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		fmt.Fprint(w, `{"access_token": "at", "dtable_uuid": "uuid"}`)
	}))
	defer srv.Close()

	auth, err := exchangeToken("good-token", srv.URL)
	if err != nil {
		t.Fatalf("exchangeToken: %v", err)
	}
	if auth.AccessToken != "at" || auth.DTableUUID != "uuid" {
		t.Fatalf("unexpected auth response: %+v", auth)
	}
}
