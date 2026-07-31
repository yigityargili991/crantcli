//go:build windows

package browser

import "fmt"

// CreateProcess caps the entire command line at 32767 characters, which a
// Neuroglancer state exceeds on its own. Leave room for the rundll32 prefix
// and stage anything larger through a redirect file.
const maxSafeOpenArgument = 30 << 10

func platformOpenURL(rawURL string) (OpenResult, error) {
	target, err := prepareCommandOpenURL(rawURL)
	if err != nil {
		return OpenResult{}, fmt.Errorf("preparing browser handoff: %w", err)
	}
	return runPlatformCommand(BackendRundll32, "rundll32", "url.dll,FileProtocolHandler", target)
}
