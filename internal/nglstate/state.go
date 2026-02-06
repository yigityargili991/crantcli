package nglstate

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"crant_type_look/internal/clipboard"
)

// StateSource describes where the state was loaded from.
type StateSource string

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
}

// LoadState loads a Neuroglancer state using smart resolution:
// 1. If stateArg is a URL, decode it
// 2. If stateArg is a file path, read it
// 3. If stateArg is empty, try stdin (if not a terminal)
// 4. If stdin is empty, try clipboard for a Neuroglancer URL
// 5. If generate is true or nothing found, use default template
func LoadState(stateArg string, generate bool) (*LoadResult, error) {
	// Explicit --state argument
	if stateArg != "" {
		if IsNeuroglancerURL(stateArg) {
			state, err := DecodeURL(stateArg)
			if err != nil {
				return nil, fmt.Errorf("decoding URL: %w", err)
			}
			return &LoadResult{State: state, Source: SourceURL, OriginalURL: stateArg}, nil
		}

		// Try as file path
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
	if !generate {
		clip, err := clipboard.Read()
		if err == nil && IsNeuroglancerURL(clip) {
			state, err := DecodeURL(clip)
			if err == nil {
				return &LoadResult{State: state, Source: SourceClipboard, OriginalURL: clip}, nil
			}
		}
	}

	// Fallback: generate from template
	state, err := parseJSON(DefaultScene)
	if err != nil {
		return nil, fmt.Errorf("parsing default template: %w", err)
	}
	return &LoadResult{State: state, Source: SourceTemplate}, nil
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
	case SourceClipboard, SourceURL, SourceTemplate:
		nglURL, err := EncodeURL(result.State, "")
		if err != nil {
			return err
		}
		if err := clipboard.Write(nglURL); err != nil {
			// Fallback to stdout if clipboard fails
			fmt.Println(nglURL)
			return nil
		}
		fmt.Fprintf(os.Stderr, "Neuroglancer URL copied to clipboard\n")
		return nil
	default:
		// Stdin or file without -o: write JSON to stdout
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

func writeToFile(state map[string]interface{}, path string) error {
	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling state: %w", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0644); err != nil {
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
