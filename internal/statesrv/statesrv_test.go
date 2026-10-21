package statesrv

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

const testState = `{"layers":[{"type":"segmentation","name":"seg"}]}`

// recordedRequest is what the test state server saw of one request.
type recordedRequest struct {
	host, path, authorization, requestedWith string
}

// stateServer is a TLS test server that any host name reaches, so tests can
// tell a matching host from another. Its certificate covers example.com and
// 127.0.0.1.
type stateServer struct {
	client *http.Client

	mu       sync.Mutex
	requests []recordedRequest
}

func newStateServer(t *testing.T, handler http.HandlerFunc) *stateServer {
	t.Helper()
	s := &stateServer{}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.requests = append(s.requests, recordedRequest{
			host:          r.Host,
			path:          r.URL.Path,
			authorization: r.Header.Get("Authorization"),
			requestedWith: r.Header.Get("X-Requested-With"),
		})
		s.mu.Unlock()
		handler(w, r)
	}))
	t.Cleanup(srv.Close)

	transport := srv.Client().Transport.(*http.Transport).Clone()
	addr := srv.Listener.Addr().String()
	transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		var dialer net.Dialer
		return dialer.DialContext(ctx, network, addr)
	}
	s.client = &http.Client{Transport: transport}
	return s
}

func (s *stateServer) recorded() []recordedRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]recordedRequest(nil), s.requests...)
}

// resolver returns a Resolver that sends the token "secret-token", found in
// CAVE_TOKEN, to example.com and its subdomains, and counts how often it
// reads the token.
func (s *stateServer) resolver(tokenReads *int) *Resolver {
	return &Resolver{
		Client: s.client,
		Credentials: []Credential{{
			Name:       "CAVE token",
			AuthServer: "https://example.com",
			Token: func() (string, string) {
				if tokenReads != nil {
					*tokenReads++
				}
				return "secret-token", "CAVE_TOKEN"
			},
			Sources: []string{"crantcli's credential store", "CAVE_TOKEN", "CAVE_TOKEN_FILE"},
			Setup:   "run 'crantcli setup' or set CAVE_TOKEN",
		}},
	}
}

func serveJSON(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}
}

func TestResolveReturnsState(t *testing.T) {
	srv := newStateServer(t, serveJSON(testState))

	data, err := srv.resolver(nil).Resolve("middleauth+https://example.com/nglstate/api/v1/5195345298243584")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if string(data) != testState {
		t.Fatalf("Resolve() = %q, want %q", data, testState)
	}
	requests := srv.recorded()
	if len(requests) != 1 {
		t.Fatalf("server saw %d requests, want 1", len(requests))
	}
	got := requests[0]
	if got.path != "/nglstate/api/v1/5195345298243584" || got.authorization != "Bearer secret-token" || got.requestedWith != "XMLHttpRequest" {
		t.Fatalf("request = %+v, want the state path with the token and X-Requested-With", got)
	}
}

func TestResolveSendsTokenOnlyToMatchingHost(t *testing.T) {
	srv := newStateServer(t, serveJSON(testState))
	tokenReads := 0
	resolver := srv.resolver(&tokenReads)

	// 127.0.0.1 is not under example.com, so the token must stay home and
	// is not even read.
	if _, err := resolver.Resolve("middleauth+https://127.0.0.1/nglstate/api/v1/1"); err != nil {
		t.Fatalf("Resolve(other host): %v", err)
	}
	if tokenReads != 0 {
		t.Fatalf("token read %d times for a host it does not belong to", tokenReads)
	}
	if _, err := resolver.Resolve("https://example.com/nglstate/api/v1/2"); err != nil {
		t.Fatalf("Resolve(matching host): %v", err)
	}

	requests := srv.recorded()
	if len(requests) != 2 {
		t.Fatalf("server saw %d requests, want 2", len(requests))
	}
	if requests[0].authorization != "" {
		t.Fatalf("request to %s carried Authorization %q", requests[0].host, requests[0].authorization)
	}
	if requests[1].authorization != "Bearer secret-token" {
		t.Fatalf("request to %s carried Authorization %q, want the token", requests[1].host, requests[1].authorization)
	}
}

func TestResolveSendsEachTokenOnlyToItsAuthServer(t *testing.T) {
	// Two auth servers, as crantcli has: each token must reach only its own
	// issuer's host, however the credentials are ordered.
	srv := newStateServer(t, serveJSON(testState))
	resolver := &Resolver{
		Client: srv.client,
		Credentials: []Credential{
			{Name: "first token", AuthServer: "https://example.com", Token: func() (string, string) { return "first-token", "FIRST" }},
			{Name: "second token", AuthServer: "https://127.0.0.1", Token: func() (string, string) { return "second-token", "SECOND" }},
		},
	}

	for _, location := range []string{
		"middleauth+https://127.0.0.1/nglstate/api/v1/1",
		"middleauth+https://example.com/nglstate/api/v1/2",
	} {
		if _, err := resolver.Resolve(location); err != nil {
			t.Fatalf("Resolve(%q): %v", location, err)
		}
	}

	want := map[string]string{"127.0.0.1": "Bearer second-token", "example.com": "Bearer first-token"}
	for _, got := range srv.recorded() {
		if got.authorization != want[got.host] {
			t.Errorf("request to %s carried Authorization %q, want %q", got.host, got.authorization, want[got.host])
		}
	}
}

func TestCredentialMatches(t *testing.T) {
	cred := Credential{AuthServer: "https://proofreading.zetta.ai"}
	tests := []struct {
		host string
		want bool
	}{
		{"proofreading.zetta.ai", true},
		{"data.proofreading.zetta.ai", true},
		{"Data.Proofreading.Zetta.AI", true},
		{"zetta.ai", false},
		{"global.daf-apis.com", false},
		{"evilproofreading.zetta.ai", false},
		{"proofreading.zetta.ai.example.org", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := cred.matches(tt.host); got != tt.want {
			t.Errorf("matches(%q) = %v, want %v", tt.host, got, tt.want)
		}
	}

	insecure := Credential{AuthServer: "http://proofreading.zetta.ai"}
	if insecure.matches("proofreading.zetta.ai") {
		t.Error("an http auth server matched; its token would travel unencrypted")
	}
}

func TestResolveWithoutConfiguredToken(t *testing.T) {
	srv := newStateServer(t, serveJSON(testState))
	resolver := srv.resolver(nil)
	resolver.Credentials[0].Token = func() (string, string) { return " \n", "" }

	_, err := resolver.Resolve("middleauth+https://example.com/nglstate/api/v1/1")
	want := "this state server needs the CAVE token, which is not configured (checked crantcli's credential store, CAVE_TOKEN, and CAVE_TOKEN_FILE); run 'crantcli setup' or set CAVE_TOKEN"
	if err == nil || err.Error() != want {
		t.Fatalf("Resolve() error = %v, want %q", err, want)
	}
	if n := len(srv.recorded()); n != 0 {
		t.Fatalf("server saw %d requests without a token to send", n)
	}
}

func TestResolveRejectsMalformedToken(t *testing.T) {
	srv := newStateServer(t, serveJSON(testState))
	resolver := srv.resolver(nil)
	resolver.Credentials[0].Token = func() (string, string) { return "secret\ntoken", "CAVE_TOKEN" }

	_, err := resolver.Resolve("https://example.com/nglstate/api/v1/1")
	if err == nil || !strings.Contains(err.Error(), "the CAVE token from CAVE_TOKEN contains embedded whitespace or control characters") {
		t.Fatalf("Resolve() error = %v, want a malformed-token error naming its source", err)
	}
	if strings.Contains(err.Error(), "secret") {
		t.Fatalf("error leaks the token: %v", err)
	}
}

func TestResolveUnauthorized(t *testing.T) {
	srv := newStateServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="https://auth.example.org/sticky_auth/api/v1/authorize"`)
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, "token "+r.Header.Get("Authorization")+" is invalid")
	})
	resolver := srv.resolver(nil)

	t.Run("token rejected", func(t *testing.T) {
		_, err := resolver.Resolve("middleauth+https://example.com/nglstate/api/v1/1")
		if err == nil || !strings.Contains(err.Error(), "rejected the CAVE token from CAVE_TOKEN (HTTP 401); run 'crantcli setup' or set CAVE_TOKEN to replace it") {
			t.Fatalf("Resolve() error = %v, want a rejected-token error naming its source", err)
		}
		if strings.Contains(err.Error(), "secret-token") {
			t.Fatalf("error leaks the token: %v", err)
		}
	})

	t.Run("no token for the host", func(t *testing.T) {
		_, err := resolver.Resolve("middleauth+https://127.0.0.1/nglstate/api/v1/1")
		want := `requires signing in with "https://auth.example.org/sticky_auth/api/v1/authorize", and crantcli has no token for it (HTTP 401)`
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("Resolve() error = %v, want it to contain %q", err, want)
		}
	})
}

func TestResolveNotFound(t *testing.T) {
	srv := newStateServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
	})

	_, err := srv.resolver(nil).Resolve("middleauth+https://example.com/nglstate/api/v1/1")
	if err == nil || !strings.Contains(err.Error(), "no such state (HTTP 404)") {
		t.Fatalf("Resolve() error = %v, want a not-found error", err)
	}
}

func TestResolveServerErrorIsSanitized(t *testing.T) {
	srv := newStateServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, "upstream \x1b]52;c;ZXZpbA==\x07down")
	})

	_, err := srv.resolver(nil).Resolve("https://127.0.0.1/nglstate/api/v1/1")
	if err == nil || !strings.Contains(err.Error(), "HTTP 502: upstream") {
		t.Fatalf("Resolve() error = %v, want the server's status and words", err)
	}
	if strings.ContainsAny(err.Error(), "\x1b\x07") {
		t.Fatalf("error passes server control characters through: %q", err)
	}
}

func TestResolveOversizedState(t *testing.T) {
	previous := maxStateBytes
	maxStateBytes = int64(len(testState))
	t.Cleanup(func() { maxStateBytes = previous })

	srv := newStateServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/at-limit" {
			_, _ = io.WriteString(w, testState)
			return
		}
		_, _ = io.WriteString(w, testState+" ")
	})
	resolver := srv.resolver(nil)

	if _, err := resolver.Resolve("https://example.com/at-limit"); err != nil {
		t.Fatalf("Resolve(state at the limit): %v", err)
	}
	_, err := resolver.Resolve("https://example.com/over-limit")
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("Resolve() error = %v, want a size error", err)
	}
}

func TestResolveNonJSON(t *testing.T) {
	// A sign-in page is the likeliest answer that is not a state.
	srv := newStateServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, "<!doctype html><title>Sign in</title>")
	})

	_, err := srv.resolver(nil).Resolve("middleauth+https://example.com/nglstate/api/v1/1")
	want := `response is not JSON (Content-Type "text/html; charset=utf-8")`
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("Resolve() error = %v, want it to contain %q", err, want)
	}
}

func TestResolveUnsupportedLocations(t *testing.T) {
	srv := newStateServer(t, serveJSON(testState))
	resolver := srv.resolver(nil)

	for _, location := range []string{
		"gs://bucket/state.json",
		"s3://bucket/state.json",
		"http://example.com/state.json",
		"middleauth+http://example.com/nglstate/api/v1/1",
		"middleauth+gs://bucket/%zz",
	} {
		t.Run(location, func(t *testing.T) {
			_, err := resolver.Resolve(location)
			if !errors.Is(err, errors.ErrUnsupported) {
				t.Fatalf("Resolve() error = %v, want errors.ErrUnsupported", err)
			}
		})
	}
	if n := len(srv.recorded()); n != 0 {
		t.Fatalf("server saw %d requests for unsupported locations", n)
	}
}

func TestResolveRedirects(t *testing.T) {
	srv := newStateServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/moved":
			http.Redirect(w, r, "https://127.0.0.1/state.json", http.StatusFound)
		case "/insecure":
			http.Redirect(w, r, "http://127.0.0.1/state.json", http.StatusFound)
		case "/state.json":
			_, _ = io.WriteString(w, testState)
		default:
			http.NotFound(w, r)
		}
	})
	resolver := srv.resolver(nil)

	t.Run("followed without a token", func(t *testing.T) {
		data, err := resolver.Resolve("https://127.0.0.1/moved")
		if err != nil || string(data) != testState {
			t.Fatalf("Resolve() = %q, %v; want the state behind the redirect", data, err)
		}
	})

	t.Run("not followed with a token", func(t *testing.T) {
		before := len(srv.recorded())
		_, err := resolver.Resolve("https://example.com/moved")
		if err == nil || !strings.Contains(err.Error(), "redirect (HTTP 302)") {
			t.Fatalf("Resolve() error = %v, want a redirect error", err)
		}
		if n := len(srv.recorded()) - before; n != 1 {
			t.Fatalf("server saw %d requests, want 1 (the redirect not followed)", n)
		}
	})

	t.Run("not followed away from https", func(t *testing.T) {
		_, err := resolver.Resolve("https://127.0.0.1/insecure")
		if err == nil || !strings.Contains(err.Error(), "which is not https") {
			t.Fatalf("Resolve() error = %v, want a refused-redirect error", err)
		}
	})
}

func TestJoinList(t *testing.T) {
	tests := []struct {
		items []string
		want  string
	}{
		{nil, ""},
		{[]string{"a"}, "a"},
		{[]string{"a", "b"}, "a and b"},
		{[]string{"a", "b", "c"}, "a, b, and c"},
	}
	for _, tt := range tests {
		if got := joinList(tt.items); got != tt.want {
			t.Errorf("joinList(%q) = %q, want %q", tt.items, got, tt.want)
		}
	}
}

func TestBearerRealm(t *testing.T) {
	tests := []struct{ challenge, want string }{
		{`Bearer realm="https://global.daf-apis.com/sticky_auth/api/v1/authorize"`, "https://global.daf-apis.com/sticky_auth/api/v1/authorize"},
		{`bearer error="invalid_token", realm="https://auth.example.org"`, "https://auth.example.org"},
		{`Basic realm="files"`, ""},
		{"", ""},
	}
	for _, tt := range tests {
		if got := bearerRealm(tt.challenge); got != tt.want {
			t.Errorf("bearerRealm(%q) = %q, want %q", tt.challenge, got, tt.want)
		}
	}
}
