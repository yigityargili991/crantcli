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
}

// stubUpdateSeams swaps the update command's version, platform, HTTP, and
// process seams, restoring them all when the test ends.
func stubUpdateSeams(t *testing.T, version, goos string, fetch func(url string) ([]byte, error)) *[]updateStartCall {
	t.Helper()

	calls := []updateStartCall{}

	origVersion := Version
	origGOOS := updateGOOS
	origFetch := updateFetch
	origStart := updateStart
	Version = version
	updateGOOS = goos
	updateFetch = fetch
	updateStart = func(name string, args ...string) error {
		calls = append(calls, updateStartCall{name: name, args: append([]string{}, args...)})
		return nil
	}
	t.Cleanup(func() {
		Version = origVersion
		updateGOOS = origGOOS
		updateFetch = origFetch
		updateStart = origStart
	})
	return &calls
}

func TestUpdateAlreadyUpToDate(t *testing.T) {
	calls := stubUpdateSeams(t, "v0.16.2", "linux", func(url string) ([]byte, error) {
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
	calls := stubUpdateSeams(t, "v0.16.1", "darwin", func(url string) ([]byte, error) {
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
	calls := stubUpdateSeams(t, "v0.16.1", "windows", func(url string) ([]byte, error) {
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
	calls := stubUpdateSeams(t, "dev", "linux", func(url string) ([]byte, error) {
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
	calls := stubUpdateSeams(t, "v0.16.1", "linux", func(url string) ([]byte, error) {
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
	calls := stubUpdateSeams(t, "v0.16.1", "linux", func(url string) ([]byte, error) {
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
	stubUpdateSeams(t, "v0.16.1", "linux", func(url string) ([]byte, error) {
		if url == updateLatestURL {
			return []byte(`{"tag_name":"v0.16.2"}`), nil
		}
		return []byte("#!/bin/sh\n"), nil
	})
	updateStart = func(string, ...string) error { return errors.New("exec failed") }

	_, _, err := executeRootForTest(t, "update")
	if err == nil || !strings.Contains(err.Error(), "launch installer") {
		t.Fatalf("error = %v, want launch installer failure", err)
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

func TestWithoutEnv(t *testing.T) {
	env := []string{"HOME=/tmp", "CRANTCLI_VERSION=v0.1.0", "PATH=/bin"}
	got := withoutEnv(env, "CRANTCLI_VERSION")
	if len(got) != 2 || got[0] != "HOME=/tmp" || got[1] != "PATH=/bin" {
		t.Fatalf("withoutEnv = %v, want CRANTCLI_VERSION removed", got)
	}
}
