package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type updateRunCall struct {
	name          string
	args          []string
	env           []string
	scriptContent []byte
}

// stubUpdateSeams swaps the update command's version, platform, executable
// locations, HTTP, and process seams, restoring them all when the test ends.
func stubUpdateSeams(t *testing.T, version, goos, exe string, fetch func(url string) ([]byte, error)) *[]updateRunCall {
	t.Helper()

	calls := []updateRunCall{}

	origVersion := Version
	origGOOS := updateGOOS
	origGOARCH := updateGOARCH
	origExe := updateExecutable
	origInvocation := updateInvocation
	origFetch := updateFetch
	origRun := updateRun
	Version = version
	updateGOOS = goos
	updateGOARCH = "amd64"
	updateExecutable = func() (string, error) { return exe, nil }
	updateInvocation = func() (string, error) { return exe, nil }
	updateFetch = fetch
	updateRun = func(name string, args []string, env []string) error {
		var scriptContent []byte
		for _, arg := range args {
			if strings.HasSuffix(arg, ".sh") || strings.HasSuffix(arg, ".ps1") {
				scriptContent, _ = os.ReadFile(arg)
				break
			}
		}
		calls = append(calls, updateRunCall{
			name:          name,
			args:          append([]string{}, args...),
			env:           append([]string{}, env...),
			scriptContent: scriptContent,
		})
		return nil
	}
	t.Cleanup(func() {
		Version = origVersion
		updateGOOS = origGOOS
		updateGOARCH = origGOARCH
		updateExecutable = origExe
		updateInvocation = origInvocation
		updateFetch = origFetch
		updateRun = origRun
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

func readyReleaseJSON(tag string) []byte {
	return []byte(fmt.Sprintf(
		`{"tag_name":%q,"assets":[{"name":"checksums.txt"},{"name":%q}]}`,
		tag,
		releaseAssetName(updateGOOS, updateGOARCH),
	))
}

func TestUpdateAlreadyUpToDate(t *testing.T) {
	calls := stubUpdateSeams(t, "v0.16.2", "linux", "/nonexistent/bin/crantcli", func(url string) ([]byte, error) {
		if url != updateLatestURL {
			return nil, fmt.Errorf("unexpected fetch %s", url)
		}
		return readyReleaseJSON("v0.16.2"), nil
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

func TestUpdateDoesNotDowngradeNewerBuild(t *testing.T) {
	calls := stubUpdateSeams(t, "v0.17.0-rc.1", "linux", "/nonexistent/bin/crantcli", func(url string) ([]byte, error) {
		if url != updateLatestURL {
			return nil, fmt.Errorf("unexpected fetch %s", url)
		}
		return readyReleaseJSON("v0.16.2"), nil
	})

	out, _, err := executeRootForTest(t, "update")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if !strings.Contains(out, "already up to date (v0.16.2)") {
		t.Fatalf("stdout = %q, want already-up-to-date message", out)
	}
	if len(*calls) != 0 {
		t.Fatalf("installer started a downgrade: %+v", *calls)
	}
}

func TestUpdateWaitsForInstallableRelease(t *testing.T) {
	calls := stubUpdateSeams(t, "v0.16.1", "linux", "/nonexistent/bin/crantcli", func(url string) ([]byte, error) {
		if url != updateLatestURL {
			return nil, fmt.Errorf("unexpected fetch %s", url)
		}
		return []byte(`{"tag_name":"v0.16.2","assets":[{"name":"checksums.txt"}]}`), nil
	})

	_, _, err := executeRootForTest(t, "update")
	if !errors.Is(err, errReleaseNotReady) {
		t.Fatalf("error = %v, want release-not-ready error", err)
	}
	if len(*calls) != 0 {
		t.Fatalf("installer started before release assets were ready: %+v", *calls)
	}
}

func TestUpdateRunsShOnUnix(t *testing.T) {
	calls := stubUpdateSeams(t, "v0.16.1", "darwin", "/nonexistent/bin/crantcli", func(url string) ([]byte, error) {
		switch url {
		case updateLatestURL:
			return readyReleaseJSON("v0.16.2"), nil
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
	if !strings.Contains(out, "Updated crantcli to v0.16.2") {
		t.Fatalf("stdout = %q, want completion message", out)
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
	if string(call.scriptContent) != "#!/bin/sh\necho installer\n" {
		t.Fatalf("script content = %q, want downloaded installer", call.scriptContent)
	}
}

func TestUpdateRunsPowerShellOnWindows(t *testing.T) {
	calls := stubUpdateSeams(t, "v0.16.1", "windows", "/nonexistent/bin/crantcli.exe", func(url string) ([]byte, error) {
		switch url {
		case updateLatestURL:
			return readyReleaseJSON("v0.16.2"), nil
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
	if strings.Contains(joined, "Start-Sleep") || !strings.Contains(joined, "-File") || !strings.Contains(joined, ".ps1") {
		t.Fatalf("powershell args = %v, want synchronous -File invocation", call.args)
	}
}

func TestUpdateDevBuildAlwaysUpdates(t *testing.T) {
	calls := stubUpdateSeams(t, "dev", "linux", "/nonexistent/bin/crantcli", func(url string) ([]byte, error) {
		switch url {
		case updateLatestURL:
			return readyReleaseJSON("v0.16.2"), nil
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
}

func TestUpdateLookupFailureDoesNotDowngrade(t *testing.T) {
	calls := stubUpdateSeams(t, "v0.17.0-rc.1", "linux", "/nonexistent/bin/crantcli", func(url string) ([]byte, error) {
		if url != updateLatestURL {
			return nil, fmt.Errorf("unexpected fetch %s", url)
		}
		return nil, errors.New("network down")
	})

	_, _, err := executeRootForTest(t, "update")
	if err == nil || !strings.Contains(err.Error(), "check latest release: network down") {
		t.Fatalf("error = %v, want lookup failure", err)
	}
	if len(*calls) != 0 {
		t.Fatalf("installer started without a resolved target: %+v", *calls)
	}
}

func TestUpdateRejectsRenamedExecutable(t *testing.T) {
	calls := stubUpdateSeams(t, "v0.16.1", "linux", "/downloads/crant_type_look-linux-amd64", func(url string) ([]byte, error) {
		if url != updateLatestURL {
			return nil, fmt.Errorf("unexpected fetch %s", url)
		}
		return readyReleaseJSON("v0.16.2"), nil
	})

	_, _, err := executeRootForTest(t, "update")
	if err == nil || !strings.Contains(err.Error(), `is not named "crantcli"`) {
		t.Fatalf("error = %v, want renamed-executable rejection", err)
	}
	if len(*calls) != 0 {
		t.Fatalf("installer started for renamed executable: %+v", *calls)
	}
}

func TestUpdateTargetsCanonicalSymlinkLauncher(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation is not reliably available on Windows CI")
	}
	unsetEnvForTest(t, "CRANTCLI_INSTALL_DIR")

	target := filepath.Join(t.TempDir(), "crant_type_look-linux-amd64")
	if err := os.WriteFile(target, []byte("old binary"), 0o755); err != nil {
		t.Fatalf("write target: %v", err)
	}
	launcherDir := t.TempDir()
	launcher := filepath.Join(launcherDir, "crantcli")
	if err := os.Symlink(target, launcher); err != nil {
		t.Fatalf("create launcher symlink: %v", err)
	}

	calls := stubUpdateSeams(t, "v0.16.1", "linux", target, func(url string) ([]byte, error) {
		switch url {
		case updateLatestURL:
			return readyReleaseJSON("v0.16.2"), nil
		case updateRawURL + "v0.16.2/install.sh":
			return []byte("#!/bin/sh\n"), nil
		default:
			return nil, fmt.Errorf("unexpected fetch %s", url)
		}
	})
	updateInvocation = func() (string, error) { return launcher, nil }

	if _, _, err := executeRootForTest(t, "update"); err != nil {
		t.Fatalf("update: %v", err)
	}
	if len(*calls) != 1 {
		t.Fatalf("start calls = %+v, want exactly one", *calls)
	}
	if dir, ok := envValue((*calls)[0].env, "CRANTCLI_INSTALL_DIR"); !ok || dir != launcherDir {
		t.Fatalf("CRANTCLI_INSTALL_DIR = %q (present %v), want launcher dir %q", dir, ok, launcherDir)
	}
}

func TestUpdateScriptDownloadFails(t *testing.T) {
	calls := stubUpdateSeams(t, "v0.16.1", "linux", "/nonexistent/bin/crantcli", func(url string) ([]byte, error) {
		if url == updateLatestURL {
			return readyReleaseJSON("v0.16.2"), nil
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

func TestUpdateRunFailure(t *testing.T) {
	stubUpdateSeams(t, "v0.16.1", "linux", "/nonexistent/bin/crantcli", func(url string) ([]byte, error) {
		if url == updateLatestURL {
			return readyReleaseJSON("v0.16.2"), nil
		}
		return []byte("#!/bin/sh\n"), nil
	})
	updateRun = func(string, []string, []string) error { return errors.New("exec failed") }

	_, _, err := executeRootForTest(t, "update")
	if err == nil || !strings.Contains(err.Error(), "run installer") {
		t.Fatalf("error = %v, want run installer failure", err)
	}
}

func TestRunProcessReturnsInstallerExitError(t *testing.T) {
	env := append(os.Environ(), "CRANTCLI_UPDATE_HELPER_PROCESS=1")
	err := runProcess(os.Args[0], []string{"-test.run=TestUpdateHelperProcess"}, env)
	if err == nil {
		t.Fatal("runProcess returned nil for a failing installer process")
	}
}

func TestUpdateHelperProcess(t *testing.T) {
	if os.Getenv("CRANTCLI_UPDATE_HELPER_PROCESS") != "1" {
		return
	}
	os.Exit(7)
}

// The installer must replace the installation that launched the update, not
// its own default directory (PR review: one-shot /usr/local/bin installs and
// 'make install' put the binary where install.sh cannot infer it).
func TestUpdateTargetsRunningInstallDir(t *testing.T) {
	unsetEnvForTest(t, "CRANTCLI_INSTALL_DIR")
	t.Setenv("CRANTCLI_VERSION", "v0.0.1") // a stale pin must be replaced by the resolved tag

	calls := stubUpdateSeams(t, "v0.16.1", "linux", "/nonexistent/bin/crantcli", func(url string) ([]byte, error) {
		if url == updateLatestURL {
			return readyReleaseJSON("v0.16.2"), nil
		}
		return []byte("#!/bin/sh\n"), nil
	})

	if _, _, err := executeRootForTest(t, "update"); err != nil {
		t.Fatalf("update: %v", err)
	}
	if len(*calls) != 1 {
		t.Fatalf("start calls = %+v, want exactly one", *calls)
	}

	env := (*calls)[0].env
	if version, pinned := envValue(env, "CRANTCLI_VERSION"); !pinned || version != "v0.16.2" {
		t.Fatalf("CRANTCLI_VERSION = %q (present %v), want v0.16.2", version, pinned)
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
			return readyReleaseJSON("v0.16.2"), nil
		}
		return []byte("#!/bin/sh\n"), nil
	})

	if _, _, err := executeRootForTest(t, "update"); err != nil {
		t.Fatalf("update: %v", err)
	}
	if len(*calls) != 1 {
		t.Fatalf("start calls = %+v, want exactly one", *calls)
	}

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
		{"v0.17.0", "v0.16.2", true},
		{"v0.17.0-rc.1", "v0.16.2", true},
		{"v0.16.2+local", "v0.16.2", true},
		{"v0.16.2-rc.1", "v0.16.2", false},
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
	got := installerEnv(env, "/usr/local/bin", "v0.16.2")
	if version, pinned := envValue(got, "CRANTCLI_VERSION"); !pinned || version != "v0.16.2" {
		t.Fatalf("installerEnv CRANTCLI_VERSION = %q (present %v), want v0.16.2: %v", version, pinned, got)
	}
	if dir, _ := envValue(got, "CRANTCLI_INSTALL_DIR"); dir != "/usr/local/bin" {
		t.Fatalf("installerEnv install dir = %q, want /usr/local/bin: %v", dir, got)
	}
	if len(got) != 4 {
		t.Fatalf("installerEnv = %v, want 4 entries", got)
	}

	existing := []string{"CRANTCLI_INSTALL_DIR=/custom", "PATH=/bin"}
	got = installerEnv(existing, "/usr/local/bin", "v0.16.2")
	if dir, _ := envValue(got, "CRANTCLI_INSTALL_DIR"); dir != "/custom" {
		t.Fatalf("installerEnv overrode exported dir: %v", got)
	}
	if len(got) != 3 {
		t.Fatalf("installerEnv = %v, want install dir preserved and version appended", got)
	}

	got = installerEnv([]string{"PATH=/bin"}, "", "")
	if len(got) != 1 {
		t.Fatalf("installerEnv with unknown dir = %v, want unchanged env", got)
	}

	empty := []string{"CRANTCLI_INSTALL_DIR=", "PATH=/bin"}
	got = installerEnv(empty, "/usr/local/bin", "")
	if dir, ok := envValue(got, "CRANTCLI_INSTALL_DIR"); !ok || dir != "/usr/local/bin" {
		t.Fatalf("installerEnv empty install dir = %q (present %v), want detected dir: %v", dir, ok, got)
	}
	if len(got) != 2 {
		t.Fatalf("installerEnv kept duplicate empty install dir: %v", got)
	}
}

func TestInstallerEnvMatchesWindowsKeysCaseInsensitively(t *testing.T) {
	origGOOS := updateGOOS
	updateGOOS = "windows"
	t.Cleanup(func() { updateGOOS = origGOOS })

	env := []string{
		"CrantCli_Version=v0.1.0",
		`CrantCli_Install_Dir=C:\Tools\crantcli`,
		"PATH=C:\\Windows",
	}
	got := installerEnv(env, `C:\Other`, "v0.16.2")

	if version, ok := envValue(got, "CRANTCLI_VERSION"); !ok || version != "v0.16.2" {
		t.Fatalf("CRANTCLI_VERSION = %q (present %v), want v0.16.2: %v", version, ok, got)
	}
	if _, stale := envValue(got, "CrantCli_Version"); stale {
		t.Fatalf("installerEnv kept differently-cased stale version: %v", got)
	}
	if dir, ok := envValue(got, "CrantCli_Install_Dir"); !ok || dir != `C:\Tools\crantcli` {
		t.Fatalf("CrantCli_Install_Dir = %q (present %v), want existing directory: %v", dir, ok, got)
	}
	if _, duplicate := envValue(got, "CRANTCLI_INSTALL_DIR"); duplicate {
		t.Fatalf("installerEnv appended a duplicate Windows install directory: %v", got)
	}
}

func TestWithoutEnv(t *testing.T) {
	env := []string{"HOME=/tmp", "CRANTCLI_VERSION=v0.1.0", "PATH=/bin"}
	got := withoutEnv(env, "CRANTCLI_VERSION")
	if len(got) != 2 || got[0] != "HOME=/tmp" || got[1] != "PATH=/bin" {
		t.Fatalf("withoutEnv = %v, want CRANTCLI_VERSION removed", got)
	}
}
