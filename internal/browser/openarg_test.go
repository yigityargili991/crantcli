package browser

import (
	"runtime"
	"testing"
)

func TestOpenerArgumentIsSafe(t *testing.T) {
	plain := "https://example.org/#!%7B%7D"
	if !openerArgumentIsSafe(plain) {
		t.Errorf("openerArgumentIsSafe(%q) = false, want true on every platform", plain)
	}

	// Go escapes a '"' into '\"' when it builds the Windows command line, and
	// rundll32 passes that escaping straight to the browser.
	state := "https://example.org/#!{\"layers\":[]}"
	want := runtime.GOOS != "windows"
	if got := openerArgumentIsSafe(state); got != want {
		t.Errorf("openerArgumentIsSafe(%q) = %v, want %v on %s", state, got, want, runtime.GOOS)
	}
}
