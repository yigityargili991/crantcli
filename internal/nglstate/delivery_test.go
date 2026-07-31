package nglstate

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"crantcli/internal/browser"
	"crantcli/internal/clipboard"
)

func isolateDelivery(t *testing.T) (*bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	previousClipboard := deliveryClipboardWrite
	previousBrowser := deliveryBrowserOpen
	previousStdout := deliveryStdout
	previousStderr := deliveryStderr
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	deliveryStdout = stdout
	deliveryStderr = stderr
	t.Cleanup(func() {
		deliveryClipboardWrite = previousClipboard
		deliveryBrowserOpen = previousBrowser
		deliveryStdout = previousStdout
		deliveryStderr = previousStderr
	})
	return stdout, stderr
}

func deliveryFixture(source StateSource) *LoadResult {
	return &LoadResult{State: map[string]interface{}{"layers": []interface{}{}}, Source: source}
}

func TestDeliverStateCopiesAndOpensSameURL(t *testing.T) {
	stdout, stderr := isolateDelivery(t)
	var copied, opened string
	deliveryClipboardWrite = func(value string) (clipboard.WriteResult, error) {
		copied = value
		return clipboard.WriteResult{Backend: clipboard.BackendBuiltInLinux}, nil
	}
	deliveryBrowserOpen = func(value string) (browser.OpenResult, error) {
		opened = value
		return browser.OpenResult{Backend: browser.BackendXDGPortal}, nil
	}

	if err := DeliverState(deliveryFixture(SourceTemplate), DeliveryOptions{Open: true}); err != nil {
		t.Fatalf("DeliverState: %v", err)
	}
	if copied == "" || copied != opened {
		t.Fatalf("clipboard and browser received different URLs: copied=%q opened=%q", copied, opened)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty after clipboard success", stdout.String())
	}
	if !strings.Contains(stderr.String(), "built-in Linux") || !strings.Contains(stderr.String(), "XDG desktop portal") {
		t.Fatalf("status output missing backends: %q", stderr.String())
	}
}

type orderedWriter struct {
	order *[]string
	buf   bytes.Buffer
}

func (w *orderedWriter) Write(p []byte) (int, error) {
	*w.order = append(*w.order, "fallback")
	return w.buf.Write(p)
}

func TestDeliverStateClipboardFailureStillOpensBeforeFallback(t *testing.T) {
	_, stderr := isolateDelivery(t)
	var order []string
	var copyAttempt string
	fallback := &orderedWriter{order: &order}
	deliveryStdout = fallback
	deliveryClipboardWrite = func(value string) (clipboard.WriteResult, error) {
		order = append(order, "copy")
		copyAttempt = value
		return clipboard.WriteResult{}, errors.New("no clipboard")
	}
	deliveryBrowserOpen = func(string) (browser.OpenResult, error) {
		order = append(order, "open")
		return browser.OpenResult{Backend: browser.BackendXDGOpen}, nil
	}

	// stdout still received the URL, so the command succeeded: scripted callers
	// on headless machines must not have to treat a missing clipboard as an error.
	if err := DeliverState(deliveryFixture(SourceTemplate), DeliveryOptions{Open: true}); err != nil {
		t.Fatalf("DeliverState error = %v, want success once the URL reached stdout", err)
	}
	if got := strings.Join(order, ","); got != "copy,open,fallback" {
		t.Fatalf("action order = %q, want copy,open,fallback", got)
	}
	if copyAttempt == "" || strings.TrimSpace(fallback.buf.String()) != copyAttempt {
		t.Fatalf("fallback = %q, want the clipboard-attempted URL %q", fallback.buf.String(), copyAttempt)
	}
	if !strings.Contains(stderr.String(), "clipboard copy failed") {
		t.Fatalf("stderr = %q, want explicit clipboard warning", stderr.String())
	}
}

func TestDeliverStateStdinJSONAndOpenAreIndependent(t *testing.T) {
	stdout, _ := isolateDelivery(t)
	clipboardCalled := false
	opened := ""
	deliveryClipboardWrite = func(string) (clipboard.WriteResult, error) {
		clipboardCalled = true
		return clipboard.WriteResult{}, nil
	}
	deliveryBrowserOpen = func(value string) (browser.OpenResult, error) {
		opened = value
		return browser.OpenResult{Backend: browser.BackendGIO}, nil
	}

	err := DeliverState(deliveryFixture(SourceStdin), DeliveryOptions{Open: true})
	if err != nil {
		t.Fatalf("DeliverState: %v", err)
	}
	if clipboardCalled {
		t.Fatal("stdin-sourced state unexpectedly used clipboard output")
	}
	if opened == "" {
		t.Fatal("browser was not attempted")
	}
	if !strings.Contains(stdout.String(), `"layers"`) {
		t.Fatalf("stdout does not contain state JSON: %q", stdout.String())
	}
}

func TestDeliverStateOutputFailureDoesNotSuppressOpen(t *testing.T) {
	_, _ = isolateDelivery(t)
	opened := false
	deliveryBrowserOpen = func(string) (browser.OpenResult, error) {
		opened = true
		return browser.OpenResult{Backend: browser.BackendXDGPortal}, nil
	}

	err := DeliverState(deliveryFixture(SourceTemplate), DeliveryOptions{
		OutputFile: "/path/that/does/not/exist/state.json",
		Open:       true,
	})
	if err == nil {
		t.Fatal("DeliverState unexpectedly ignored output-file failure")
	}
	if !opened {
		t.Fatal("output-file failure suppressed browser handoff")
	}
}

var _ io.Writer = (*orderedWriter)(nil)

type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

func TestDeliverStateFailsWhenNoDestinationReceivesURL(t *testing.T) {
	_, stderr := isolateDelivery(t)
	deliveryStdout = failingWriter{err: errors.New("broken pipe")}
	deliveryClipboardWrite = func(string) (clipboard.WriteResult, error) {
		return clipboard.WriteResult{}, errors.New("no clipboard")
	}

	err := DeliverState(deliveryFixture(SourceTemplate), DeliveryOptions{})
	if err == nil {
		t.Fatal("DeliverState succeeded with no reachable destination")
	}
	for _, want := range []string{"copying Neuroglancer URL", "printing fallback URL"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
	if !strings.Contains(stderr.String(), "clipboard copy failed") {
		t.Errorf("stderr = %q, want the clipboard warning", stderr.String())
	}
}

func TestDeliverStateReportsEncodingFailure(t *testing.T) {
	_, _ = isolateDelivery(t)
	result := &LoadResult{
		State:  map[string]interface{}{"unsupported": make(chan struct{})},
		Source: SourceTemplate,
	}
	err := DeliverState(result, DeliveryOptions{Open: true})
	if err == nil || !strings.Contains(err.Error(), "encoding Neuroglancer URL") {
		t.Fatalf("error = %v", err)
	}
}

func TestDeliverStateReportsJSONOutputFailure(t *testing.T) {
	_, _ = isolateDelivery(t)
	deliveryStdout = failingWriter{err: errors.New("broken stdout")}
	err := DeliverState(deliveryFixture(SourceStdin), DeliveryOptions{})
	if err == nil || !strings.Contains(err.Error(), "writing state JSON") {
		t.Fatalf("error = %v", err)
	}
}

func TestDeliverStateReportsBrowserFailure(t *testing.T) {
	_, _ = isolateDelivery(t)
	deliveryBrowserOpen = func(string) (browser.OpenResult, error) {
		return browser.OpenResult{}, errors.New("desktop unavailable")
	}
	err := DeliverState(deliveryFixture(SourceStdin), DeliveryOptions{Open: true})
	if err == nil || !strings.Contains(err.Error(), "opening browser") {
		t.Fatalf("error = %v", err)
	}
}
