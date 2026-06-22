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
		return Published{}, fmt.Errorf("reading gist raw url: %w", err)
	}
	raw := strings.TrimSpace(string(rawOut))
	if !strings.HasSuffix(raw, "/info") {
		return Published{}, fmt.Errorf("unexpected gist raw url %q (want a .../info path)", raw)
	}
	base := strings.TrimSuffix(raw, "info")
	return Published{URL: base + "|neuroglancer-precomputed:", ID: id, Kind: kindGist}, nil
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
	name, args := splitHook(hookCmd)
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
	name, args := splitHook(hookCmd)
	if name == "" {
		return fmt.Errorf("no --labels-hook configured to clean hook-published source %q", id)
	}
	_, err := run(nil, name, append(args, "clean", id)...)
	return err
}

// splitHook splits a hook command string into its executable and arguments,
// returning a fresh args slice so callers can safely append.
func splitHook(hookCmd string) (string, []string) {
	fields := strings.Fields(hookCmd)
	if len(fields) == 0 {
		return "", nil
	}
	return fields[0], append([]string{}, fields[1:]...)
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
		if err := deleteEntry(e, hookCmd); err != nil && !isGone(err) {
			kept = append(kept, e) // transient failure: retry on a later run
			continue
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

// isGone reports whether a delete error means the resource no longer exists, in
// which case dropping its manifest entry is safe.
func isGone(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not found") ||
		strings.Contains(msg, "404") ||
		strings.Contains(msg, "could not resolve")
}
