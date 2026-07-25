package procenv

import (
	"strings"
	"testing"
)

func environmentValue(environ []string, key string) (string, bool) {
	prefix := strings.ToUpper(key) + "="
	for _, entry := range environ {
		if strings.HasPrefix(strings.ToUpper(entry), prefix) {
			return entry[len(prefix):], true
		}
	}
	return "", false
}

func TestSanitizedRemovesCredentialVariables(t *testing.T) {
	for _, key := range []string{
		"CRANTTABLE_TOKEN",
		"CRANTTABLE_TOKEN_FILE",
		"CAVE_TOKEN",
		"CAVE_TOKEN_FILE",
		"CRANTCLI_GITHUB_TOKEN",
		"GH_TOKEN",
		"GITHUB_TOKEN",
		"GH_ENTERPRISE_TOKEN",
		"GITHUB_ENTERPRISE_TOKEN",
	} {
		t.Setenv(key, "must-not-leak")
	}
	t.Setenv("CRANTCLI_TEST_VISIBLE", "visible")

	environ := Sanitized()
	for key := range sensitiveKeys {
		if value, ok := environmentValue(environ, key); ok {
			t.Errorf("%s leaked with value %q", key, value)
		}
	}
	if value, ok := environmentValue(environ, "CRANTCLI_TEST_VISIBLE"); !ok || value != "visible" {
		t.Fatalf("ordinary environment variable missing: value=%q, present=%v", value, ok)
	}
}

func TestSanitizedKeepsExplicitExceptions(t *testing.T) {
	t.Setenv("GH_TOKEN", "github-token")
	t.Setenv("CAVE_TOKEN", "cave-token")

	environ := Sanitized("GH_TOKEN")
	if value, ok := environmentValue(environ, "GH_TOKEN"); !ok || value != "github-token" {
		t.Fatalf("GH_TOKEN exception missing: value=%q, present=%v", value, ok)
	}
	if value, ok := environmentValue(environ, "CAVE_TOKEN"); ok {
		t.Fatalf("CAVE_TOKEN leaked with value %q", value)
	}
}
