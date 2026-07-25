package browser

import (
	"fmt"
	"os/exec"
	"runtime"
)

// OpenURL opens a URL in the system default browser.
func OpenURL(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("URL is empty")
	}

	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", rawURL)
	case "linux":
		cmd = exec.Command("xdg-open", rawURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	default:
		return fmt.Errorf("opening browser is not supported on %s", runtime.GOOS)
	}

	// Run (not Start) so the helper is reaped and launch errors surface; the
	// platform openers return immediately after handing off to the browser.
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("starting browser command: %w", err)
	}
	return nil
}
