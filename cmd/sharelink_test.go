package cmd

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"crantcli/internal/nglstate"
	"crantcli/internal/statesrv"
)

// routeStateServers points stateServerResolver at a local TLS server that
// answers for every state server host, and gives credential i the token
// "fake-token-<i>" so no test reads a real credential store. It returns the
// Authorization header each host last received.
func routeStateServers(t *testing.T) func(host string) string {
	t.Helper()
	var mu sync.Mutex
	authorization := map[string]string{}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		authorization[r.Host] = r.Header.Get("Authorization")
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"layers":[{"type":"segmentation","name":"seg"}]}`)
	}))
	t.Cleanup(srv.Close)

	addr := srv.Listener.Addr().String()
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			var dialer net.Dialer
			return dialer.DialContext(ctx, network, addr)
		},
		// The test server answers for hosts its certificate does not name.
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	t.Cleanup(transport.CloseIdleConnections)

	previous := stateServerResolver
	resolver := &statesrv.Resolver{
		Client:      &http.Client{Transport: transport},
		Credentials: append([]statesrv.Credential(nil), previous.Credentials...),
	}
	for i := range resolver.Credentials {
		token := fmt.Sprintf("fake-token-%d", i)
		resolver.Credentials[i].Token = func() string { return token }
	}
	stateServerResolver = resolver
	t.Cleanup(func() { stateServerResolver = previous })

	return func(host string) string {
		mu.Lock()
		defer mu.Unlock()
		return authorization[host]
	}
}

func TestShareLinksLoadThroughStateServerResolver(t *testing.T) {
	authorizationFor := routeStateServers(t)

	const zettaLink = "https://spelunker.cave-explorer.org/#!middleauth+https://proofreading.zetta.ai/nglstate/api/v1/1"
	result, err := nglstate.LoadState(zettaLink, false)
	if err != nil {
		t.Fatalf("LoadState(%q): %v", zettaLink, err)
	}
	if result.Source != nglstate.SourceURL || result.Remote == nil {
		t.Fatalf("LoadState() = source %q, remote %+v; want a resolved Share link", result.Source, result.Remote)
	}
	if got := authorizationFor("proofreading.zetta.ai"); got != "Bearer fake-token-0" {
		t.Fatalf("proofreading.zetta.ai received Authorization %q, want the CAVE token", got)
	}

	// global.daf-apis.com trusts another auth server, so the CAVE token, which
	// proofreading.zetta.ai issued, must not go there.
	const globalLink = "https://spelunker.cave-explorer.org/#!middleauth+https://global.daf-apis.com/nglstate/api/v1/2"
	if _, err := nglstate.LoadState(globalLink, false); err != nil {
		t.Fatalf("LoadState(%q): %v", globalLink, err)
	}
	if got := authorizationFor("global.daf-apis.com"); got != "" {
		t.Fatalf("global.daf-apis.com received Authorization %q, want none", got)
	}
}
