package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyReleaseArtifactRejectsMalformedBundle(t *testing.T) {
	bundlePath := filepath.Join(t.TempDir(), "release.bundle.json")
	if err := os.WriteFile(bundlePath, []byte("{"), 0o600); err != nil {
		t.Fatalf("write malformed bundle: %v", err)
	}

	err := verifyReleaseArtifact("artifact", bundlePath)
	if err == nil || !strings.Contains(err.Error(), "load Sigstore bundle") {
		t.Fatalf("verifyReleaseArtifact error = %v, want malformed-bundle rejection", err)
	}
}

func TestVerifyReleaseCommandIsHidden(t *testing.T) {
	if !verifyReleaseCmd.Hidden {
		t.Fatal("internal release verifier is visible in command help")
	}
}
