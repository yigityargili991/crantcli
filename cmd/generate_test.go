package cmd

import (
	"strings"
	"testing"
)

func TestGenerateHelpMatchesStdoutBehavior(t *testing.T) {
	if !strings.Contains(generateCmd.Long, "stdout") {
		t.Fatalf("generate long help = %q, want stdout mention", generateCmd.Long)
	}
	if strings.Contains(strings.ToLower(generateCmd.Long), "clipboard") {
		t.Fatalf("generate long help = %q, should not mention clipboard output", generateCmd.Long)
	}
}
