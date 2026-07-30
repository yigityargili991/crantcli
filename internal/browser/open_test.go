package browser

import "testing"

func TestCappedOpenerOutput(t *testing.T) {
	var output cappedOutput
	payload := make([]byte, maxOpenerOutput+1024)
	if n, err := output.Write(payload); err != nil || n != len(payload) {
		t.Fatalf("Write = (%d, %v), want (%d, nil)", n, err, len(payload))
	}
	if output.Len() != maxOpenerOutput {
		t.Fatalf("buffer length = %d, want %d", output.Len(), maxOpenerOutput)
	}
}

func TestOpenURLRejectsInvalidValues(t *testing.T) {
	for _, value := range []string{"", "not a URL", "file:///tmp/state", "javascript:alert(1)"} {
		if _, err := OpenURLWithResult(value); err == nil {
			t.Errorf("OpenURLWithResult(%q) unexpectedly succeeded", value)
		}
	}
}
