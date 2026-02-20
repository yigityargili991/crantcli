package cmd

import (
	"errors"
	"strings"
	"testing"

	"crantinject/internal/nglstate"
)

func TestValidateAddModeFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		unpile      bool
		state       string
		generate    bool
		output      string
		rootIDsOnly bool
		wantErr     string
	}{
		{
			name:    "unpile with state",
			unpile:  true,
			state:   "state.json",
			wantErr: "--unpile cannot be used with --state",
		},
		{
			name:     "unpile with generate",
			unpile:   true,
			generate: true,
			wantErr:  "--unpile cannot be used with --generate",
		},
		{
			name:    "unpile with output",
			unpile:  true,
			output:  "out.json",
			wantErr: "--unpile cannot be used with --output",
		},
		{
			name:        "unpile with root ids only",
			unpile:      true,
			rootIDsOnly: true,
			wantErr:     "--unpile cannot be used with --root-ids-only",
		},
		{
			name:   "unpile alone",
			unpile: true,
		},
		{
			name:        "no unpile ignores other flags",
			unpile:      false,
			state:       "state.json",
			generate:    true,
			output:      "out.json",
			rootIDsOnly: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := validateAddModeFlags(tt.unpile, tt.state, tt.generate, tt.output, tt.rootIDsOnly)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}

			if err == nil {
				t.Fatalf("expected error %q, got nil", tt.wantErr)
			}
			if err.Error() != tt.wantErr {
				t.Fatalf("expected error %q, got %q", tt.wantErr, err.Error())
			}
		})
	}
}

func TestResolveAddState_DefaultRequiresValidClipboardURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		clip          string
		clipErr       error
		decodeState   map[string]interface{}
		decodeErr     error
		wantErrSubstr string
	}{
		{
			name:          "clipboard read error",
			clipErr:       errors.New("clipboard unavailable"),
			wantErrSubstr: "reading clipboard: clipboard unavailable",
		},
		{
			name:          "invalid clipboard content",
			clip:          "not a valid url",
			wantErrSubstr: "clipboard does not contain a valid Neuroglancer URL",
		},
		{
			name:          "decode error",
			clip:          "https://example.org/#!{}",
			decodeErr:     errors.New("bad fragment"),
			wantErrSubstr: "decoding Neuroglancer URL from clipboard: bad fragment",
		},
		{
			name:        "success trims clipboard and decodes",
			clip:        "  https://example.org/#!{\"layers\":[]}  \n",
			decodeState: map[string]interface{}{"layers": []interface{}{}},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var decodeInput string
			decodeCalled := false
			result, err := resolveAddState("", false, false, addStateResolverDeps{
				loadState: func(stateArg string, generate bool) (*nglstate.LoadResult, error) {
					t.Fatalf("loadState should not be called in default clipboard mode")
					return nil, nil
				},
				readClipboard: func() (string, error) {
					return tt.clip, tt.clipErr
				},
				decodeURL: func(url string) (map[string]interface{}, error) {
					decodeCalled = true
					decodeInput = url
					return tt.decodeState, tt.decodeErr
				},
			})

			if tt.wantErrSubstr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErrSubstr)
				}
				if !strings.Contains(err.Error(), tt.wantErrSubstr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErrSubstr, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if result == nil {
				t.Fatalf("expected non-nil result")
			}
			if result.Source != nglstate.SourceClipboard {
				t.Fatalf("expected source %q, got %q", nglstate.SourceClipboard, result.Source)
			}
			if result.OriginalURL != "https://example.org/#!{\"layers\":[]}" {
				t.Fatalf("unexpected original URL: %q", result.OriginalURL)
			}
			if !decodeCalled {
				t.Fatalf("expected decodeURL to be called")
			}
			if decodeInput != "https://example.org/#!{\"layers\":[]}" {
				t.Fatalf("expected trimmed URL to be decoded, got %q", decodeInput)
			}
		})
	}
}

func TestResolveAddState_ExplicitModes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		stateArg      string
		generate      bool
		unpile        bool
		wantStateArg  string
		wantGenerate  bool
		wantReadClip  bool
		wantDecodeURL bool
	}{
		{
			name:         "unpile uses template",
			unpile:       true,
			wantStateArg: "",
			wantGenerate: true,
		},
		{
			name:         "state arg uses explicit file/url path",
			stateArg:     "state.json",
			wantStateArg: "state.json",
			wantGenerate: false,
		},
		{
			name:         "generate uses template",
			generate:     true,
			wantStateArg: "",
			wantGenerate: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			loadCalled := false
			readClipCalled := false
			decodeCalled := false
			wantResult := &nglstate.LoadResult{
				State:  map[string]interface{}{"layers": []interface{}{}},
				Source: nglstate.SourceTemplate,
			}

			got, err := resolveAddState(tt.stateArg, tt.generate, tt.unpile, addStateResolverDeps{
				loadState: func(stateArg string, generate bool) (*nglstate.LoadResult, error) {
					loadCalled = true
					if stateArg != tt.wantStateArg {
						t.Fatalf("unexpected stateArg: got %q want %q", stateArg, tt.wantStateArg)
					}
					if generate != tt.wantGenerate {
						t.Fatalf("unexpected generate: got %v want %v", generate, tt.wantGenerate)
					}
					return wantResult, nil
				},
				readClipboard: func() (string, error) {
					readClipCalled = true
					return "", nil
				},
				decodeURL: func(url string) (map[string]interface{}, error) {
					decodeCalled = true
					return nil, nil
				},
			})
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if got != wantResult {
				t.Fatalf("expected loadState result to be returned directly")
			}
			if !loadCalled {
				t.Fatalf("expected loadState to be called")
			}
			if readClipCalled {
				t.Fatalf("did not expect clipboard read for explicit mode")
			}
			if decodeCalled {
				t.Fatalf("did not expect URL decode for explicit mode")
			}
		})
	}
}
