package cmd

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
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
		resolver.Credentials[i].Token = func() (string, string) { return token, "the test" }
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

	// global.daf-apis.com trusts its own auth server, so it gets the global
	// CAVE token and never the CAVE token proofreading.zetta.ai issued.
	const globalLink = "https://spelunker.cave-explorer.org/#!middleauth+https://global.daf-apis.com/nglstate/api/v1/2"
	if _, err := nglstate.LoadState(globalLink, false); err != nil {
		t.Fatalf("LoadState(%q): %v", globalLink, err)
	}
	if got := authorizationFor("global.daf-apis.com"); got != "Bearer fake-token-1" {
		t.Fatalf("global.daf-apis.com received Authorization %q, want the global CAVE token", got)
	}
	if got := authorizationFor("proofreading.zetta.ai"); got != "Bearer fake-token-0" {
		t.Fatalf("proofreading.zetta.ai received Authorization %q after the global fetch, want the CAVE token", got)
	}
}

func TestShareLinkTokensGoOnlyToTheirStateServers(t *testing.T) {
	// Each token goes only to the host whose state server accepts it, on the
	// default https port. data.proofreading.zetta.ai serves CAVE, not states,
	// and other subdomains and ports can be other services.
	authorizationFor := routeStateServers(t)
	for _, host := range []string{
		"data.proofreading.zetta.ai",
		"proofreading.zetta.ai:8443",
		"metrics.global.daf-apis.com",
		"global.daf-apis.com:8443",
	} {
		link := "https://spelunker.cave-explorer.org/#!middleauth+https://" + host + "/nglstate/api/v1/1"
		if _, err := nglstate.LoadState(link, false); err != nil {
			t.Fatalf("LoadState(%q): %v", link, err)
		}
		if got := authorizationFor(host); got != "" {
			t.Errorf("%s received Authorization %q, want no token", host, got)
		}
	}
}

func TestStateServerCredentialsCoverBothAuthServers(t *testing.T) {
	// The resolver's credentials mirror the realms the state servers name:
	// proofreading.zetta.ai for the CAVE token, global.daf-apis.com for the
	// global one, each with the places its token is looked up.
	want := map[string]string{
		"https://proofreading.zetta.ai": "CAVE_TOKEN",
		"https://global.daf-apis.com":   "CAVE_GLOBAL_TOKEN",
	}
	if len(stateServerResolver.Credentials) != len(want) {
		t.Fatalf("resolver has %d credentials, want %d", len(stateServerResolver.Credentials), len(want))
	}
	for _, cred := range stateServerResolver.Credentials {
		env, ok := want[cred.AuthServer]
		if !ok {
			t.Fatalf("unexpected auth server %q", cred.AuthServer)
		}
		if !slices.Contains(cred.Sources, env) || !strings.Contains(cred.Setup, env) {
			t.Errorf("%s credential: sources %q and setup %q should name %s", cred.AuthServer, cred.Sources, cred.Setup, env)
		}
	}
}
