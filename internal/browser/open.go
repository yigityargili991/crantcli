package browser

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"strings"
	"time"

	"crantcli/internal/procenv"
)

// Backend identifies the desktop mechanism that accepted an open request.
type Backend string

const (
	BackendXDGPortal Backend = "XDG desktop portal"
	BackendXDGOpen   Backend = "xdg-open"
	BackendGIO       Backend = "gio"
	BackendMacOpen   Backend = "open"
	BackendRundll32  Backend = "rundll32"
)

// OpenResult identifies how a URL was handed to the desktop.
type OpenResult struct {
	Backend Backend
}

const (
	openerTimeout   = 15 * time.Second
	maxOpenerOutput = 64 << 10
)

type cappedOutput struct {
	bytes.Buffer
}

func (w *cappedOutput) Write(p []byte) (int, error) {
	remaining := maxOpenerOutput - w.Len()
	if remaining > 0 {
		if remaining > len(p) {
			remaining = len(p)
		}
		_, _ = w.Buffer.Write(p[:remaining])
	}
	// Report the full input as consumed so a noisy opener is discarded rather
	// than terminated with an unrelated short-write error.
	return len(p), nil
}

var runPlatformCommand = runOpenCommand

// OpenURL opens an HTTP(S) URL in the system default browser.
func OpenURL(rawURL string) error {
	_, err := OpenURLWithResult(rawURL)
	return err
}

// OpenURLWithResult opens rawURL and reports which platform backend accepted
// it. A successful result means the desktop accepted the handoff; compositors
// and browsers retain control over window placement and focus.
func OpenURLWithResult(rawURL string) (OpenResult, error) {
	if rawURL == "" {
		return OpenResult{}, fmt.Errorf("URL is empty")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return OpenResult{}, fmt.Errorf("invalid browser URL")
	}
	return platformOpenURL(rawURL)
}

func runOpenCommand(backend Backend, name string, args ...string) (OpenResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), openerTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = procenv.Sanitized()
	var output cappedOutput
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return OpenResult{}, fmt.Errorf("%s timed out after %s", name, openerTimeout)
		}
		message := strings.TrimSpace(output.String())
		if len(message) > 4096 {
			message = message[:4096] + "…"
		}
		if message != "" {
			return OpenResult{}, fmt.Errorf("%s: %w: %s", name, err, message)
		}
		return OpenResult{}, fmt.Errorf("%s: %w", name, err)
	}
	return OpenResult{Backend: backend}, nil
}
