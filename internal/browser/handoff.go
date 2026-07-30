package browser

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// prepareCommandOpenURL resolves the target handed to an argv-based opener.
// Every platform caps the length of a single exec argument, and Neuroglancer
// states routinely exceed the tightest of those caps, so oversized URLs are
// staged through a private local redirect file instead.
var prepareCommandOpenURL = commandOpenURL

// handoffLifetime bounds how long a staged redirect file is kept.
const handoffLifetime = 24 * time.Hour

func commandOpenURL(rawURL string) (string, error) {
	dir, err := handoffDir()
	if err != nil {
		return "", err
	}
	// Sweep on every open, not only oversized ones, so a session that stages a
	// single large state does not leave its URL on disk indefinitely.
	removeStaleHandoffs(dir)

	if len(rawURL) <= maxSafeOpenArgument {
		return rawURL, nil
	}

	token, err := handoffToken()
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, token+".html")
	encodedURL, err := json.Marshal(rawURL)
	if err != nil {
		return "", fmt.Errorf("encoding browser handoff URL: %w", err)
	}
	html := "<!doctype html><meta charset=utf-8>" +
		"<meta http-equiv=\"Content-Security-Policy\" content=\"default-src 'none'; script-src 'unsafe-inline'\">" +
		"<title>Opening Neuroglancer</title><script>location.replace(" + string(encodedURL) + ")</script>" +
		"<noscript>JavaScript is required for this local Neuroglancer handoff.</noscript>\n"
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", fmt.Errorf("creating browser handoff: %w", err)
	}
	if _, err := file.WriteString(html); err != nil {
		file.Close()
		os.Remove(path)
		return "", fmt.Errorf("writing browser handoff: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		os.Remove(path)
		return "", fmt.Errorf("syncing browser handoff: %w", err)
	}
	if err := file.Close(); err != nil {
		os.Remove(path)
		return "", fmt.Errorf("closing browser handoff: %w", err)
	}
	return fileURL(path), nil
}

func handoffDir() (string, error) {
	cacheRoot, cacheErr := os.UserCacheDir()
	if cacheErr != nil || cacheRoot == "" {
		dir, err := os.MkdirTemp("", "crantcli-browser-handoffs-*")
		if err != nil {
			return "", fmt.Errorf("creating temporary browser handoff directory: %w", err)
		}
		return dir, nil
	}

	dir := filepath.Join(cacheRoot, "crantcli", "browser-handoffs")
	if info, err := os.Lstat(dir); err == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return "", fmt.Errorf("browser handoff path %s is not a private directory", dir)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("checking browser handoff directory: %w", err)
	} else if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("creating browser handoff directory: %w", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return "", fmt.Errorf("securing browser handoff directory: %w", err)
	}
	return dir, nil
}

// sweepStaleHandoffs removes expired redirect files on every open, including
// the portal path that never stages one. Best-effort: a sweep failure must not
// block the open itself.
func sweepStaleHandoffs() {
	if dir, err := handoffDir(); err == nil {
		removeStaleHandoffs(dir)
	}
}

func removeStaleHandoffs(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-handoffLifetime)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".html") {
			continue
		}
		info, err := entry.Info()
		if err == nil && info.ModTime().Before(cutoff) {
			_ = os.Remove(filepath.Join(dir, entry.Name()))
		}
	}
}

// fileURL renders a local path as a file:// URL. Windows paths are converted to
// slash form and given the leading slash that turns "C:/dir/x" into the
// "file:///C:/dir/x" spelling browsers expect.
func fileURL(path string) string {
	converted := filepath.ToSlash(path)
	if !strings.HasPrefix(converted, "/") {
		converted = "/" + converted
	}
	return (&url.URL{Scheme: "file", Path: converted}).String()
}

// handoffToken produces a random identifier usable both as a handoff filename
// and as an XDG portal handle_token, which admits only [A-Za-z0-9_].
func handoffToken() (string, error) {
	var random [12]byte
	if _, err := rand.Read(random[:]); err != nil {
		return "", fmt.Errorf("generating browser handoff token: %w", err)
	}
	return "crantcli_" + strings.ToLower(hex.EncodeToString(random[:])), nil
}
