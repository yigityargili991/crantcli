// Package statesrv fetches the Neuroglancer states that Share links point to.
// The viewer's Share button stores a state on a state server and copies a
// link that names where, such as
// #!middleauth+https://global.daf-apis.com/nglstate/api/v1/<id>; this package
// turns that location into the state JSON.
//
// A middle_auth state server wants a bearer token from the auth server it
// trusts. Each token is sent only to the host of the auth server that issued
// it, on the default https port, so a link cannot send it anywhere else.
package statesrv

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strconv"
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
	// "https://proofreading.zetta.ai", whose host also serves the state
	// server that accepts the token. The token goes to that host on the
	// default https port and nowhere else: not to its subdomains or other
	// ports, which can be other services.
	AuthServer string
	// Token returns the token and the source that supplied it, such as
	// "CAVE_TOKEN", or two empty strings when none is configured. It is
	// called only to fetch a state from a matching host, so a credential
	// store is not read for a state that does not need it.
	Token func() (token, source string)
	// Sources names every place Token looks, in order, for the error when
	// it finds no token.
	Sources []string
	// Setup says how to configure the token, as in "run 'crantcli setup' or
	// set CAVE_TOKEN".
	Setup string
}

// matches reports whether target is an https URL on the auth server's host
// and the default https port: subdomains and other ports can be other
// services. An auth server that is not https on the default port matches
// nothing, so a token never travels unencrypted or to another port. A host
// that is not ASCII matches nothing either: net/http contacts it under its
// IDNA form, not the name lowercasing gives.
func (c *Credential) matches(target *url.URL) bool {
	auth, err := url.Parse(c.AuthServer)
	if err != nil || auth.Scheme != "https" || auth.Hostname() == "" || !onDefaultPort(auth) {
		return false
	}
	// strings.ToLower folds "global.daf-apİs.com" (U+0130) onto
	// global.daf-apis.com, but net/http dials and verifies that host as
	// global.xn--daf-apis-u0e.com, a domain anyone can register. An ASCII host
	// is contacted under the name it is matched by.
	host := target.Hostname()
	if !isASCII(host) || !isASCII(auth.Hostname()) {
		return false
	}
	return target.Scheme == "https" && onDefaultPort(target) &&
		strings.ToLower(host) == strings.ToLower(auth.Hostname())
}

// onDefaultPort reports whether an https URL names no port or 443.
func onDefaultPort(u *url.URL) bool {
	return u.Port() == "" || u.Port() == "443"
}

// token returns the configured token and its source, or an error that says
// where Token looked and how to configure a token.
func (c *Credential) token() (token, source string, err error) {
	if c.Token != nil {
		token, source = c.Token()
		token = strings.TrimSpace(token)
	}
	if token == "" {
		checked := ""
		if len(c.Sources) > 0 {
			checked = " (checked " + joinList(c.Sources) + ")"
		}
		return "", "", fmt.Errorf("this state server needs the %s, which is not configured%s; %s", c.Name, checked, c.Setup)
	}
	if strings.ContainsFunc(token, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) {
		return "", "", fmt.Errorf("the %s contains embedded whitespace or control characters; %s, and paste only the token on one line", c.describe(source), c.Setup)
	}
	return token, source, nil
}

// describe names the token and, when known, where it came from, as in
// "global CAVE token from CAVE_GLOBAL_TOKEN". It never includes the token.
func (c *Credential) describe(source string) string {
	if source == "" {
		return c.Name
	}
	return c.Name + " from " + source
}

// joinList joins items as an English list: "a, b, and c".
func joinList(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	case 2:
		return items[0] + " and " + items[1]
	}
	return strings.Join(items[:len(items)-1], ", ") + ", and " + items[len(items)-1]
}

// Resolver fetches states from state servers.
type Resolver struct {
	// Client sends the requests; nil means httpx.DefaultClient. Either way
	// Resolve applies its own redirect policy.
	Client *http.Client
	// Credentials are the tokens Resolve may attach, each only for the host
	// of the auth server that issued it.
	Credentials []Credential
}

// Resolve returns the state JSON stored at location, the fragment of a Share
// link after "#!". A location whose scheme is not https or middleauth+https
// returns an error wrapping errors.ErrUnsupported.
func (r *Resolver) Resolve(location string) (data []byte, err error) {
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

	cred := r.credentialFor(target)
	var source string
	if cred != nil {
		var token string
		token, source, err = cred.token()
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+token)
		// The server gets the token and can repeat it in whatever Go puts in
		// an error, such as a header or a malformed response line, so no
		// error from here on carries it.
		defer func() {
			if err != nil {
				err = redactToken(err, token)
			}
		}()
	}

	resp, err := httpx.Do(r.client(), req)
	if err != nil {
		return nil, describeFailure(err, cred, source)
	}
	defer resp.Body.Close()

	data, err = io.ReadAll(io.LimitReader(resp.Body, maxStateBytes+1))
	if err != nil {
		return nil, fmt.Errorf("reading the state: %w", err)
	}
	if int64(len(data)) > maxStateBytes {
		return nil, fmt.Errorf("the state exceeds %d bytes", maxStateBytes)
	}
	if !json.Valid(data) {
		return nil, describeNonJSON(resp.Header.Get("Content-Type"))
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
	// net/http would contact a non-ASCII host under its IDNA form, which can
	// be another domain than the one the link seems to name. %+q spells out
	// the characters that make it look like an ASCII name.
	if host := target.Hostname(); !isASCII(host) {
		return nil, fmt.Errorf("the state location's host %+q contains non-ASCII characters and may imitate another host; crantcli accepts only ASCII host names, such as an internationalized name's xn-- form", host)
	}
	return target, nil
}

// isASCII reports whether s is all ASCII, the only host names net/http
// contacts as written.
func isASCII(s string) bool {
	return !strings.ContainsFunc(s, func(r rune) bool { return r > unicode.MaxASCII })
}

func (r *Resolver) credentialFor(target *url.URL) *Credential {
	for i := range r.Credentials {
		if r.Credentials[i].matches(target) {
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

// describeFailure says why the state server did not return the state, and
// for a rejected token, where that token came from. The server's words reach
// the user only through httpx.StatusError, which neutralizes control
// characters, or quoted.
func describeFailure(err error, cred *Credential, source string) error {
	var statusErr *httpx.StatusError
	if !errors.As(err, &statusErr) {
		return fmt.Errorf("fetching the state: %w", err)
	}
	code := statusErr.StatusCode
	switch {
	case (code == http.StatusUnauthorized || code == http.StatusForbidden) && cred != nil:
		return fmt.Errorf("the state server rejected the %s (HTTP %d); %s to replace it", cred.describe(source), code, cred.Setup)
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

// describeNonJSON says the response is not a state and names its media type,
// such as text/html for a sign-in page. It leaves out the Content-Type's
// parameters, which the server fills as it likes, and a media type that is
// not type/subtype. The media type keeps the server's spelling, which
// mime.ParseMediaType would lowercase, so Resolve can find a token in it.
func describeNonJSON(contentType string) error {
	_, _, err := mime.ParseMediaType(contentType)
	mediaType, _, _ := strings.Cut(contentType, ";")
	mediaType = strings.TrimSpace(mediaType)
	if (err != nil && !errors.Is(err, mime.ErrInvalidMediaParameter)) || !strings.Contains(mediaType, "/") {
		return errors.New("the state server's response is not JSON")
	}
	return fmt.Errorf("the state server's response is not JSON (Content-Type %q)", mediaType)
}

// redactToken returns an error with err's message, in which token reads
// [REDACTED] both as is and as %q spells it, the way net/http quotes server
// text. It does not wrap err, whose errors still hold the server's words.
func redactToken(err error, token string) error {
	msg := err.Error()
	quoted := strconv.Quote(token)
	for _, spelling := range []string{token, quoted[1 : len(quoted)-1]} {
		if spelling != "" {
			msg = strings.ReplaceAll(msg, spelling, "[REDACTED]")
		}
	}
	return errors.New(msg)
}
