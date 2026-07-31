//go:build darwin

package browser

import "fmt"

// ARG_MAX is 1 MiB on macOS and covers argv plus environment, so leave room
// for both and stage anything larger through a redirect file.
const maxSafeOpenArgument = 200 << 10

func platformOpenURL(rawURL string) (OpenResult, error) {
	target, err := prepareCommandOpenURL(rawURL)
	if err != nil {
		return OpenResult{}, fmt.Errorf("preparing browser handoff: %w", err)
	}
	return runPlatformCommand(BackendMacOpen, "open", target)
}
