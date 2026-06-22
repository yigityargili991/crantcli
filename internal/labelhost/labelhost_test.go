package labelhost

import (
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"
)

func withManifest(t *testing.T, now time.Time) {
	t.Helper()
	dir := t.TempDir()
	manifestPathFunc = func() string { return filepath.Join(dir, "label_gists.json") }
	nowFunc = func() time.Time { return now }
	origRun := run
	t.Cleanup(func() {
		manifestPathFunc = defaultManifestPath
		nowFunc = time.Now
		run = origRun
	})
}

func TestRecordAndRecordedURLs(t *testing.T) {
	base := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	withManifest(t, base)

	if err := Record(Published{ID: "aaa", URL: "https://x/a/|neuroglancer-precomputed:", Kind: kindGist}); err != nil {
		t.Fatal(err)
	}
	if err := Record(Published{ID: "bbb", URL: "https://x/b/|neuroglancer-precomputed:", Kind: kindHook}); err != nil {
		t.Fatal(err)
	}

	urls := RecordedURLs()
	sort.Strings(urls)
	want := []string{"https://x/a/|neuroglancer-precomputed:", "https://x/b/|neuroglancer-precomputed:"}
	if !reflect.DeepEqual(urls, want) {
		t.Errorf("RecordedURLs = %v, want %v", urls, want)
	}
}

func TestReadManifest_MissingFile(t *testing.T) {
	withManifest(t, time.Now())
	entries, err := readManifest()
	if err != nil {
		t.Fatalf("expected nil error for missing manifest, got %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected no entries, got %v", entries)
	}
}

// stubRun makes the command runner succeed unless the deleted id is in failIDs,
// in which case it returns the given error text.
func stubRun(failIDs map[string]string) {
	run = func(_ []byte, name string, args ...string) ([]byte, error) {
		if len(args) > 0 && args[0] == "gist" && contains(args, "delete") {
			id := args[len(args)-2] // gh gist delete <id> --yes
			if msg, bad := failIDs[id]; bad {
				return nil, &fakeErr{msg}
			}
		}
		return nil, nil
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}

func TestGC_KeepsOnTransientDropsOnGone(t *testing.T) {
	base := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	withManifest(t, base)

	nowFunc = func() time.Time { return base.Add(-100 * time.Hour) }
	for _, id := range []string{"flaky", "gone", "ok"} {
		if err := Record(Published{ID: id, Kind: kindGist}); err != nil {
			t.Fatal(err)
		}
	}

	nowFunc = func() time.Time { return base }
	stubRun(map[string]string{
		"flaky": "dial tcp: connection refused",
		"gone":  "HTTP 404: Not Found",
	})

	if err := GC(time.Hour, ""); err != nil {
		t.Fatal(err)
	}

	entries, err := readManifest()
	if err != nil {
		t.Fatal(err)
	}
	// "ok" deleted, "gone" dropped (404), "flaky" kept for retry.
	if len(entries) != 1 || entries[0].ID != "flaky" {
		t.Errorf("remaining = %v, want only flaky", entries)
	}
}

func TestClean_AllIgnoresAge(t *testing.T) {
	base := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	withManifest(t, base)

	// Record at "now" so they are well within any TTL.
	if err := Record(Published{ID: "fresh1", Kind: kindGist}); err != nil {
		t.Fatal(err)
	}
	if err := Record(Published{ID: "fresh2", Kind: kindGist}); err != nil {
		t.Fatal(err)
	}
	stubRun(nil)

	deleted, kept, err := Clean(true, time.Hour, "")
	if err != nil {
		t.Fatal(err)
	}
	if deleted != 2 || kept != 0 {
		t.Errorf("deleted=%d kept=%d, want 2/0", deleted, kept)
	}
}

func TestPublishHook(t *testing.T) {
	withManifest(t, time.Now())
	var gotStdin []byte
	var gotArgs []string
	run = func(stdin []byte, name string, args ...string) ([]byte, error) {
		gotStdin = stdin
		gotArgs = append([]string{name}, args...)
		return []byte(`{"url":"https://host/p/|neuroglancer-precomputed:","id":"h1"}`), nil
	}

	pub, err := Publish("python label_hook.py", []byte(`{"info":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if pub.URL != "https://host/p/|neuroglancer-precomputed:" || pub.ID != "h1" || pub.Kind != kindHook {
		t.Errorf("unexpected published: %+v", pub)
	}
	if string(gotStdin) != `{"info":true}` {
		t.Errorf("hook stdin = %q, want the info JSON", gotStdin)
	}
	if !reflect.DeepEqual(gotArgs, []string{"python", "label_hook.py", "publish"}) {
		t.Errorf("hook argv = %v, want [python label_hook.py publish]", gotArgs)
	}
}

func TestPublishHook_QuotedCommand(t *testing.T) {
	withManifest(t, time.Now())
	var gotArgs []string
	run = func(_ []byte, name string, args ...string) ([]byte, error) {
		gotArgs = append([]string{name}, args...)
		return []byte(`{"url":"https://host/p/|neuroglancer-precomputed:","id":"h1"}`), nil
	}

	_, err := Publish(`python3 "/tmp/my hook.py" --label 'two words' escaped\ space`, []byte("{}"))
	if err != nil {
		t.Fatal(err)
	}

	want := []string{"python3", "/tmp/my hook.py", "--label", "two words", "escaped space", "publish"}
	if !reflect.DeepEqual(gotArgs, want) {
		t.Errorf("hook argv = %v, want %v", gotArgs, want)
	}
}

func TestPublishHook_BadOutput(t *testing.T) {
	withManifest(t, time.Now())
	run = func([]byte, string, ...string) ([]byte, error) { return []byte("not json"), nil }
	if _, err := Publish("hook", []byte("{}")); err == nil {
		t.Fatal("expected error for non-JSON hook output")
	}
}

func TestPublishHook_EmptyID(t *testing.T) {
	withManifest(t, time.Now())
	run = func([]byte, string, ...string) ([]byte, error) {
		return []byte(`{"url":"https://host/p/|neuroglancer-precomputed:"}`), nil
	}
	_, err := Publish("hook", []byte("{}"))
	if err == nil {
		t.Fatal("expected error for empty hook id")
	}
	if !strings.Contains(err.Error(), "empty id") {
		t.Fatalf("error = %q, want empty id message", err.Error())
	}
}

func TestPublishHook_BadCommandQuoting(t *testing.T) {
	withManifest(t, time.Now())
	_, err := Publish(`python3 "/tmp/my hook.py`, []byte("{}"))
	if err == nil {
		t.Fatal("expected error for unterminated quote")
	}
	if !strings.Contains(err.Error(), "unterminated quote") {
		t.Fatalf("error = %q, want unterminated quote message", err.Error())
	}
}

func TestParseGistID(t *testing.T) {
	tests := []struct{ out, want string }{
		{"https://gist.github.com/user/abc123\n", "abc123"},
		{"- Creating gist info\nhttps://gist.github.com/user/def456", "def456"},
		{"https://gist.github.com/user/ghi789/", "ghi789"},
		{"no url here", ""},
	}
	for _, tt := range tests {
		if got := parseGistID(tt.out); got != tt.want {
			t.Errorf("parseGistID(%q) = %q, want %q", tt.out, got, tt.want)
		}
	}
}

func TestIsGone(t *testing.T) {
	if !isGone(&fakeErr{"HTTP 404: Not Found"}) {
		t.Error("404 should be treated as gone")
	}
	if isGone(&fakeErr{"connection refused"}) {
		t.Error("transient error should not be treated as gone")
	}
}

type fakeErr struct{ msg string }

func (e *fakeErr) Error() string { return e.msg }
