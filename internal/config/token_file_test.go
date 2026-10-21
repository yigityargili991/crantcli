package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

type tokenFileSource struct {
	tokenVar, fileVar string
	lookup            func() string
}

// tokenFileSources pairs each token-file variable with the token variable
// that takes precedence over it and the lookup that reads it.
var tokenFileSources = []tokenFileSource{
	{"CRANTTABLE_TOKEN", "CRANTTABLE_TOKEN_FILE", GetAPIToken},
	{"CAVE_TOKEN", "CAVE_TOKEN_FILE", GetCAVEToken},
}

// lookupTokenFile points source's token-file variable at path and returns
// the token its lookup finds and what the lookup wrote to stderr. A lookup
// still blocked after five seconds fails the test.
func lookupTokenFile(t *testing.T, source tokenFileSource, path string) (token, warning string) {
	t.Helper()
	_, _ = setupTestHome(t)
	t.Setenv(source.tokenVar, "")
	t.Setenv(source.fileVar, path)

	capture, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatalf("create stderr capture: %v", err)
	}
	defer capture.Close()
	original := os.Stderr
	os.Stderr = capture
	defer func() { os.Stderr = original }()

	done := make(chan string, 1)
	go func() { done <- source.lookup() }()
	select {
	case token = <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("lookup with %s=%s still blocked after 5s", source.fileVar, path)
	}
	data, err := os.ReadFile(capture.Name())
	if err != nil {
		t.Fatalf("read stderr capture: %v", err)
	}
	return token, string(data)
}

// requireTokenFileRejected checks that every lookup ignores the file at
// path with a warning that names it and does not repeat secret.
func requireTokenFileRejected(t *testing.T, path, secret string) {
	t.Helper()
	for _, source := range tokenFileSources {
		t.Run(source.fileVar, func(t *testing.T) {
			token, warning := lookupTokenFile(t, source, path)
			if token != "" {
				t.Errorf("lookup returned a %d-byte token, want none", len(token))
			}
			if !strings.Contains(warning, path) {
				t.Errorf("warning = %q, want one that names %s", warning, path)
			}
			if secret != "" && strings.Contains(warning, secret) {
				t.Errorf("warning repeats the file's contents: %q", warning)
			}
		})
	}
}

// requireTokenFileRead checks that every lookup reads want from the file at
// path without a warning.
func requireTokenFileRead(t *testing.T, path, want string) {
	t.Helper()
	for _, source := range tokenFileSources {
		t.Run(source.fileVar, func(t *testing.T) {
			token, warning := lookupTokenFile(t, source, path)
			if token != want {
				t.Errorf("lookup = %q, want %q", token, want)
			}
			if warning != "" {
				t.Errorf("unexpected warning %q", warning)
			}
		})
	}
}

// writePaddedTokenFile writes token followed by newlines, size bytes in all.
func writePaddedTokenFile(t *testing.T, token string, size int) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "token")
	data := token + strings.Repeat("\n", size-len(token))
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}
	return path
}

func TestTokenFileRejectsDirectory(t *testing.T) {
	requireTokenFileRejected(t, t.TempDir(), "")
}

func TestTokenFileRejectsOversizedFile(t *testing.T) {
	path := writePaddedTokenFile(t, "oversized-token", maxTokenFileBytes+1)
	requireTokenFileRejected(t, path, "oversized-token")
}

func TestTokenFileReadsFileAtSizeLimit(t *testing.T) {
	path := writePaddedTokenFile(t, "file-token", maxTokenFileBytes)
	requireTokenFileRead(t, path, "file-token")
}

func TestTokenFileReadsThroughSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating symbolic links may require elevated Windows privileges")
	}
	target := writePaddedTokenFile(t, "linked-token", len("linked-token\n"))
	link := filepath.Join(t.TempDir(), "token-link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	requireTokenFileRead(t, link, "linked-token")
}
