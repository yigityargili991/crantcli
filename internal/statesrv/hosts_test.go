package statesrv

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// contactedRequest is a request a namedServer received, with the name the
// client verified the server's certificate against on that connection.
type contactedRequest struct {
	serverName, host, authorization string
}

// namedServer is a TLS server that every host name reaches, holding a
// certificate for whichever name the client asks for, signed by a CA its
// client trusts, as the server that name leads to would. Unlike stateServer,
// its client is the stock net/http transport with full certificate
// verification, so a test sees the names net/http really dials and verifies,
// after any IDNA conversion, rather than the names in the URLs.
type namedServer struct {
	client *http.Client

	mu       sync.Mutex
	dialed   []string
	requests []contactedRequest
}

func newNamedServer(t *testing.T, handler http.HandlerFunc) *namedServer {
	t.Helper()
	caKey := newTestKey(t)
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "statesrv test CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, caKey.Public(), caKey)
	if err != nil {
		t.Fatalf("creating the test CA: %v", err)
	}
	ca, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("parsing the test CA: %v", err)
	}
	leafKey := newTestKey(t)

	s := &namedServer{}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.requests = append(s.requests, contactedRequest{
			serverName:    r.TLS.ServerName,
			host:          r.Host,
			authorization: r.Header.Get("Authorization"),
		})
		s.mu.Unlock()
		handler(w, r)
	}))
	srv.TLS = &tls.Config{
		GetCertificate: func(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
			serial, err := rand.Int(rand.Reader, big.NewInt(1<<62))
			if err != nil {
				return nil, err
			}
			leaf := &x509.Certificate{
				SerialNumber: serial,
				Subject:      pkix.Name{CommonName: hello.ServerName},
				DNSNames:     []string{hello.ServerName},
				NotBefore:    time.Now().Add(-time.Hour),
				NotAfter:     time.Now().Add(time.Hour),
				KeyUsage:     x509.KeyUsageDigitalSignature,
				ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
			}
			der, err := x509.CreateCertificate(rand.Reader, leaf, ca, leafKey.Public(), caKey)
			if err != nil {
				return nil, err
			}
			return &tls.Certificate{Certificate: [][]byte{der}, PrivateKey: leafKey}, nil
		},
	}
	srv.StartTLS()
	t.Cleanup(srv.Close)

	pool := x509.NewCertPool()
	pool.AddCert(ca)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.TLSClientConfig = &tls.Config{RootCAs: pool}
	addr := srv.Listener.Addr().String()
	transport.DialContext = func(ctx context.Context, network, dialed string) (net.Conn, error) {
		s.mu.Lock()
		s.dialed = append(s.dialed, dialed)
		s.mu.Unlock()
		var dialer net.Dialer
		return dialer.DialContext(ctx, network, addr)
	}
	t.Cleanup(transport.CloseIdleConnections)
	s.client = &http.Client{Transport: transport}
	return s
}

func newTestKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generating a test key: %v", err)
	}
	return key
}

// recorded returns the addresses the client dialed and the requests the
// server received, in order.
func (s *namedServer) recorded() (dialed []string, requests []contactedRequest) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.dialed...), append([]contactedRequest(nil), s.requests...)
}

// globalCredential is crantcli's global CAVE token with a fake value.
func globalCredential() Credential {
	return Credential{
		Name:       "global CAVE token",
		AuthServer: "https://global.daf-apis.com",
		Token:      func() (string, string) { return "global-token", "CAVE_GLOBAL_TOKEN" },
		Setup:      "run 'crantcli setup' or set CAVE_GLOBAL_TOKEN",
	}
}

func TestResolveSendsTokenToItsHostOverVerifiedTLS(t *testing.T) {
	srv := newNamedServer(t, serveJSON(testState))
	resolver := &Resolver{Client: srv.client, Credentials: []Credential{globalCredential()}}

	if _, err := resolver.Resolve("middleauth+https://global.daf-apis.com/nglstate/api/v1/1"); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	dialed, requests := srv.recorded()
	want := contactedRequest{serverName: "global.daf-apis.com", host: "global.daf-apis.com", authorization: "Bearer global-token"}
	if len(dialed) != 1 || dialed[0] != "global.daf-apis.com:443" || len(requests) != 1 || requests[0] != want {
		t.Fatalf("dialed %q and received %+v; want global.daf-apis.com:443 and %+v", dialed, requests, want)
	}
}

func TestResolveSendsNoTokenToUnicodeLookalike(t *testing.T) {
	// "İ" (U+0130) lowercases to "i", so each host reads as global.daf-apis.com
	// once lowercased. net/http dials and verifies it under its IDNA form,
	// global.xn--daf-apis-u0e.com, a domain anyone can register and get a
	// certificate for.
	for _, host := range []string{
		"global.daf-apİs.com",
		"global.daf-ap%C4%B0s.com",
	} {
		t.Run(host, func(t *testing.T) {
			srv := newNamedServer(t, serveJSON(testState))
			resolver := &Resolver{Client: srv.client, Credentials: []Credential{globalCredential()}}

			_, err := resolver.Resolve("middleauth+https://" + host + "/nglstate/api/v1/1")
			dialed, requests := srv.recorded()
			for _, got := range requests {
				if got.authorization != "" {
					t.Errorf("the global CAVE token went to %q (dialed %q), not global.daf-apis.com: %+v", got.serverName, dialed, got)
				}
			}
			if len(dialed) != 0 {
				t.Errorf("dialed %q for a link whose host net/http cannot contact under the name it shows", dialed)
			}
			want := `host "global.daf-ap\u0130s.com" contains non-ASCII characters`
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Errorf("Resolve() error = %v, want it to contain %q", err, want)
			}
		})
	}
}

func TestResolveSendsTokenOnlyToItsExactHost(t *testing.T) {
	// A subdomain or another port of the auth host can be another service,
	// such as one a stale DNS record still points to, so it gets no token.
	tests := []struct {
		host, wantAuthorization string
	}{
		{"global.daf-apis.com:443", "Bearer global-token"},
		{"metrics.global.daf-apis.com", ""},
		{"global.daf-apis.com:8443", ""},
	}
	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			srv := newNamedServer(t, serveJSON(testState))
			resolver := &Resolver{Client: srv.client, Credentials: []Credential{globalCredential()}}

			if _, err := resolver.Resolve("middleauth+https://" + tt.host + "/nglstate/api/v1/1"); err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			dialed, requests := srv.recorded()
			if len(requests) != 1 || requests[0].authorization != tt.wantAuthorization {
				t.Fatalf("dialed %q and received %+v; want one request with Authorization %q", dialed, requests, tt.wantAuthorization)
			}
		})
	}
}
