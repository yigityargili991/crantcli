package cmd

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

type updateStartCall struct {
	name string
	args []string
	env  []string
}

// stubUpdateSeams swaps the update command's version, platform, executable
// location, HTTP, and process seams, restoring them all when the test ends.
func stubUpdateSeams(t *testing.T, version, goos, exe string, fetch func(url string) ([]byte, error)) *[]updateStartCall {
	t.Helper()

	calls := []updateStartCall{}

	origVersion := Version
	origGOOS := updateGOOS
	origExe := updateExecutable
	origFetch := updateFetch
	origStart := updateStart
	Version = version
	updateGOOS = goos
	updateExecutable = func() (string, error) { return exe, nil }
	updateFetch = fetch
	updateStart = func(name string, args []string, env []string) error {
		calls = append(calls, updateStartCall{name: name, args: append([]string{}, args...), env: append([]string{}, env...)})
		return nil
	}
	t.Cleanup(func() {
		Version = origVersion
		updateGOOS = origGOOS
		updateExecutable = origExe
		updateFetch = origFetch
		updateStart = origStart
	})
	return &calls
}

// unsetEnvForTest removes key from the process environment for the duration
// of the test (t.Setenv can only set, not unset).
func unsetEnvForTest(t *testing.T, key string) {
	t.Helper()
	if value, ok := os.LookupEnv(key); ok {
		os.Unsetenv(key)
		t.Cleanup(func() { os.Setenv(key, value) })
	}
}

func envValue(env []string, key string) (string, bool) {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return strings.TrimPrefix(entry, prefix), true
		}
	}
	return "", false
}

func TestUpdateAlreadyUpToDate(t *testing.T) {
	calls := stubUpdateSeams(t, "v0.16.2", "linux", "/nonexistent/bin/crantcli", func(url string) ([]byte, error) {
		if url != updateLatestURL {
			return nil, fmt.Errorf("unexpected fetch %s", url)
		}
		return []byte(`{"tag_name":"v0.16.2"}`), nil
	})

	out, errOut, err := executeRootForTest(t, "update")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if !strings.Contains(out, "already up to date (v0.16.2)") {
		t.Fatalf("stdout = %q, want already-up-to-date message", out)
	}
	if errOut != "" {
		t.Fatalf("stderr = %q, want empty", errOut)
	}
	if len(*calls) != 0 {
		t.Fatalf("installer started despite up-to-date version: %+v", *calls)
	}
}

func TestUpdateSpawnsShOnUnix(t *testing.T) {
	calls := stubUpdateSeams(t, "v0.16.1", "darwin", "/nonexistent/bin/crantcli", func(url string) ([]byte, error) {
		switch url {
		case updateLatestURL:
			return []byte(`{"tag_name":"v0.16.2"}`), nil
		case updateRawURL + "v0.16.2/install.sh":
			return []byte("#!/bin/sh\necho installer\n"), nil
		default:
			return nil, fmt.Errorf("unexpected fetch %s", url)
		}
	})

	out, _, err := executeRootForTest(t, "update")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if !strings.Contains(out, "Updating crantcli v0.16.1 -> v0.16.2") {
		t.Fatalf("stdout = %q, want update banner", out)
	}
	if len(*calls) != 1 {
		t.Fatalf("start calls = %+v, want exactly one", *calls)
	}
	call := (*calls)[0]
	if call.name != "sh" {
		t.Fatalf("started %q, want sh", call.name)
	}
	if len(call.args) != 1 || !strings.HasSuffix(call.args[0], ".sh") {
		t.Fatalf("sh args = %v, want a single .sh script path", call.args)
	}
	content, readErr := os.ReadFile(call.args[0])
	if readErr != nil {
		t.Fatalf("read spawned script: %v", readErr)
	}
	t.Cleanup(func() { os.Remove(call.args[0]) })
	if string(content) != "#!/bin/sh\necho installer\n" {
		t.Fatalf("script content = %q, want downloaded installer", content)
	}
}

func TestUpdateSpawnsPowerShellOnWindows(t *testing.T) {
	calls := stubUpdateSeams(t, "v0.16.1", "windows", "/nonexistent/bin/crantcli", func(url string) ([]byte, error) {
		switch url {
		case updateLatestURL:
			return []byte(`{"tag_name":"v0.16.2"}`), nil
		case updateRawURL + "v0.16.2/install.ps1":
			return []byte("Write-Host installer\n"), nil
		default:
			return nil, fmt.Errorf("unexpected fetch %s", url)
		}
	})

	_, _, err := executeRootForTest(t, "update")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if len(*calls) != 1 {
		t.Fatalf("start calls = %+v, want exactly one", *calls)
	}
	call := (*calls)[0]
	if call.name != "powershell" {
		t.Fatalf("started %q, want powershell", call.name)
	}
	joined := strings.Join(call.args, " ")
	if !strings.Contains(joined, "Start-Sleep") || !strings.Contains(joined, ".ps1") {
		t.Fatalf("powershell args = %v, want delayed .ps1 invocation", call.args)
	}
	for _, arg := range call.args {
		if strings.HasSuffix(arg, ".ps1") {
			t.Cleanup(func() { os.Remove(arg) })
		}
	}
}

func TestUpdateDevBuildAlwaysUpdates(t *testing.T) {
	calls := stubUpdateSeams(t, "dev", "linux", "/nonexistent/bin/crantcli", func(url string) ([]byte, error) {
		switch url {
		case updateLatestURL:
			return []byte(`{"tag_name":"v0.16.2"}`), nil
		case updateRawURL + "v0.16.2/install.sh":
			return []byte("#!/bin/sh\n"), nil
		default:
			return nil, fmt.Errorf("unexpected fetch %s", url)
		}
	})

	if _, _, err := executeRootForTest(t, "update"); err != nil {
		t.Fatalf("update: %v", err)
	}
	if len(*calls) != 1 {
		t.Fatalf("dev build skipped update: start calls = %+v", *calls)
	}
	t.Cleanup(func() { os.Remove((*calls)[0].args[0]) })
}

func TestUpdateLookupFailureProceeds(t *testing.T) {
	calls := stubUpdateSeams(t, "v0.16.1", "linux", "/nonexistent/bin/crantcli", func(url string) ([]byte, error) {
		switch url {
		case updateLatestURL:
			return nil, errors.New("network down")
		case updateRawURL + "main/install.sh":
			return []byte("#!/bin/sh\n"), nil
		default:
			return nil, fmt.Errorf("unexpected fetch %s", url)
		}
	})

	out, errOut, err := executeRootForTest(t, "update")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if !strings.Contains(errOut, "warning: could not check the latest release") {
		t.Fatalf("stderr = %q, want lookup warning", errOut)
	}
	if !strings.Contains(out, "Updating crantcli v0.16.1 -> latest") {
		t.Fatalf("stdout = %q, want update banner with latest target", out)
	}
	if len(*calls) != 1 || (*calls)[0].name != "sh" {
		t.Fatalf("start calls = %+v, want one sh invocation", *calls)
	}
	t.Cleanup(func() { os.Remove((*calls)[0].args[0]) })
}

func TestUpdateScriptDownloadFails(t *testing.T) {
	calls := stubUpdateSeams(t, "v0.16.1", "linux", "/nonexistent/bin/crantcli", func(url string) ([]byte, error) {
		if url == updateLatestURL {
			return []byte(`{"tag_name":"v0.16.2"}`), nil
		}
		return nil, errors.New("not found")
	})

	_, _, err := executeRootForTest(t, "update")
	if err == nil || !strings.Contains(err.Error(), "download install.sh") {
		t.Fatalf("error = %v, want download install.sh failure", err)
	}
	if len(*calls) != 0 {
		t.Fatalf("installer started despite failed download: %+v", *calls)
	}
}

func TestUpdateStartFailure(t *testing.T) {
	stubUpdateSeams(t, "v0.16.1", "linux", "/nonexistent/bin/crantcli", func(url string) ([]byte, error) {
		if url == updateLatestURL {
			return []byte(`{"tag_name":"v0.16.2"}`), nil
		}
		return []byte("#!/bin/sh\n"), nil
	})
	updateStart = func(string, []string, []string) error { return errors.New("exec failed") }

	_, _, err := executeRootForTest(t, "update")
	if err == nil || !strings.Contains(err.Error(), "launch installer") {
		t.Fatalf("error = %v, want launch installer failure", err)
	}
}

// The installer must replace the installation that launched the update, not
// its own default directory (PR review: one-shot /usr/local/bin installs and
// 'make install' put the binary where install.sh cannot infer it).
func TestUpdateTargetsRunningInstallDir(t *testing.T) {
	unsetEnvForTest(t, "CRANTCLI_INSTALL_DIR")
	t.Setenv("CRANTCLI_VERSION", "v0.0.1") // a stale pin must not reach the installer

	calls := stubUpdateSeams(t, "v0.16.1", "linux", "/nonexistent/bin/crantcli", func(url string) ([]byte, error) {
		if url == updateLatestURL {
			return []byte(`{"tag_name":"v0.16.2"}`), nil
		}
		return []byte("#!/bin/sh\n"), nil
	})

	if _, _, err := executeRootForTest(t, "update"); err != nil {
		t.Fatalf("update: %v", err)
	}
	if len(*calls) != 1 {
		t.Fatalf("start calls = %+v, want exactly one", *calls)
	}
	t.Cleanup(func() { os.Remove((*calls)[0].args[0]) })

	env := (*calls)[0].env
	if _, pinned := envValue(env, "CRANTCLI_VERSION"); pinned {
		t.Fatalf("installer env still carries CRANTCLI_VERSION: %v", env)
	}
	dir, ok := envValue(env, "CRANTCLI_INSTALL_DIR")
	if !ok || dir != "/nonexistent/bin" {
		t.Fatalf("CRANTCLI_INSTALL_DIR = %q (present %v), want /nonexistent/bin", dir, ok)
	}
}

// An explicitly exported CRANTCLI_INSTALL_DIR reflects a deliberate choice
// and must not be overridden by the executable location.
func TestUpdateRespectsExportedInstallDir(t *testing.T) {
	t.Setenv("CRANTCLI_INSTALL_DIR", "/custom/dir")

	calls := stubUpdateSeams(t, "v0.16.1", "linux", "/nonexistent/bin/crantcli", func(url string) ([]byte, error) {
		if url == updateLatestURL {
			return []byte(`{"tag_name":"v0.16.2"}`), nil
		}
		return []byte("#!/bin/sh\n"), nil
	})

	if _, _, err := executeRootForTest(t, "update"); err != nil {
		t.Fatalf("update: %v", err)
	}
	if len(*calls) != 1 {
		t.Fatalf("start calls = %+v, want exactly one", *calls)
	}
	t.Cleanup(func() { os.Remove((*calls)[0].args[0]) })

	dir, ok := envValue((*calls)[0].env, "CRANTCLI_INSTALL_DIR")
	if !ok || dir != "/custom/dir" {
		t.Fatalf("CRANTCLI_INSTALL_DIR = %q (present %v), want /custom/dir", dir, ok)
	}
}

func TestIsUpToDate(t *testing.T) {
	cases := []struct {
		current, latest string
		want            bool
	}{
		{"v0.16.2", "v0.16.2", true},
		{"0.16.2", "v0.16.2", true},
		{"v0.16.1", "v0.16.2", false},
		{"dev", "v0.16.2", false},
		{"", "v0.16.2", false},
	}
	for _, tc := range cases {
		if got := isUpToDate(tc.current, tc.latest); got != tc.want {
			t.Errorf("isUpToDate(%q, %q) = %v, want %v", tc.current, tc.latest, got, tc.want)
		}
	}
}

func TestInstallerEnv(t *testing.T) {
	env := []string{"HOME=/tmp", "CRANTCLI_VERSION=v0.1.0", "PATH=/bin"}
	got := installerEnv(env, "/usr/local/bin")
	if _, pinned := envValue(got, "CRANTCLI_VERSION"); pinned {
		t.Fatalf("installerEnv kept CRANTCLI_VERSION: %v", got)
	}
	if dir, _ := envValue(got, "CRANTCLI_INSTALL_DIR"); dir != "/usr/local/bin" {
		t.Fatalf("installerEnv install dir = %q, want /usr/local/bin: %v", dir, got)
	}
	if len(got) != 3 {
		t.Fatalf("installerEnv = %v, want 3 entries", got)
	}

	existing := []string{"CRANTCLI_INSTALL_DIR=/custom", "PATH=/bin"}
	got = installerEnv(existing, "/usr/local/bin")
	if dir, _ := envValue(got, "CRANTCLI_INSTALL_DIR"); dir != "/custom" {
		t.Fatalf("installerEnv overrode exported dir: %v", got)
	}
	if len(got) != 2 {
		t.Fatalf("installerEnv = %v, want exported env untouched", got)
	}

	got = installerEnv([]string{"PATH=/bin"}, "")
	if len(got) != 1 {
		t.Fatalf("installerEnv with unknown dir = %v, want unchanged env", got)
	}
}

func TestWithoutEnv(t *testing.T) {
	env := []string{"HOME=/tmp", "CRANTCLI_VERSION=v0.1.0", "PATH=/bin"}
	got := withoutEnv(env, "CRANTCLI_VERSION")
	if len(got) != 2 || got[0] != "HOME=/tmp" || got[1] != "PATH=/bin" {
		t.Fatalf("withoutEnv = %v, want CRANTCLI_VERSION removed", got)
	}
}
