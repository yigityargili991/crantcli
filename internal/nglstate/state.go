package nglstate

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"crantcli/internal/clipboard"
)

// StateSource describes where the state was loaded from.
type StateSource string

const (
	SourceURL       StateSource = "url"
	SourceFile      StateSource = "file"
	SourceStdin     StateSource = "stdin"
	SourceClipboard StateSource = "clipboard"
	SourceLastState StateSource = "last-state"
	SourceTemplate  StateSource = "template"
)

var (
	clipboardRead  = clipboard.Read
	clipboardWrite = clipboard.Write
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

// LoadState loads a Neuroglancer state using smart resolution:
// 1. If stateArg is a URL, decode it
// 2. If stateArg is a file path, read it
// 3. If generate is true, use the default template
// 4. If stateArg is empty, try stdin (if not a terminal)
// 5. If stdin is empty, try clipboard for a Neuroglancer URL
// 6. If clipboard is empty, try the last state URL from a previous session
// 7. If nothing found, use the default template
func LoadState(stateArg string, generate bool) (*LoadResult, error) {
	// Explicit --state argument
	if stateArg != "" {
		// Prefer existing files, even if the filename contains URL-like substrings
		// such as "neuroglancer".
		if state, err := loadStateFileIfExists(stateArg); err != nil {
			return nil, err
		} else if state != nil {
			return &LoadResult{State: state, Source: SourceFile}, nil
		}

		if IsNeuroglancerURL(stateArg) {
			state, err := DecodeURL(stateArg)
			if err != nil {
				return nil, fmt.Errorf("decoding URL: %w", err)
			}
			return &LoadResult{State: state, Source: SourceURL, OriginalURL: stateArg}, nil
		}

		// Try as file path and return the concrete read/parse error.
		data, err := os.ReadFile(stateArg)
		if err != nil {
			return nil, fmt.Errorf("reading state file %q: %w", stateArg, err)
		}
		state, err := parseJSON(data)
		if err != nil {
			return nil, fmt.Errorf("parsing state file %q: %w", stateArg, err)
		}
		return &LoadResult{State: state, Source: SourceFile}, nil
	}

	if generate {
		return loadDefaultTemplate()
	}

	// Try stdin if it's not a terminal
	if !isTerminal(os.Stdin) {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, fmt.Errorf("reading stdin: %w", err)
		}
		if len(strings.TrimSpace(string(data))) > 0 {
			state, err := parseJSON(data)
			if err != nil {
				return nil, fmt.Errorf("parsing stdin: %w", err)
			}
			return &LoadResult{State: state, Source: SourceStdin}, nil
		}
	}

	// Try clipboard
	clip, err := clipboardRead()
	if err == nil && IsNeuroglancerURL(clip) {
		state, err := DecodeURL(clip)
		if err != nil {
			return nil, fmt.Errorf("decoding clipboard Neuroglancer URL: %w", err)
		}
		return &LoadResult{State: state, Source: SourceClipboard, OriginalURL: clip}, nil
	}

	// Try last session state
	if lastURL := readLastStateURL(); IsNeuroglancerURL(lastURL) {
		state, err := DecodeURL(lastURL)
		if err != nil {
			return nil, fmt.Errorf("decoding last-session Neuroglancer URL: %w", err)
		}
		return &LoadResult{State: state, Source: SourceLastState, OriginalURL: lastURL}, nil
	}

	return loadDefaultTemplate()
}

func loadDefaultTemplate() (*LoadResult, error) {
	// Check for user-configured default state
	data, err := ReadDefaultState()
	if err != nil {
		return nil, fmt.Errorf("reading default state: %w", err)
	}
	if len(data) > 0 {
		state, err := parseJSON(data)
		if err != nil {
			return nil, fmt.Errorf("parsing default state: %w", err)
		}
		if err := ValidateUsableState(state); err != nil {
			return nil, fmt.Errorf("validating default state: %w", err)
		}
		return &LoadResult{State: state, Source: SourceTemplate}, nil
	}

	// Fallback: generate from embedded template
	state, err := parseJSON(DefaultScene)
	if err != nil {
		return nil, fmt.Errorf("parsing default template: %w", err)
	}
	if err := ValidateUsableState(state); err != nil {
		return nil, fmt.Errorf("validating default template: %w", err)
	}
	return &LoadResult{State: state, Source: SourceTemplate}, nil
}

func loadStateFileIfExists(path string) (map[string]interface{}, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading state file %q: %w", path, err)
	}
	if info.IsDir() {
		return nil, fmt.Errorf("state path %q is a directory", path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading state file %q: %w", path, err)
	}
	state, err := parseJSON(data)
	if err != nil {
		return nil, fmt.Errorf("parsing state file %q: %w", path, err)
	}
	return state, nil
}

// WriteState outputs the state to the appropriate destination.
// If outputFile is set, write to file.
// If source was clipboard or URL, encode as URL and copy to clipboard.
// Otherwise, write JSON to stdout.
func WriteState(result *LoadResult, outputFile string) error {
	if outputFile != "" {
		return writeToFile(result.State, outputFile)
	}

	switch result.Source {
	case SourceClipboard, SourceURL, SourceLastState, SourceTemplate:
		nglURL, err := EncodeURL(result.State, "")
		if err != nil {
			return err
		}
		result.OutputURL = nglURL
		if err := writeLastStateURL(nglURL); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: could not save last state URL: %v\n", err)
		}
		if err := clipboardWrite(nglURL); err != nil {
			fmt.Println(nglURL)
			return nil
		}
		fmt.Fprintf(os.Stderr, "Neuroglancer URL copied to clipboard\n")
		return nil
	default:
		return writeToStdout(result.State)
	}
}

func parseJSON(data []byte) (map[string]interface{}, error) {
	var state map[string]interface{}
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, err
	}
	return state, nil
}

// ValidateUsableState verifies that a Neuroglancer state has layers and at
// least one segmentation layer that commands can modify.
func ValidateUsableState(state map[string]interface{}) error {
	if state == nil {
		return fmt.Errorf("state is empty")
	}
	layersRaw, ok := state["layers"]
	if !ok {
		return fmt.Errorf("state has no 'layers' key")
	}
	layers, ok := layersRaw.([]interface{})
	if !ok {
		return fmt.Errorf("'layers' is not an array")
	}
	if len(layers) == 0 {
		return fmt.Errorf("state has no usable layers")
	}
	if _, _, err := FindSegmentationLayer(state, ""); err != nil {
		return err
	}
	return nil
}

func writeToFile(state map[string]interface{}, path string) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("writing file %q: %w", path, err)
	}
	fmt.Fprintf(os.Stderr, "Wrote state to %s\n", path)
	return nil
}

func writeToStdout(state map[string]interface{}) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}
	_, err = fmt.Println(string(data))
	return err
}

func isTerminal(f *os.File) bool {
	stat, err := f.Stat()
	if err != nil {
		return true
	}
	return (stat.Mode() & os.ModeCharDevice) != 0
}
