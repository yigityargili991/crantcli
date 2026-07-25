// Package procenv prepares subprocess environments without leaking credentials
// that belong to crantcli or its authenticated tooling.
package procenv

import (
	"os"
	"strings"
)

var sensitiveKeys = map[string]struct{}{
	"CRANTTABLE_TOKEN":        {},
	"CRANTTABLE_TOKEN_FILE":   {},
	"CAVE_TOKEN":              {},
	"CAVE_TOKEN_FILE":         {},
	"CRANTCLI_GITHUB_TOKEN":   {},
	"GH_TOKEN":                {},
	"GITHUB_TOKEN":            {},
	"GH_ENTERPRISE_TOKEN":     {},
	"GITHUB_ENTERPRISE_TOKEN": {},
}

// Sanitized returns the current environment without known credential
// variables. Named exceptions are retained for the subprocesses that
// intentionally consume them.
func Sanitized(allowed ...string) []string {
	allowedKeys := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		allowedKeys[strings.ToUpper(key)] = struct{}{}
	}

	environ := os.Environ()
	filtered := make([]string, 0, len(environ))
	for _, entry := range environ {
		key, _, _ := strings.Cut(entry, "=")
		key = strings.ToUpper(key)
		if _, sensitive := sensitiveKeys[key]; sensitive {
			if _, allowed := allowedKeys[key]; !allowed {
				continue
			}
		}
		filtered = append(filtered, entry)
	}
	return filtered
}
