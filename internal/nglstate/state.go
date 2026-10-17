package nglstate

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"crantcli/internal/clipboard"
)

// StateSource describes where the state was loaded from.
type StateSource string

// maxStateBytes bounds state documents read from stdin and files; legitimate
// Neuroglancer scenes are a few MiB at most.
const maxStateBytes = 64 << 20

var (
	stateClipboardRead           = clipboard.Read
	stateWarningWriter io.Writer = os.Stderr
)

const (
	SourceURL       StateSource = "url"
	SourceFile      StateSource = "file"
	SourceStdin     StateSource = "stdin"
	SourceClipboard StateSource = "clipboard"
	SourceTemplate  StateSource = "template"
)

// LoadResult holds a loaded state and its source.
type LoadResult struct {
	State  map[string]interface{}
	Source StateSource
	// OriginalURL is set when the state was decoded from a URL.
	OriginalURL string
	// OutputURL is set when state output is written as a Neuroglancer URL.
	OutputURL string
}

// errNoInputState reports that neither --state, stdin, nor the clipboard
// supplied a state.
var errNoInputState = errors.New("no state given: pass --state, pipe state JSON on stdin, or copy a Neuroglancer URL")

// LoadState loads a Neuroglancer state using smart resolution:
// 1. If stateArg is an existing file path, read it
// 2. If stateArg is a Neuroglancer URL, decode it
// 3. If stateArg is empty, try stdin (if not a terminal)
// 4. If stdin is empty, try clipboard for a Neuroglancer URL
// 5. If generate is true or nothing found, use default template
func LoadState(stateArg string, generate bool) (*LoadResult, error) {
	return loadState(stateArg, generate, true)
}

// LoadInputState resolves a state the way LoadState does without generate,
// but returns an error instead of falling back to a default state. Commands
// that update the user's own state use it, since editing a default scene
// would not do what was asked.
func LoadInputState(stateArg string) (*LoadResult, error) {
	return loadState(stateArg, false, false)
}

func loadState(stateArg string, generate, useDefault bool) (*LoadResult, error) {
	// Explicit --state argument
	if stateArg != "" {
		// Prefer existing files, even if the filename contains URL-like substrings
		// such as "neuroglancer".
		data, readErr := os.ReadFile(stateArg)
		if readErr == nil {
			state, err := parseJSON(data)
			if err != nil {
				return nil, fmt.Errorf("parsing state file %q: %w", stateArg, err)
			}
			return &LoadResult{State: state, Source: SourceFile}, nil
		}
		// Fall back to URL decoding only when the argument does not name an
		// existing path. Very long URLs are not valid paths (ReadFile may return
		// ENAMETOOLONG), so os.Stat also fails and they still decode as URLs.
		if IsNeuroglancerURL(stateArg) {
			if _, statErr := os.Stat(stateArg); statErr != nil && !os.IsPermission(statErr) {
				state, err := DecodeURL(stateArg)
				if err != nil {
					return nil, fmt.Errorf("decoding URL: %w", err)
				}
				return &LoadResult{State: state, Source: SourceURL, OriginalURL: stateArg}, nil
			}
		}

		return nil, fmt.Errorf("reading state file %q: %w", stateArg, readErr)
	}

	// Try stdin if it's not a terminal
	if !isTerminal(os.Stdin) {
		// Bound the read so a hostile or broken pipe cannot exhaust memory.
		data, err := io.ReadAll(io.LimitReader(os.Stdin, maxStateBytes+1))
		if err != nil {
			return nil, fmt.Errorf("reading stdin: %w", err)
		}
		if len(data) > maxStateBytes {
			return nil, fmt.Errorf("stdin state exceeds %d bytes", maxStateBytes)
		}
		if len(strings.TrimSpace(string(data))) > 0 {
			state, err := parseJSON(data)
			if err != nil {
				return nil, fmt.Errorf("parsing stdin: %w", err)
			}
			return &LoadResult{State: state, Source: SourceStdin}, nil
		}
	}

	// Try clipboard. An unavailable optional clipboard still permits the
	// documented default-state fallback, but it must not be silent. A value
	// that identifies itself as a Neuroglancer URL without a state fragment is
	// just the bare viewer link, so it also falls back with a warning. A
	// fragment that will not decode is treated as intentional; malformed state
	// is therefore an error rather than a quiet switch to an unrelated
	// template.
	if !generate {
		clip, clipErr := stateClipboardRead()
		if clipErr != nil {
			if !useDefault {
				return nil, fmt.Errorf("%w (clipboard input unavailable: %v)", errNoInputState, clipErr)
			}
			fmt.Fprintf(stateWarningWriter, "Warning: clipboard input unavailable; using the default state: %v\n", clipErr)
		} else if IsNeuroglancerURL(clip) {
			state, err := DecodeURL(clip)
			if errors.Is(err, errNoFragment) {
				if !useDefault {
					return nil, fmt.Errorf("%w (the clipboard holds a Neuroglancer viewer URL without a state fragment)", errNoInputState)
				}
				fmt.Fprintln(stateWarningWriter, "Warning: clipboard holds a Neuroglancer viewer URL without a state fragment; using the default state")
			} else if err != nil {
				return nil, fmt.Errorf("decoding clipboard URL: %w", err)
			} else {
				return &LoadResult{State: state, Source: SourceClipboard, OriginalURL: clip}, nil
			}
		}
	}

	if !useDefault {
		return nil, errNoInputState
	}

	// Check for user-configured default state
	if data, err := ReadDefaultState(); err == nil && len(data) > 0 {
		state, err := parseJSON(data)
		if err == nil {
			return &LoadResult{State: state, Source: SourceTemplate}, nil
		}
	}

	// Fallback: generate from embedded template
	state, err := parseJSON(DefaultScene)
	if err != nil {
		return nil, fmt.Errorf("parsing default template: %w", err)
	}
	return &LoadResult{State: state, Source: SourceTemplate}, nil
}

// WriteState preserves the legacy no-browser API for state-editing commands.
// New callers that also need a browser handoff should use DeliverState so all
// destinations are attempted independently.
func WriteState(result *LoadResult, outputFile string) error {
	return DeliverState(result, DeliveryOptions{OutputFile: outputFile})
}

func parseJSON(data []byte) (map[string]interface{}, error) {
	var state map[string]interface{}
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	// "null" unmarshals into a nil map without an error.
	if state == nil {
		return nil, errors.New("expected a JSON object, got null")
	}
	return state, nil
}

func writeToFile(state map[string]interface{}, path string) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}
	// 0600: states can embed capability URLs (e.g. unlisted-gist label sources).
	if err := writeFile0600(path, append(data, '\n')); err != nil {
		return fmt.Errorf("writing file %q: %w", path, err)
	}
	fmt.Fprintf(os.Stderr, "Wrote state to %s\n", path)
	return nil
}

// writeFile0600 writes data to path, creating it with mode 0600. OpenFile's
// mode applies only to new files, so an existing regular file that others can
// access is restricted before it is truncated. Devices, pipes, and symbolic
// links are written through, as os.WriteFile would.
func writeFile0600(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0o600)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	if info.Mode().IsRegular() {
		if info.Mode().Perm()&0o077 != 0 {
			// FAT mounts and files owned by another user cannot change mode;
			// warn rather than refuse a write os.WriteFile would have made.
			if err := f.Chmod(0o600); err != nil {
				fmt.Fprintf(stateWarningWriter, "Warning: state file is accessible by other users and could not be restricted: %v\n", err)
			}
		}
		if err := f.Truncate(0); err != nil {
			f.Close()
			return err
		}
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func writeStateJSON(w io.Writer, state map[string]interface{}) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}
	_, err = fmt.Fprintln(w, string(data))
	return err
}

func isTerminal(f *os.File) bool {
	stat, err := f.Stat()
	if err != nil {
		return true
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}
