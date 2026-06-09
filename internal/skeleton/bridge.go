package skeleton

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"crantcli/internal/httperror"
)

const MissingUVMessage = "uv is required to run the skeleton-view Python bridge.\nInstall uv: https://docs.astral.sh/uv/getting-started/installation/"

type BridgeOptions struct {
	UVPath     string
	RuntimeDir string
	RootID     string
	Token      string
	Server     string
	Datastack  string
}

type BridgeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

func (e BridgeError) Error() string {
	message := httperror.PreviewString(e.Message)
	details := httperror.PreviewString(e.Details)
	if e.Details == "" {
		return fmt.Sprintf("%s: %s", e.Code, message)
	}
	return fmt.Sprintf("%s: %s: %s", e.Code, message, details)
}

type bridgeResponse struct {
	OK       bool         `json:"ok"`
	Skeleton *Skeleton    `json:"skeleton,omitempty"`
	Error    *BridgeError `json:"error,omitempty"`
}

func LookPathUV() (string, error) {
	path, err := exec.LookPath("uv")
	if err != nil {
		return "", fmt.Errorf(MissingUVMessage)
	}
	return path, nil
}

func FetchWithBridge(ctx context.Context, opts BridgeOptions) (*Skeleton, error) {
	if strings.TrimSpace(opts.UVPath) == "" {
		return nil, fmt.Errorf("uv path is empty")
	}
	if strings.TrimSpace(opts.RuntimeDir) == "" {
		return nil, fmt.Errorf("bridge runtime directory is empty")
	}
	if strings.TrimSpace(opts.Token) == "" {
		return nil, fmt.Errorf("CAVE token is empty")
	}
	if _, _, err := ParseRootID(opts.RootID); err != nil {
		return nil, err
	}
	if err := EnsureBridgeRuntime(opts.RuntimeDir); err != nil {
		return nil, err
	}

	args := []string{
		"run",
		"--project", opts.RuntimeDir,
		"python",
		filepath.Join(opts.RuntimeDir, "bridge.py"),
		"--root-id", opts.RootID,
		"--server", opts.Server,
		"--datastack", opts.Datastack,
	}
	cmd := exec.CommandContext(ctx, opts.UVPath, args...)
	cmd.Env = bridgeEnv(opts.Token)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if bridgeErr := decodeBridgeError(stdout.Bytes()); bridgeErr != nil {
			return nil, bridgeErr
		}
		return nil, bridgeCommandError(stderr.String(), err)
	}

	var resp bridgeResponse
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("decoding skeleton bridge output: %w", err)
	}
	if !resp.OK {
		if resp.Error != nil {
			return nil, resp.Error
		}
		return nil, fmt.Errorf("skeleton bridge failed without error details")
	}
	if err := ValidateSkeleton(resp.Skeleton); err != nil {
		return nil, err
	}
	if resp.Skeleton.RootID == "" {
		resp.Skeleton.RootID = opts.RootID
	}
	return resp.Skeleton, nil
}

func decodeBridgeError(data []byte) error {
	var resp bridgeResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil
	}
	if resp.Error != nil {
		resp.Error.Message = httperror.PreviewString(resp.Error.Message)
		resp.Error.Details = httperror.PreviewString(resp.Error.Details)
		return resp.Error
	}
	return nil
}

func bridgeCommandError(stderr string, runErr error) error {
	msg := httperror.PreviewString(stderr)
	if msg == "" {
		msg = runErr.Error()
	}
	return fmt.Errorf("running skeleton bridge: %s", msg)
}

func bridgeEnv(token string) []string {
	env := filterBridgeEnv(os.Environ())
	return append(env, "CAVE_TOKEN="+token)
}

var bridgeAllowedEnv = map[string]bool{
	"ALL_PROXY":          true,
	"APPDATA":            true,
	"COMSPEC":            true,
	"CURL_CA_BUNDLE":     true,
	"HOME":               true,
	"HTTP_PROXY":         true,
	"HTTPS_PROXY":        true,
	"LANG":               true,
	"LANGUAGE":           true,
	"LOCALAPPDATA":       true,
	"LOGNAME":            true,
	"NO_PROXY":           true,
	"PATH":               true,
	"PATHEXT":            true,
	"REQUESTS_CA_BUNDLE": true,
	"SHELL":              true,
	"SSL_CERT_DIR":       true,
	"SSL_CERT_FILE":      true,
	"SYSTEMROOT":         true,
	"TEMP":               true,
	"TMP":                true,
	"TMPDIR":             true,
	"USER":               true,
	"USERNAME":           true,
	"USERPROFILE":        true,
	"UV_CACHE_DIR":       true,
	"UV_CONFIG_FILE":     true,
	"UV_TOOL_DIR":        true,
	"WINDIR":             true,
	"XDG_CACHE_HOME":     true,
	"XDG_CONFIG_HOME":    true,
	"XDG_DATA_HOME":      true,
}

func filterBridgeEnv(base []string) []string {
	env := make([]string, 0, len(base))
	for _, entry := range base {
		name, _, ok := strings.Cut(entry, "=")
		if !ok {
			continue
		}
		upperName := strings.ToUpper(name)
		if bridgeAllowedEnv[upperName] || strings.HasPrefix(upperName, "LC_") {
			env = append(env, entry)
		}
	}
	return env
}

func ParseMaxNodes(raw int) (int, error) {
	if raw < 0 {
		return 0, fmt.Errorf("--max-nodes must be >= 0")
	}
	if raw == 1 {
		return 0, fmt.Errorf("--max-nodes must be 0 or >= 2")
	}
	return raw, nil
}

func FormatNodeCount(nodes int) string {
	return strconv.FormatInt(int64(nodes), 10)
}
