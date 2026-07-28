package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"crantcli/internal/httpx"

	"github.com/spf13/cobra"
)

const (
	updateRepoSlug  = "yigityargili991/crantcli"
	updateLatestURL = "https://api.github.com/repos/" + updateRepoSlug + "/releases/latest"
	updateRawURL    = "https://raw.githubusercontent.com/" + updateRepoSlug + "/"
)

// Test seams: platform, HTTP fetches, process launches, and the executable
// location are replaceable so update behavior can be exercised without
// network access or a real installer.
var (
	updateGOOS       = runtime.GOOS
	updateFetch      = fetchURL
	updateStart      = startProcess
	updateExecutable = os.Executable
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update crantcli to the latest release",
	Long: `Check for a newer crantcli release and update in place.

The update re-runs the platform installer (install.sh on macOS/Linux,
install.ps1 on Windows) for the latest GitHub release, including its checksum
and signature verification. crantcli exits while the installer replaces the
binary, so the new version is available the next time you run crantcli.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, _ []string) error {
		return runUpdate(cmd)
	},
}

func init() {
	updateCmd.ValidArgsFunction = noFileCompletion
	rootCmd.AddCommand(updateCmd)
}

func runUpdate(cmd *cobra.Command) error {
	out := cmd.OutOrStdout()

	// The installer script is taken from the release being installed so its
	// logic always matches the target binary; without a resolved tag we fall
	// back to the copy on the default branch.
	ref := "main"
	target := "latest"
	latest, err := latestReleaseTag()
	switch {
	case err != nil:
		fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not check the latest release (%v); updating anyway\n", err)
	case isUpToDate(Version, latest):
		fmt.Fprintf(out, "crantcli is already up to date (%s)\n", latest)
		return nil
	default:
		ref = latest
		target = latest
	}

	scriptName := installerScriptName(updateGOOS)
	scriptPath, err := downloadInstaller(updateRawURL+ref+"/"+scriptName, scriptName)
	if err != nil {
		return fmt.Errorf("download %s: %w", scriptName, err)
	}

	installDir := runningInstallDir()
	if installDir == "" {
		fmt.Fprintln(cmd.ErrOrStderr(), "warning: could not determine the running installation directory; the installer will use its default")
	}

	name, args := installerCommand(updateGOOS, scriptPath)
	fmt.Fprintf(out, "Updating crantcli %s -> %s\n", Version, target)
	if err := updateStart(name, args, installerEnv(os.Environ(), installDir)); err != nil {
		return fmt.Errorf("launch installer: %w", err)
	}
	fmt.Fprintln(out, "Installer launched; it will replace the crantcli binary after this process exits.")
	return nil
}

// isUpToDate reports whether the running build already matches the latest
// release tag. Development builds ("dev" or unset) always update.
func isUpToDate(current, latest string) bool {
	if current == "" || current == "dev" {
		return false
	}
	return strings.TrimPrefix(strings.TrimSpace(current), "v") ==
		strings.TrimPrefix(strings.TrimSpace(latest), "v")
}

func latestReleaseTag() (string, error) {
	body, err := updateFetch(updateLatestURL)
	if err != nil {
		return "", err
	}
	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("parse latest release response: %w", err)
	}
	if payload.TagName == "" {
		return "", errors.New("latest release response has no tag_name")
	}
	return payload.TagName, nil
}

func installerScriptName(goos string) string {
	if goos == "windows" {
		return "install.ps1"
	}
	return "install.sh"
}

// installerCommand returns the command that runs the downloaded installer.
// The process is started detached (never waited on) so crantcli can exit
// before the binary is replaced; on Windows the brief sleep gives this
// process time to exit, since a running .exe cannot be overwritten.
func installerCommand(goos, scriptPath string) (string, []string) {
	if goos == "windows" {
		return "powershell", []string{
			"-NoProfile", "-ExecutionPolicy", "Bypass", "-Command",
			"Start-Sleep -Seconds 2; & '" + strings.ReplaceAll(scriptPath, "'", "''") + "'",
		}
	}
	return "sh", []string{scriptPath}
}

// downloadInstaller fetches the installer script into a temp file. The file
// is deliberately left behind: the installer runs after crantcli exits, so
// only the OS can clean it up.
func downloadInstaller(url, scriptName string) (string, error) {
	body, err := updateFetch(url)
	if err != nil {
		return "", err
	}
	f, err := os.CreateTemp("", "crantcli-update-*"+filepath.Ext(scriptName))
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.Write(body); err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

func fetchURL(url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	// api.github.com rejects requests without a User-Agent.
	req.Header.Set("User-Agent", "crantcli-update")
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := httpx.Do(httpx.DefaultClient, req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, httpx.MaxResponseBody))
}

// startProcess launches the installer without waiting for it: crantcli must
// exit before the installer replaces the binary.
func startProcess(name string, args []string, env []string) error {
	proc := exec.Command(name, args...)
	proc.Stdout = os.Stdout
	proc.Stderr = os.Stderr
	proc.Env = env
	return proc.Start()
}

// runningInstallDir returns the directory of the running executable so the
// update can replace the installation that launched it. Symlinks are
// resolved: replacing the real file keeps launchers pointing at it working.
// Empty when the location cannot be determined.
func runningInstallDir() string {
	exe, err := updateExecutable()
	if err != nil {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return filepath.Dir(exe)
}

// installerEnv builds the installer environment. A pinned CRANTCLI_VERSION is
// removed (it would install that version instead of the latest), and
// CRANTCLI_INSTALL_DIR defaults to the running executable's directory so the
// update targets the current installation even when it sits in a custom
// directory the installer cannot infer on its own (e.g. a one-shot
// /usr/local/bin install). An explicitly exported CRANTCLI_INSTALL_DIR wins.
func installerEnv(env []string, installDir string) []string {
	filtered := withoutEnv(env, "CRANTCLI_VERSION")
	if installDir == "" || envHas(env, "CRANTCLI_INSTALL_DIR") {
		return filtered
	}
	return append(filtered, "CRANTCLI_INSTALL_DIR="+installDir)
}

func withoutEnv(env []string, key string) []string {
	prefix := key + "="
	filtered := make([]string, 0, len(env))
	for _, entry := range env {
		if !strings.HasPrefix(entry, prefix) {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func envHas(env []string, key string) bool {
	prefix := key + "="
	for _, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			return true
		}
	}
	return false
}
