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
	"golang.org/x/mod/semver"
)

const (
	updateRepoSlug    = "yigityargili991/crantcli"
	updateAssetPrefix = "crant_type_look"
	updateLatestURL   = "https://api.github.com/repos/" + updateRepoSlug + "/releases/latest"
	updateRawURL      = "https://raw.githubusercontent.com/" + updateRepoSlug + "/"
)

var errReleaseNotReady = errors.New("latest release is not ready")

// Test seams: platform, HTTP fetches, process launches, and the executable
// location are replaceable so update behavior can be exercised without
// network access or a real installer.
var (
	updateGOOS       = runtime.GOOS
	updateGOARCH     = runtime.GOARCH
	updateFetch      = fetchURL
	updateStart      = startProcess
	updateExecutable = os.Executable
	updateInvocation = invokedExecutable
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
	// logic always matches the target binary.
	latest, err := latestReleaseTag()
	if err != nil {
		return fmt.Errorf("check latest release: %w", err)
	}
	if isUpToDate(Version, latest) {
		fmt.Fprintf(out, "crantcli is already up to date (%s)\n", latest)
		return nil
	}

	installDir, err := runningInstallDir()
	if err != nil {
		return fmt.Errorf("locate running installation: %w", err)
	}

	scriptName := installerScriptName(updateGOOS)
	scriptPath, err := downloadInstaller(updateRawURL+latest+"/"+scriptName, scriptName)
	if err != nil {
		return fmt.Errorf("download %s: %w", scriptName, err)
	}

	name, args := installerCommand(updateGOOS, scriptPath)
	fmt.Fprintf(out, "Updating crantcli %s -> %s\n", Version, latest)
	if err := updateStart(name, args, installerEnv(os.Environ(), installDir, latest)); err != nil {
		return fmt.Errorf("launch installer: %w", err)
	}
	fmt.Fprintln(out, "Installer launched; it will replace the crantcli binary after this process exits.")
	return nil
}

// isUpToDate reports whether the running build is at least as new as the
// latest release tag. Development builds ("dev" or unset) always update.
func isUpToDate(current, latest string) bool {
	if current == "" || current == "dev" {
		return false
	}
	current = canonicalSemver(current)
	latest = canonicalSemver(latest)
	if semver.IsValid(current) && semver.IsValid(latest) {
		return semver.Compare(current, latest) >= 0
	}
	return strings.TrimPrefix(current, "v") == strings.TrimPrefix(latest, "v")
}

func canonicalSemver(version string) string {
	version = strings.TrimSpace(version)
	if !strings.HasPrefix(version, "v") {
		version = "v" + version
	}
	return version
}

func latestReleaseTag() (string, error) {
	body, err := updateFetch(updateLatestURL)
	if err != nil {
		return "", err
	}
	var payload struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name string `json:"name"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("parse latest release response: %w", err)
	}
	if payload.TagName == "" {
		return "", errors.New("latest release response has no tag_name")
	}
	required := []string{"checksums.txt", releaseAssetName(updateGOOS, updateGOARCH)}
	available := make(map[string]bool, len(payload.Assets))
	for _, asset := range payload.Assets {
		available[asset.Name] = true
	}
	var missing []string
	for _, name := range required {
		if !available[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) != 0 {
		return payload.TagName, fmt.Errorf("%w: %s is missing %s", errReleaseNotReady, payload.TagName, strings.Join(missing, ", "))
	}
	return payload.TagName, nil
}

func releaseAssetName(goos, goarch string) string {
	ext := ""
	if goos == "windows" {
		ext = ".exe"
	}
	return fmt.Sprintf("%s-%s-%s%s", updateAssetPrefix, goos, goarch, ext)
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

// runningInstallDir returns a directory where the platform installer will
// replace the executable that launched the update. A canonical invocation
// path takes precedence over its resolved target when both identify the same
// file, allowing a canonical symlink to a historically named release asset to
// be replaced. Renamed binaries without such a launcher cannot be updated in
// place because the installers always write the canonical filename.
func runningInstallDir() (string, error) {
	exe, err := updateExecutable()
	if err != nil {
		return "", err
	}
	wantName := "crantcli"
	if updateGOOS == "windows" {
		wantName += ".exe"
	}
	if invoked, err := updateInvocation(); err == nil &&
		executableNameMatches(filepath.Base(invoked), wantName) &&
		sameExecutable(invoked, exe) {
		return filepath.Dir(invoked), nil
	}
	if executableNameMatches(filepath.Base(exe), wantName) {
		return filepath.Dir(exe), nil
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		if executableNameMatches(filepath.Base(resolved), wantName) {
			return filepath.Dir(resolved), nil
		}
	}
	return "", fmt.Errorf("running executable %q is not named %q", exe, wantName)
}

func invokedExecutable() (string, error) {
	if len(os.Args) == 0 || os.Args[0] == "" {
		return "", errors.New("process invocation path is unavailable")
	}
	path, err := exec.LookPath(os.Args[0])
	if err != nil {
		return "", err
	}
	return filepath.Abs(path)
}

func sameExecutable(first, second string) bool {
	firstInfo, err := os.Stat(first)
	if err != nil {
		return false
	}
	secondInfo, err := os.Stat(second)
	return err == nil && os.SameFile(firstInfo, secondInfo)
}

func executableNameMatches(name, want string) bool {
	if updateGOOS == "windows" {
		return strings.EqualFold(name, want)
	}
	return name == want
}

// installerEnv builds the installer environment. Any inherited
// CRANTCLI_VERSION is replaced with the resolved release tag.
// CRANTCLI_INSTALL_DIR defaults to the running executable's directory so the
// update targets the current installation even when it sits in a custom
// directory the installer cannot infer on its own (e.g. a one-shot
// /usr/local/bin install). An explicitly exported CRANTCLI_INSTALL_DIR wins.
func installerEnv(env []string, installDir, version string) []string {
	filtered := withoutEnv(env, "CRANTCLI_VERSION")
	if version != "" {
		filtered = append(filtered, "CRANTCLI_VERSION="+version)
	}
	if installDir == "" || envHas(env, "CRANTCLI_INSTALL_DIR") {
		return filtered
	}
	filtered = withoutEnv(filtered, "CRANTCLI_INSTALL_DIR")
	return append(filtered, "CRANTCLI_INSTALL_DIR="+installDir)
}

func withoutEnv(env []string, key string) []string {
	filtered := make([]string, 0, len(env))
	for _, entry := range env {
		if !envKeyMatches(entry, key) {
			filtered = append(filtered, entry)
		}
	}
	return filtered
}

func envHas(env []string, key string) bool {
	for _, entry := range env {
		if !envKeyMatches(entry, key) {
			continue
		}
		_, value, _ := strings.Cut(entry, "=")
		if strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func envKeyMatches(entry, key string) bool {
	name, _, ok := strings.Cut(entry, "=")
	if !ok {
		return false
	}
	if updateGOOS == "windows" {
		return strings.EqualFold(name, key)
	}
	return name == key
}
