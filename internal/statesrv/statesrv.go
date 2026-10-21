// Package statesrv fetches the Neuroglancer states that Share links point to.
// The viewer's Share button stores a state on a state server and copies a
// link that names where, such as
// #!middleauth+https://global.daf-apis.com/nglstate/api/v1/<id>; this package
// turns that location into the state JSON.
//
// A middle_auth state server wants a bearer token from the auth server it
// trusts. Each token is sent only to the host of the auth server that issued
// it and that host's subdomains, so a link cannot send it anywhere else.
package statesrv

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"unicode"

	"crantcli/internal/httpx"
)

// middleauthPrefix marks a location the viewer fetches with a middle_auth
// token. The URL after it is fetched like any other.
const middleauthPrefix = "middleauth+"

// maxStateBytes bounds a fetched state; legitimate scenes are a few MiB.
var maxStateBytes int64 = httpx.MaxResponseBody

// Credential is a bearer token and the middle_auth server that issued it.
type Credential struct {
	// Name says what the token is called, as in "CAVE token".
	Name string
	// AuthServer is the URL of the issuing server, such as
	// "https://proofreading.zetta.ai". The token goes to its host and that
	// host's subdomains, which the same operator runs, and nowhere else.
	AuthServer string
	// Token returns the token, or "" when none is configured. It is called
	// only to fetch a state from a matching host, so a credential store is
	// not read for a state that does not need it.
	Token func() string
	// Setup says how to configure the token, as in "run 'crantcli setup' or
	// set CAVE_TOKEN".
	Setup string
}

// matches reports whether host is the auth server's host or a subdomain of
// it. An auth server that is not https matches nothing, so a token never
// travels unencrypted.
func (c *Credential) matches(host string) bool {
	auth, err := url.Parse(c.AuthServer)
	if err != nil || auth.Scheme != "https" || auth.Hostname() == "" {
		return false
	}
	authHost := strings.ToLower(auth.Hostname())
	host = strings.ToLower(host)
	return host == authHost || strings.HasSuffix(host, "."+authHost)
}

// token returns the configured token, or an error that says how to
// configure it.
func (c *Credential) token() (string, error) {
	var token string
	if c.Token != nil {
		token = strings.TrimSpace(c.Token())
	}
	if token == "" {
		return "", fmt.Errorf("this state server needs the %s, which is not configured; %s", c.Name, c.Setup)
	}
	if strings.ContainsFunc(token, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
		return "", fmt.Errorf("the %s contains embedded whitespace or control characters; %s, and paste only the token on one line", c.Name, c.Setup)
	}
	return token, nil
}

// Resolver fetches states from state servers.
type Resolver struct {
	// Client sends the requests; nil means httpx.DefaultClient. Either way
	// Resolve applies its own redirect policy.
	Client *http.Client
	// Credentials are the tokens Resolve may attach, each only for the hosts
	// of the auth server that issued it.
	Credentials []Credential
}

// Resolve returns the state JSON stored at location, the fragment of a Share
// link after "#!". A location whose scheme is not https or middleauth+https
// returns an error wrapping errors.ErrUnsupported.
func (r *Resolver) Resolve(location string) ([]byte, error) {
	target, err := stateURL(location)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("creating state request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	// middle_auth redirects a request without a token to its sign-in page
	// unless the request says it is programmatic, as the viewer's are. Then
	// it answers 401 and names the auth server it trusts.
	req.Header.Set("X-Requested-With", "XMLHttpRequest")

	cred := r.credentialFor(target.Hostname())
	if cred != nil {
		token, err := cred.token()
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := httpx.Do(r.client(), req)
	if err != nil {
		return nil, describeFailure(err, cred)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxStateBytes+1))
	if err != nil {
		return nil, fmt.Errorf("reading the state: %w", err)
	}
	if int64(len(data)) > maxStateBytes {
		return nil, fmt.Errorf("the state exceeds %d bytes", maxStateBytes)
	}
	if !json.Valid(data) {
		return nil, fmt.Errorf("the state server's response is not JSON (Content-Type %q)", resp.Header.Get("Content-Type"))
	}
	return data, nil
}

// stateURL returns the URL a location names. The viewer can also read states
// from other kinds of storage, such as gs://, which crantcli does not.
func stateURL(location string) (*url.URL, error) {
	raw := strings.TrimPrefix(location, middleauthPrefix)
	if !strings.HasPrefix(raw, "https://") {
		return nil, fmt.Errorf("%w: crantcli fetches stored states only over https, not from %q", errors.ErrUnsupported, location)
	}
	target, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("parsing the state location: %w", err)
	}
	if target.Hostname() == "" {
		return nil, fmt.Errorf("the state location %q names no host", location)
	}
	return target, nil
}

func (r *Resolver) credentialFor(host string) *Credential {
	for i := range r.Credentials {
		if r.Credentials[i].matches(host) {
			return &r.Credentials[i]
		}
	}
	return nil
}

func (r *Resolver) client() *http.Client {
	base := r.Client
	if base == nil {
		base = httpx.DefaultClient
	}
	client := *base
	client.CheckRedirect = checkRedirect
	return &client
}

// checkRedirect follows a redirect only for a request without a token and
// only to https, as a browser on an https page would. A request with a token
// stops at the redirect, which Resolve reports: a state server that accepts
// the token answers with the state, and Go would forward the token to the
// target's subdomains.
func checkRedirect(req *http.Request, via []*http.Request) error {
	if via[0].Header.Get("Authorization") != "" {
		return http.ErrUseLastResponse
	}
	if req.URL.Scheme != "https" {
		return fmt.Errorf("refusing a redirect to %q, which is not https", req.URL.Redacted())
	}
	if len(via) >= 10 {
		return errors.New("stopped after 10 redirects")
	}
	return nil
}

// describeFailure says why the state server did not return the state. The
// server's words reach the user only through httpx.StatusError, which
// neutralizes control characters, or quoted.
func describeFailure(err error, cred *Credential) error {
	var statusErr *httpx.StatusError
	if !errors.As(err, &statusErr) {
		return fmt.Errorf("fetching the state: %w", err)
	}
	code := statusErr.StatusCode
	switch {
	case (code == http.StatusUnauthorized || code == http.StatusForbidden) && cred != nil:
		return fmt.Errorf("the state server rejected the %s (HTTP %d); %s to replace it", cred.Name, code, cred.Setup)
	case code == http.StatusUnauthorized || code == http.StatusForbidden:
		if realm := bearerRealm(statusErr.Header.Get("WWW-Authenticate")); realm != "" {
			return fmt.Errorf("the state server requires signing in with %q, and crantcli has no token for it (HTTP %d)", realm, code)
		}
		return fmt.Errorf("the state server requires signing in, and crantcli has no token for it (HTTP %d)", code)
	case code == http.StatusNotFound:
		return errors.New("the state server has no such state (HTTP 404); the link may be incomplete, or the state deleted")
	case code >= 300 && code < 400:
		return fmt.Errorf("the state server answered with a redirect (HTTP %d) instead of the state", code)
	default:
		return fmt.Errorf("the state server answered %w", err)
	}
}

// bearerRealm returns the realm of a Bearer challenge. middle_auth sets it to
// the authorize URL of the auth server it trusts.
func bearerRealm(challenge string) string {
	scheme, params, ok := strings.Cut(strings.TrimSpace(challenge), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	for _, param := range strings.Split(params, ",") {
		name, value, ok := strings.Cut(strings.TrimSpace(param), "=")
		if ok && strings.EqualFold(name, "realm") {
			return strings.Trim(value, `"`)
		}
	}
	return ""
}
