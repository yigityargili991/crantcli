//go:build !windows

package browser

// execve passes each argument to the opener verbatim, so nothing but length
// forces a URL through the redirect file.
func openerArgumentIsSafe(string) bool {
	return true
}
