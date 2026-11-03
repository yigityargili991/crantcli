// Copyright 2026 Yigit Yargili
// SPDX-License-Identifier: Apache-2.0

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
	"crantcli/internal/procenv"

	"github.com/spf13/cobra"
	"golang.org/x/mod/semver"
)

const (
	updateRepoSlug            = "yigityargili991/crantcli"
	updateAssetPrefix         = "crant_type_look"
	updateReleasesURL         = "https://api.github.com/repos/" + updateRepoSlug + "/releases?per_page=100"
	updateReleaseDownloadURL  = "https://github.com/" + updateRepoSlug + "/releases/download/"
	updateReleasePageURL      = "https://github.com/" + updateRepoSlug + "/releases/tag/"
	updateChangelogURL        = "https://raw.githubusercontent.com/" + updateRepoSlug + "/"
	updateCertificateIssuer   = "https://token.actions.githubusercontent.com"
	updateCertificateIdentity = `^https://github\.com/yigityargili991/crantcli/\.github/workflows/release\.yml@refs/tags/v[^/]+$`
	updateBundleSuffix        = ".bundle.json"
)

var errReleaseNotReady = errors.New("latest release is not ready")

// Test seams: platform, HTTP fetches, process launches, and the executable
// location are replaceable so update behavior can be exercised without
// network access or a real installer.
var (
	updateGOOS       = runtime.GOOS
	updateGOARCH     = runtime.GOARCH
	updateFetch      = fetchURL
	updateVerify     = verifyReleaseArtifact
	updateRun        = runProcess
	updateExecutable = os.Executable
	updateInvocation = invokedExecutable
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update crantcli to the newest release",
	Long: `Check for a newer crantcli release, release candidates included, and
update in place.

The update authenticates the release's platform installer with Sigstore, then
runs it to verify and install the newest binary. It returns only after the
binary has been atomically replaced. Signature verification is built in; no
separate cosign installation is required.`,
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
	verifierPath, err := updateExecutable()
	if err != nil {
		return fmt.Errorf("locate update verifier: %w", err)
	}

	scriptName := installerScriptName(updateGOOS)
	releaseURL := updateReleaseDownloadURL + latest + "/"
	scriptPath, err := downloadInstaller(releaseURL+scriptName, scriptName)
	if err != nil {
		return fmt.Errorf("download %s: %w", scriptName, err)
	}
	defer os.Remove(scriptPath)
	bundleName := scriptName + updateBundleSuffix
	bundlePath, err := downloadInstaller(releaseURL+bundleName, bundleName)
	if err != nil {
		return fmt.Errorf("download %s: %w", bundleName, err)
	}
	defer os.Remove(bundlePath)
	if err := updateVerify(scriptPath, bundlePath); err != nil {
		return fmt.Errorf("verify %s: %w", scriptName, err)
	}

	name, args := installerCommand(updateGOOS, scriptPath)
	// The installer downloads with CRANTCLI_GITHUB_TOKEN and needs no other
	// credential, so the CRANT, CAVE, and other GitHub tokens stay behind.
	env := installerEnv(procenv.Sanitized("CRANTCLI_GITHUB_TOKEN"), installDir, latest, verifierPath)
	fmt.Fprintf(out, "Updating crantcli %s -> %s\n", Version, latest)
	if err := updateRun(name, args, env); err != nil {
		return fmt.Errorf("run installer: %w", err)
	}
	fmt.Fprintf(out, "Updated crantcli to %s\n", latest)
	printChangelog(out, Version, latest)
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

// latestReleaseTag returns the newest published release by semver, release
// candidates included, so v0.22.4 outranks v0.22.4-rc1 and both outrank
// v0.22.3. Tags that are not semver, such as the raider model's raider-vN
// pre-releases, carry no crantcli binaries and are skipped.
func latestReleaseTag() (string, error) {
	body, err := updateFetch(updateReleasesURL)
	if err != nil {
		return "", err
	}
	var releases []struct {
		TagName string `json:"tag_name"`
		Draft   bool   `json:"draft"`
		Assets  []struct {
			Name string `json:"name"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(body, &releases); err != nil {
		return "", fmt.Errorf("parse release list response: %w", err)
	}
	newest := -1
	for i, release := range releases {
		if release.Draft || !semver.IsValid(release.TagName) {
			continue
		}
		if newest < 0 || semver.Compare(release.TagName, releases[newest].TagName) > 0 {
			newest = i
		}
	}
	if newest < 0 {
		return "", errors.New("release list response has no crantcli release")
	}
	payload := releases[newest]
	scriptName := installerScriptName(updateGOOS)
	binaryName := releaseAssetName(updateGOOS, updateGOARCH)
	required := []string{
		"checksums.txt",
		binaryName,
		binaryName + updateBundleSuffix,
		scriptName,
		scriptName + updateBundleSuffix,
	}
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

// printChangelog prints the CHANGELOG.md sections released after from, up to
// and including tag, read from the changelog at tag. The update has already
// succeeded, so a changelog that cannot be read or has no such section only
// points at the release page.
func printChangelog(out io.Writer, from, tag string) {
	var sections []string
	if body, err := updateFetch(updateChangelogURL + tag + "/CHANGELOG.md"); err == nil {
		sections = changelogSections(string(body), canonicalSemver(from), tag)
	}
	if len(sections) == 0 {
		fmt.Fprintf(out, "Release notes: %s%s\n", updateReleasePageURL, tag)
		return
	}
	fmt.Fprintf(out, "\n%s\n", strings.Join(sections, "\n\n"))
}

// changelogSections returns the "## " sections of changelog whose version is
// newer than from and at most to. A release candidate's changes are still
// listed under "Unreleased", so that section counts when to is a
// pre-release. When from is not a version, as for a development build, only
// to's own section counts.
func changelogSections(changelog, from, to string) []string {
	var sections []string
	var current []string
	keep := false
	flush := func() {
		if keep {
			sections = append(sections, strings.TrimSpace(strings.Join(current, "\n")))
		}
	}
	for _, line := range strings.Split(strings.ReplaceAll(changelog, "\r\n", "\n"), "\n") {
		if !strings.HasPrefix(line, "## ") {
			current = append(current, line)
			continue
		}
		flush()
		current = []string{line}
		version, _, _ := strings.Cut(strings.TrimPrefix(line, "## "), " ")
		switch {
		case version == "Unreleased":
			keep = semver.Prerelease(to) != ""
		case !semver.IsValid(version):
			keep = false
		case !semver.IsValid(from):
			keep = semver.Compare(version, to) == 0
		default:
			keep = semver.Compare(version, from) > 0 && semver.Compare(version, to) <= 0
		}
	}
	flush()
	return sections
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
func installerCommand(goos, scriptPath string) (string, []string) {
	if goos == "windows" {
		return "powershell", []string{
			"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", scriptPath,
		}
	}
	return "sh", []string{scriptPath}
}

// downloadInstaller fetches the installer script into a temporary file.
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

func runProcess(name string, args []string, env []string) error {
	proc := exec.Command(name, args...)
	proc.Stdout = os.Stdout
	proc.Stderr = os.Stderr
	proc.Env = env
	return proc.Run()
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
// CRANTCLI_VERSION is replaced with the resolved release tag. Update mode
// requires the installer to authenticate the binary signature with the
// running crantcli binary rather than falling back to the mutable checksum
// manifest or relying on an external cosign installation.
// CRANTCLI_INSTALL_DIR defaults to the running executable's directory so the
// update targets the current installation even when it sits in a custom
// directory the installer cannot infer on its own (e.g. a one-shot
// /usr/local/bin install). An explicitly exported CRANTCLI_INSTALL_DIR wins.
// Windows passes the updater's PID so backup cleanup can wait for its exit.
func installerEnv(env []string, installDir, version, verifierPath string) []string {
	filtered := withoutEnv(env, "CRANTCLI_VERSION")
	filtered = withoutEnv(filtered, "CRANTCLI_REQUIRE_SIGNATURE")
	filtered = withoutEnv(filtered, "CRANTCLI_VERIFY_BINARY")
	if updateGOOS == "windows" {
		filtered = withoutEnv(filtered, "CRANTCLI_UPDATE_PID")
		filtered = append(filtered, fmt.Sprintf("CRANTCLI_UPDATE_PID=%d", os.Getpid()))
	}
	filtered = append(filtered, "CRANTCLI_REQUIRE_SIGNATURE=1")
	if verifierPath != "" {
		filtered = append(filtered, "CRANTCLI_VERIFY_BINARY="+verifierPath)
	}
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
