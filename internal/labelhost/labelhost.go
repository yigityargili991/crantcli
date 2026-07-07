// Package labelhost publishes segment_properties "info" files to somewhere the
// browser can fetch them — a secret GitHub gist by default, or a user-supplied
// publish/cleanup hook command — and tracks them in a local manifest so they can
// be garbage-collected later.
//
// Hook contract (when a hook command is configured):
//   - Publish:  `<hook> publish`  — receives the info JSON on stdin, must print
//     a single JSON object {"url": "<source url>", "id": "<handle>"} to stdout.
//     `url` is embedded verbatim as a Neuroglancer layer source; `id` is an
//     opaque handle passed back for cleanup.
//   - Clean:    `<hook> clean <id>` — must exit 0 if the resource was removed or
//     is already gone; a non-zero exit is treated as a transient failure and the
//     entry is kept for a later retry.
package labelhost

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"
)

const (
	manifestDir  = ".crantcli"
	manifestFile = "label_gists.json" // legacy name; also holds hook-published entries

	kindGist = "gist"
	kindHook = "hook"
)

// Published describes a hosted segment_properties info file.
type Published struct {
	URL  string // full source URL to embed in the Neuroglancer state
	ID   string // opaque handle used to delete the resource later
	Kind string // kindGist or kindHook
}

// run executes a command (with optional stdin) and returns stdout. Overridable
// in tests.
var run = func(stdin []byte, name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		if msg != "" {
			return nil, fmt.Errorf("%s: %w: %s", name, err, msg)
		}
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return stdout.Bytes(), nil
}

// Seams overridable in tests.
var (
	nowFunc          = time.Now
	manifestPathFunc = defaultManifestPath
)

// Publish hosts the info bytes. If hookCmd is non-empty it is used; otherwise a
// secret GitHub gist is created via the gh CLI.
func Publish(hookCmd string, info []byte) (Published, error) {
	if hookCmd != "" {
		return publishHook(hookCmd, info)
	}
	return publishGist(info)
}

// EnsureGistAvailable verifies the gh CLI is installed and authenticated. Only
// the default (gist) backend needs it.
func EnsureGistAvailable() error {
	if _, err := exec.LookPath("gh"); err != nil {
		return fmt.Errorf("the GitHub CLI (gh) is required for --labels without --labels-hook; install it from https://cli.github.com and run 'gh auth login'")
	}
	if _, err := run(nil, "gh", "auth", "status"); err != nil {
		return fmt.Errorf("gh is not authenticated; run 'gh auth login': %w", err)
	}
	return nil
}

func publishGist(info []byte) (Published, error) {
	tmpDir, err := os.MkdirTemp("", "crant-segprops-")
	if err != nil {
		return Published{}, fmt.Errorf("creating temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	infoPath := filepath.Join(tmpDir, "info")
	if err := os.WriteFile(infoPath, info, 0o644); err != nil {
		return Published{}, fmt.Errorf("writing temp info file: %w", err)
	}

	out, err := run(nil, "gh", "gist", "create", infoPath, "--desc", "crantcli cell-type labels")
	if err != nil {
		return Published{}, fmt.Errorf("creating gist: %w", err)
	}
	id := parseGistID(string(out))
	if id == "" {
		return Published{}, fmt.Errorf("could not determine gist id from gh output: %q", strings.TrimSpace(string(out)))
	}

	rawOut, err := run(nil, "gh", "api", "gists/"+id, "--jq", `.files["info"].raw_url`)
	if err != nil {
		return Published{}, cleanupCreatedGist(id, fmt.Errorf("reading gist raw url: %w", err))
	}
	raw := strings.TrimSpace(string(rawOut))
	if !strings.HasSuffix(raw, "/info") {
		return Published{}, cleanupCreatedGist(id, fmt.Errorf("unexpected gist raw url %q (want a .../info path)", raw))
	}
	base := strings.TrimSuffix(raw, "info")
	return Published{URL: base + "|neuroglancer-precomputed:", ID: id, Kind: kindGist}, nil
}

func cleanupCreatedGist(id string, cause error) error {
	if err := deleteGist(id); err != nil {
		return fmt.Errorf("%w; additionally failed to delete created gist %s: %v", cause, id, err)
	}
	return cause
}

func deleteGist(id string) error {
	_, err := run(nil, "gh", "gist", "delete", id, "--yes")
	return err
}

// hookResult is the JSON a publish hook prints to stdout.
type hookResult struct {
	URL string `json:"url"`
	ID  string `json:"id"`
}

func publishHook(hookCmd string, info []byte) (Published, error) {
	name, args, err := splitHook(hookCmd)
	if err != nil {
		return Published{}, err
	}
	if name == "" {
		return Published{}, fmt.Errorf("empty --labels-hook command")
	}
	out, err := run(info, name, append(args, "publish")...)
	if err != nil {
		return Published{}, fmt.Errorf("publish hook failed: %w", err)
	}
	var res hookResult
	if err := json.Unmarshal(bytes.TrimSpace(out), &res); err != nil {
		return Published{}, fmt.Errorf("publish hook must print JSON {\"url\",\"id\"} to stdout; got %q: %w", strings.TrimSpace(string(out)), err)
	}
	if res.URL == "" {
		return Published{}, fmt.Errorf("publish hook returned an empty url")
	}
	if res.ID == "" {
		return Published{}, fmt.Errorf("publish hook returned an empty id")
	}
	return Published{URL: res.URL, ID: res.ID, Kind: kindHook}, nil
}

func deleteHook(hookCmd, id string) error {
	name, args, err := splitHook(hookCmd)
	if err != nil {
		return err
	}
	if name == "" {
		return fmt.Errorf("no --labels-hook configured to clean hook-published source %q", id)
	}
	_, err = run(nil, name, append(args, "clean", id)...)
	return err
}

// splitHook splits a hook command string into its executable and arguments,
// honoring shell-style quotes and returning a fresh args slice so callers can
// safely append.
func splitHook(hookCmd string) (string, []string, error) {
	fields, err := splitHookFields(hookCmd)
	if err != nil {
		return "", nil, fmt.Errorf("parsing --labels-hook: %w", err)
	}
	if len(fields) == 0 {
		return "", nil, nil
	}
	return fields[0], append([]string{}, fields[1:]...), nil
}

func splitHookFields(s string) ([]string, error) {
	var fields []string
	var current strings.Builder
	var quote rune
	escaped := false
	haveField := false

	flush := func() {
		fields = append(fields, current.String())
		current.Reset()
		haveField = false
	}

	for _, r := range s {
		if escaped {
			current.WriteRune(r)
			haveField = true
			escaped = false
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
				continue
			}
			if quote == '"' && r == '\\' {
				escaped = true
				continue
			}
			current.WriteRune(r)
			haveField = true
			continue
		}

		switch {
		case r == '\\':
			escaped = true
			haveField = true
		case r == '\'' || r == '"':
			quote = r
			haveField = true
		case unicode.IsSpace(r):
			if haveField {
				flush()
			}
		default:
			current.WriteRune(r)
			haveField = true
		}
	}

	if escaped {
		return nil, fmt.Errorf("dangling escape")
	}
	if quote != 0 {
		return nil, fmt.Errorf("unterminated quote")
	}
	if haveField {
		flush()
	}
	return fields, nil
}

// parseGistID extracts the gist ID (last path segment) from gh's output URL.
func parseGistID(out string) string {
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, "gist.github.com/") {
			continue
		}
		line = strings.TrimRight(line, "/")
		parts := strings.Split(line, "/")
		return parts[len(parts)-1]
	}
	return ""
}

type entry struct {
	ID        string    `json:"id"`
	URL       string    `json:"url,omitempty"`
	Kind      string    `json:"kind,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// kind returns the entry's backend, defaulting to gist for legacy entries
// written before the field existed.
func (e entry) kind() string {
	if e.Kind == "" {
		return kindGist
	}
	return e.Kind
}

func defaultManifestPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, manifestDir, manifestFile)
}

func readManifest() ([]entry, error) {
	path := manifestPathFunc()
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		return nil, nil
	}
	var entries []entry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parsing label manifest: %w", err)
	}
	return entries, nil
}

func writeManifest(entries []entry) error {
	path := manifestPathFunc()
	if path == "" {
		return fmt.Errorf("could not determine home directory")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating manifest directory: %w", err)
	}
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

// Record appends a published source to the manifest for later cleanup.
func Record(p Published) error {
	entries, err := readManifest()
	if err != nil {
		return err
	}
	entries = append(entries, entry{ID: p.ID, URL: p.URL, Kind: p.Kind, CreatedAt: nowFunc()})
	return writeManifest(entries)
}

// RecordedURLs returns the source URLs of all tracked label sources, so prior
// ones can be removed from a state before a new one is attached.
func RecordedURLs() []string {
	entries, _ := readManifest()
	urls := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.URL != "" {
			urls = append(urls, e.URL)
		}
	}
	return urls
}

// GC deletes tracked sources older than ttl and prunes the manifest. hookCmd is
// used to clean hook-published entries.
func GC(ttl time.Duration, hookCmd string) error {
	_, _, err := clean(false, ttl, hookCmd)
	return err
}

// Clean deletes tracked sources. When all is true, every tracked source is
// removed regardless of age; otherwise only those older than olderThan. It
// returns the number deleted and the number still tracked.
func Clean(all bool, olderThan time.Duration, hookCmd string) (deleted, kept int, err error) {
	return clean(all, olderThan, hookCmd)
}

func clean(all bool, age time.Duration, hookCmd string) (int, int, error) {
	entries, err := readManifest()
	if err != nil {
		return 0, 0, err
	}

	cutoff := nowFunc().Add(-age)
	kept := make([]entry, 0, len(entries))
	deleted := 0
	for _, e := range entries {
		expired := all || !e.CreatedAt.After(cutoff)
		if !expired {
			kept = append(kept, e)
			continue
		}
		if err := deleteEntry(e, hookCmd); err != nil {
			// A gist that 404s is already gone and safe to drop. Hook cleanups
			// instead follow the exit-code contract: any non-zero exit is
			// transient and must be retried, so their error text is never
			// consulted for a "gone" verdict.
			if e.kind() != kindGist || !isGistGone(err) {
				kept = append(kept, e) // keep for retry on a later run
				continue
			}
		}
		deleted++
	}

	if err := writeManifest(kept); err != nil {
		return deleted, len(kept), err
	}
	return deleted, len(kept), nil
}

func deleteEntry(e entry, hookCmd string) error {
	if e.kind() == kindHook {
		return deleteHook(hookCmd, e.ID)
	}
	return deleteGist(e.ID)
}

// isGistGone reports whether a `gh gist delete`/`gh api` error means the gist no
// longer exists, in which case dropping its manifest entry is safe. It is kept
// deliberately narrow: transient failures (offline, DNS, auth, rate limits) must
// NOT match, or a GC run during an outage would silently drop entries and orphan
// the gists on GitHub. gh reports a missing gist as an HTTP 404 ("Not Found") or
// a GraphQL "Could not resolve to a Gist with the id ..." message -- note that
// bare "could not resolve" also appears in "could not resolve host" (DNS), which
// is transient, so the full phrase is required.
func isGistGone(err error) bool {
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "executable file not found") {
		return false // gh itself is missing: a config/PATH problem, not a gone gist
	}
	return strings.Contains(msg, "http 404") ||
		strings.Contains(msg, "not found") ||
		strings.Contains(msg, "could not resolve to a")
}
