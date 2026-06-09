package httperror

import (
	"strings"
	"testing"
)

func TestRedactTokenLikeValues(t *testing.T) {
	input := `Authorization: Bearer abc.def_123 {"access_token":"secret","api-token":"api-secret"} token=query-secret&x=1`
	got := Redact(input)
	for _, secret := range []string{"abc.def_123", "secret", "api-secret", "query-secret"} {
		if strings.Contains(got, secret) {
			t.Fatalf("Redact leaked %q in %q", secret, got)
		}
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("Redact = %q, want redaction marker", got)
	}
}

func TestPreviewStringRedactsAndTruncates(t *testing.T) {
	got := PreviewString("token=secret " + strings.Repeat("x", PreviewLimit+20))
	if strings.Contains(got, "secret") {
		t.Fatalf("PreviewString leaked token: %q", got)
	}
	if !strings.Contains(got, "(truncated)") {
		t.Fatalf("PreviewString = %q, want truncation marker", got)
	}
}
