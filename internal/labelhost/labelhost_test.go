// Copyright 2026 Yigit Yargili
// SPDX-License-Identifier: Apache-2.0

package labelhost

import (
	"bytes"
	"fmt"
	"os"
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
			id := args[len(args)-1] // gh gist delete --yes -- <id>
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

func TestGC_KeepsAllDeleteFailuresPrunesOnlyOnSuccess(t *testing.T) {
	base := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	withManifest(t, base)

	nowFunc = func() time.Time { return base.Add(-100 * time.Hour) }
	for _, id := range []string{"flaky", "dnsdown", "http404", "ok"} {
		if err := Record(Published{ID: id, Kind: kindGist}); err != nil {
			t.Fatal(err)
		}
	}

	nowFunc = func() time.Time { return base }
	// "ok" deletes cleanly; the rest fail. A 404 is treated exactly like a
	// transient error -- kept for retry -- because GitHub also returns 404 for a
	// live secret gist the current token can't access (wrong account / missing
	// `gist` scope), so a 404 never proves the gist is gone.
	stubRun(map[string]string{
		"flaky":   "dial tcp: connection refused",
		"dnsdown": `Get "https://api.github.com/gists/dnsdown": dial tcp: lookup api.github.com: could not resolve host`,
		"http404": "HTTP 404: Not Found",
	})

	if err := GC(time.Hour, ""); err != nil {
		t.Fatal(err)
	}

	entries, err := readManifest()
	if err != nil {
		t.Fatal(err)
	}
	kept := map[string]bool{}
	for _, e := range entries {
		kept[e.ID] = true
	}
	// Only "ok" (a successful delete) is pruned; every failure is retried.
	if len(entries) != 3 || !kept["flaky"] || !kept["dnsdown"] || !kept["http404"] {
		t.Errorf("remaining = %v, want flaky, dnsdown, http404 kept (only ok dropped)", entries)
	}
}

// TestGC_HookNonZeroExitKeptEvenWhenMessageLooksGone locks in the hook contract:
// a non-zero clean exit is always transient (kept for retry), even if its stderr
// happens to contain gone-looking text like "404" or "not found". Only the gist
// backend inspects error text.
func TestGC_HookNonZeroExitKeptEvenWhenMessageLooksGone(t *testing.T) {
	base := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	withManifest(t, base)

	nowFunc = func() time.Time { return base.Add(-100 * time.Hour) }
	if err := Record(Published{ID: "h1", Kind: kindHook}); err != nil {
		t.Fatal(err)
	}

	nowFunc = func() time.Time { return base }
	run = func(_ []byte, _ string, _ ...string) ([]byte, error) {
		return nil, &fakeErr{"widget 404: not found"}
	}

	if err := GC(time.Hour, "myhook"); err != nil {
		t.Fatal(err)
	}

	entries, err := readManifest()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].ID != "h1" {
		t.Errorf("hook entry must be kept for retry on non-zero exit, got %v", entries)
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

func TestSplitHookFields(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"single quotes are literal", `'a\b'`, []string{`a\b`}},
		{"single quotes group words", `'two words'`, []string{"two words"}},
		{"escaped space unquoted", `escaped\ space`, []string{"escaped space"}},
		{"escaped quote in double quotes", `"a \"b\" c"`, []string{`a "b" c`}},
		{"escaped backslash in double quotes", `"a \\ b"`, []string{`a \ b`}},
		// Inside double quotes a backslash escapes only '"' or '\', so a quoted
		// Windows path keeps its separators.
		{"Windows path in double quotes", `"C:\Tools\label hook.exe" --flag`, []string{`C:\Tools\label hook.exe`, "--flag"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := splitHookFields(tt.in)
			if err != nil {
				t.Fatalf("splitHookFields(%q): %v", tt.in, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("splitHookFields(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSplitHookFieldsUnterminatedQuote(t *testing.T) {
	// In `"abc\"` the last quote is escaped, so the field never closes.
	for _, in := range []string{`"unterminated`, `'unterminated`, `"abc\"`} {
		if _, err := splitHookFields(in); err == nil || !strings.Contains(err.Error(), "unterminated quote") {
			t.Errorf("splitHookFields(%q) error = %v, want unterminated quote", in, err)
		}
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

func TestPublishGistDeletesCreatedGistWhenRawURLLookupFails(t *testing.T) {
	withManifest(t, time.Now())
	var deleted []string
	run = func(_ []byte, name string, args ...string) ([]byte, error) {
		if name != "gh" {
			t.Fatalf("unexpected command %q", name)
		}
		if len(args) >= 2 && args[0] == "gist" && args[1] == "create" {
			return []byte("https://gist.github.com/user/abc123\n"), nil
		}
		if len(args) >= 1 && args[0] == "api" {
			return nil, &fakeErr{"temporary api failure"}
		}
		if len(args) >= 3 && args[0] == "gist" && args[1] == "delete" {
			deleted = append(deleted, args[len(args)-1])
			return nil, nil
		}
		t.Fatalf("unexpected gh args %v", args)
		return nil, nil
	}

	_, err := Publish("", []byte("{}"))
	if err == nil {
		t.Fatal("expected raw URL lookup error")
	}
	if !strings.Contains(err.Error(), "reading gist raw url") {
		t.Fatalf("error = %q, want raw URL lookup context", err.Error())
	}
	if !reflect.DeepEqual(deleted, []string{"abc123"}) {
		t.Fatalf("deleted gists = %v, want [abc123]", deleted)
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

type fakeErr struct{ msg string }

func (e *fakeErr) Error() string { return e.msg }

// SEC-004 regression: cleaning with nothing tracked (or nothing deleted) must
// not create or rewrite the manifest file.
func TestCleanNoopWritesNothing(t *testing.T) {
	dir := t.TempDir()
	manifest := filepath.Join(dir, "label_gists.json")
	manifestPathFunc = func() string { return manifest }
	t.Cleanup(func() { manifestPathFunc = defaultManifestPath })

	deleted, kept, err := Clean(true, time.Hour, "")
	if err != nil {
		t.Fatalf("Clean: %v", err)
	}
	if deleted != 0 || kept != 0 {
		t.Fatalf("deleted=%d kept=%d, want 0/0", deleted, kept)
	}
	if _, err := os.Stat(manifest); !os.IsNotExist(err) {
		t.Fatalf("manifest was written by a no-op clean")
	}
}

// When deletions fail, the manifest content must stay byte-identical.
func TestCleanAllFailuresWritesNothing(t *testing.T) {
	base := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	dir := t.TempDir()
	manifest := filepath.Join(dir, "label_gists.json")
	manifestPathFunc = func() string { return manifest }
	nowFunc = func() time.Time { return base }
	origRun := run
	t.Cleanup(func() {
		manifestPathFunc = defaultManifestPath
		nowFunc = time.Now
		run = origRun
	})

	if err := Record(Published{ID: "flaky", Kind: kindGist}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}

	stubRun(map[string]string{"flaky": "boom"})
	deleted, kept, err := Clean(true, time.Hour, "")
	if err != nil {
		t.Fatalf("Clean: %v", err)
	}
	if deleted != 0 || kept != 1 {
		t.Fatalf("deleted=%d kept=%d, want 0/1", deleted, kept)
	}
	after, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("manifest rewritten despite no deletions:\nbefore: %s\nafter: %s", before, after)
	}
}

// TestCleanRetiresDeletedURLs covers both cleanup paths: only a source that was
// actually deleted, and has a URL, moves to the retired ledger. Failed deletes
// and fresh sources stay tracked and are not retired.
func TestCleanRetiresDeletedURLs(t *testing.T) {
	base := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name  string
		clean func() error
	}{
		{"GC", func() error { return GC(time.Hour, "") }},
		{"Clean", func() error { _, _, err := Clean(false, time.Hour, ""); return err }},
	} {
		t.Run(test.name, func(t *testing.T) {
			withManifest(t, base.Add(-100*time.Hour))
			for _, id := range []string{"ok", "bad", "nourl"} {
				url := "https://hook.example/" + id + "/|neuroglancer-precomputed:"
				if id == "nourl" {
					url = ""
				}
				if err := Record(Published{ID: id, URL: url, Kind: kindGist}); err != nil {
					t.Fatal(err)
				}
			}
			nowFunc = func() time.Time { return base }
			fresh := "https://hook.example/fresh/|neuroglancer-precomputed:"
			if err := Record(Published{ID: "fresh", URL: fresh, Kind: kindGist}); err != nil {
				t.Fatal(err)
			}
			stubRun(map[string]string{"bad": "HTTP 404: Not Found"})

			if err := test.clean(); err != nil {
				t.Fatal(err)
			}

			retired, err := readRetired()
			if err != nil {
				t.Fatal(err)
			}
			want := []retiredEntry{{URL: "https://hook.example/ok/|neuroglancer-precomputed:", RetiredAt: base}}
			if !reflect.DeepEqual(retired, want) {
				t.Errorf("retired = %v, want %v", retired, want)
			}
			if got := RetiredURLs(); !reflect.DeepEqual(got, []string{want[0].URL}) {
				t.Errorf("RetiredURLs = %v, want %v", got, []string{want[0].URL})
			}
			recorded := RecordedURLs()
			sort.Strings(recorded)
			if wantRecorded := []string{"https://hook.example/bad/|neuroglancer-precomputed:", fresh}; !reflect.DeepEqual(recorded, wantRecorded) {
				t.Errorf("RecordedURLs = %v, want %v", recorded, wantRecorded)
			}
		})
	}
}

// A cleanup that deletes nothing must not create the retired ledger either.
func TestCleanWithoutDeletionsWritesNoRetiredLedger(t *testing.T) {
	for _, test := range []struct {
		name   string
		record bool
	}{
		{name: "nothing tracked"},
		{name: "every delete fails", record: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			withManifest(t, time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC))
			if test.record {
				if err := Record(Published{ID: "flaky", URL: "https://hook.example/flaky/", Kind: kindGist}); err != nil {
					t.Fatal(err)
				}
			}
			stubRun(map[string]string{"flaky": "boom"})

			if _, _, err := Clean(true, time.Hour, ""); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(retiredPath()); !os.IsNotExist(err) {
				t.Fatalf("retired ledger written by a cleanup that deleted nothing (stat err %v)", err)
			}
		})
	}
}

func TestRetireKeepsNewestURLs(t *testing.T) {
	base := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	withManifest(t, base)
	old := make([]retiredEntry, maxRetired)
	for i := range old {
		old[i] = retiredEntry{URL: fmt.Sprintf("https://hook.example/old-%d/", i), RetiredAt: base.Add(-time.Hour)}
	}
	if err := writeList(retiredPath(), old); err != nil {
		t.Fatal(err)
	}

	// Retiring a URL that is already in the ledger moves it to the end rather
	// than listing it twice; the oldest entries make room for the new ones.
	again := old[5].URL
	if err := retire([]entry{{URL: "https://hook.example/new/"}, {URL: again}, {URL: ""}}); err != nil {
		t.Fatal(err)
	}

	retired, err := readRetired()
	if err != nil {
		t.Fatal(err)
	}
	if len(retired) != maxRetired {
		t.Fatalf("ledger holds %d URLs, want %d", len(retired), maxRetired)
	}
	if got, want := retired[0].URL, old[1].URL; got != want {
		t.Errorf("oldest kept = %q, want %q (only old-0 dropped)", got, want)
	}
	tail := []retiredEntry{{URL: "https://hook.example/new/", RetiredAt: base}, {URL: again, RetiredAt: base}}
	if got := retired[len(retired)-2:]; !reflect.DeepEqual(got, tail) {
		t.Errorf("newest = %v, want %v", got, tail)
	}
	for _, r := range retired[:len(retired)-2] {
		if r.URL == again {
			t.Fatalf("%q is listed twice", again)
		}
	}
}

// A hook may publish every run to the same URL. Once published again, that URL
// is live, and the retired ledger must not make commands drop it.
func TestRetiredURLsSkipsRepublishedURL(t *testing.T) {
	withManifest(t, time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC))
	url := "https://hook.example/fixed/|neuroglancer-precomputed:"
	if err := retire([]entry{{URL: url}}); err != nil {
		t.Fatal(err)
	}
	if got := RetiredURLs(); !reflect.DeepEqual(got, []string{url}) {
		t.Fatalf("RetiredURLs = %v, want %v", got, []string{url})
	}

	if err := Record(Published{ID: "fixed", URL: url, Kind: kindHook}); err != nil {
		t.Fatal(err)
	}
	if got := RetiredURLs(); len(got) != 0 {
		t.Fatalf("RetiredURLs = %v, want the republished URL left out", got)
	}
}

// The sources are gone by the time the ledger is written, so failing to write
// it warns but neither fails the cleanup nor keeps the deleted entries tracked.
func TestCleanWarnsWhenRetiredLedgerFails(t *testing.T) {
	withManifest(t, time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC))
	var warnings bytes.Buffer
	warnOut = &warnings
	t.Cleanup(func() { warnOut = os.Stderr })
	if err := Record(Published{ID: "ok", URL: "https://hook.example/ok/", Kind: kindGist}); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(retiredPath(), 0o700); err != nil {
		t.Fatal(err)
	}
	stubRun(nil)

	deleted, kept, err := Clean(true, time.Hour, "")
	if err != nil {
		t.Fatalf("Clean: %v", err)
	}
	if deleted != 1 || kept != 0 {
		t.Fatalf("deleted=%d kept=%d, want 1/0", deleted, kept)
	}
	if entries, err := readManifest(); err != nil || len(entries) != 0 {
		t.Fatalf("manifest = %v (err %v), want the deleted entry gone", entries, err)
	}
	if !strings.Contains(warnings.String(), "Warning: could not record deleted label sources") {
		t.Fatalf("warnings = %q, want the ledger failure reported", warnings.String())
	}
}
