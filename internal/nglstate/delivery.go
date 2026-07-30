package nglstate

import (
	"errors"
	"fmt"
	"io"
	"os"

	"crantcli/internal/browser"
	"crantcli/internal/clipboard"
)

// DeliveryOptions describes independent destinations for a completed state.
type DeliveryOptions struct {
	OutputFile string
	Open       bool
}

var (
	deliveryClipboardWrite           = clipboard.WriteText
	deliveryBrowserOpen              = browser.OpenURLWithResult
	deliveryStdout         io.Writer = os.Stdout
	deliveryStderr         io.Writer = os.Stderr
)

// DeliverState writes, copies, and opens a state without allowing failure in
// one destination to suppress the others. Requested desktop failures are
// returned after every viable action has been attempted.
func DeliverState(result *LoadResult, options DeliveryOptions) error {
	var failures []error

	needsClipboard := options.OutputFile == "" && outputUsesURL(result.Source)
	needsURL := needsClipboard || options.Open
	var rawURL string
	if needsURL {
		encoded, err := EncodeURL(result.State, "")
		if err != nil {
			failures = append(failures, fmt.Errorf("encoding Neuroglancer URL: %w", err))
		} else {
			rawURL = encoded
			result.OutputURL = encoded
		}
	}

	var clipboardErr error
	switch {
	case options.OutputFile != "":
		if err := writeToFile(result.State, options.OutputFile); err != nil {
			failures = append(failures, err)
		}
	case needsClipboard:
		if rawURL == "" {
			// The encoding failure above already explains why this action cannot run.
			break
		}
		copyResult, err := deliveryClipboardWrite(rawURL)
		if err != nil {
			fmt.Fprintln(deliveryStderr, "Warning: clipboard copy failed; other delivery actions will still be attempted.")
			fmt.Fprintln(deliveryStderr, "The Neuroglancer URL will be printed to standard output instead.")
			clipboardErr = err
		} else {
			fmt.Fprintf(deliveryStderr, "Clipboard: copied Neuroglancer URL via %s\n", copyResult.Backend)
		}
	default:
		if err := writeStateJSON(deliveryStdout, result.State); err != nil {
			failures = append(failures, fmt.Errorf("writing state JSON: %w", err))
		}
	}

	if options.Open && rawURL != "" {
		openResult, err := deliveryBrowserOpen(rawURL)
		if err != nil {
			failures = append(failures, fmt.Errorf("opening browser: %w", err))
		} else {
			fmt.Fprintf(deliveryStderr, "Browser: handed Neuroglancer URL to %s\n", openResult.Backend)
		}
	}

	// Print only after attempting --open, so a blocked or slow stdout consumer
	// cannot prevent the independent browser handoff. A clipboard failure is a
	// warning rather than a failure as long as stdout still received the URL:
	// the command did produce its result, and scripted callers on headless
	// machines should not have to treat that as an error. Only a URL that
	// reached no destination at all is fatal.
	if clipboardErr != nil {
		if _, err := fmt.Fprintln(deliveryStdout, rawURL); err != nil {
			failures = append(failures,
				fmt.Errorf("copying Neuroglancer URL to clipboard: %w", clipboardErr),
				fmt.Errorf("printing fallback URL: %w", err))
		}
	}

	return errors.Join(failures...)
}

func outputUsesURL(source StateSource) bool {
	switch source {
	case SourceClipboard, SourceURL, SourceTemplate:
		return true
	default:
		return false
	}
}
