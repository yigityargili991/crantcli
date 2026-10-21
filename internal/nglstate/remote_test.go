package nglstate

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
)

const (
	testShareLocation = "middleauth+https://global.daf-apis.com/nglstate/api/v1/5195345298243584"
	testShareLink     = "https://spelunker.cave-explorer.org/#!" + testShareLocation
	testStoredState   = `{"layers":[{"type":"segmentation","name":"seg","segments":["1"]}]}`
)

// isolateRemoteLoad gives a test an empty stdin, a private default-state
// cache, captured warnings, the clipboard text clip, and the resolver.
func isolateRemoteLoad(t *testing.T, clip string, resolve RemoteStateResolver) *bytes.Buffer {
	t.Helper()
	previousRead := stateClipboardRead
	previousWarnings := stateWarningWriter
	previousCachePath := testCachePath
	previousResolve := resolveRemoteState
	var warnings bytes.Buffer
	stateWarningWriter = &warnings
	testCachePath = t.TempDir()
	stateClipboardRead = func() (string, error) { return clip, nil }
	resolveRemoteState = resolve
	replaceStdin(t, "")
	t.Cleanup(func() {
		stateClipboardRead = previousRead
		stateWarningWriter = previousWarnings
		testCachePath = previousCachePath
		resolveRemoteState = previousResolve
	})
	return &warnings
}

// fakeResolver answers every location with body and err, and records the
// locations it was asked for.
func fakeResolver(body string, err error, asked *[]RemoteState) RemoteStateResolver {
	return func(remote RemoteState) ([]byte, error) {
		*asked = append(*asked, remote)
		return []byte(body), err
	}
}

func TestLoadStateResolvesShareLink(t *testing.T) {
	tests := []struct {
		name       string
		clip, arg  string
		load       func(string) (*LoadResult, error)
		wantSource StateSource
	}{
		{"clipboard", testShareLink, "", func(arg string) (*LoadResult, error) { return LoadState(arg, false) }, SourceClipboard},
		{"--state", "", testShareLink, func(arg string) (*LoadResult, error) { return LoadState(arg, false) }, SourceURL},
		{"clipboard for check-cave", testShareLink, "", LoadInputState, SourceClipboard},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var asked []RemoteState
			warnings := isolateRemoteLoad(t, tt.clip, fakeResolver(testStoredState, nil, &asked))

			result, err := tt.load(tt.arg)
			if err != nil {
				t.Fatalf("load: %v", err)
			}
			if len(asked) != 1 || asked[0].Location != testShareLocation {
				t.Fatalf("resolver asked for %+v, want %q once", asked, testShareLocation)
			}
			if result.Source != tt.wantSource || result.OriginalURL != testShareLink {
				t.Fatalf("source = %q, original URL = %q; want %q and the Share link", result.Source, result.OriginalURL, tt.wantSource)
			}
			if result.Remote == nil || result.Remote.Location != testShareLocation {
				t.Fatalf("remote = %+v, want %q", result.Remote, testShareLocation)
			}
			layer, _, err := FindSegmentationLayer(result.State, "seg")
			if err != nil || len(layer["segments"].([]interface{})) != 1 {
				t.Fatalf("state = %#v, %v; want the stored state", result.State, err)
			}
			want := fmt.Sprintf("%s (resolved Share link %q)", tt.wantSource, testShareLocation)
			if got := result.SourceDescription(); got != want {
				t.Fatalf("SourceDescription() = %q, want %q", got, want)
			}
			if warnings.Len() != 0 {
				t.Fatalf("unexpected warning %q", warnings.String())
			}
		})
	}
}

func TestLoadStateReadsFetchedStateLikeEmbeddedState(t *testing.T) {
	// Old scenes store graphene sources with the '+' decoded to a space;
	// a fetched state gets the same repair as one decoded from a URL.
	const legacy = `{"layers":[{"type":"segmentation","source":"graphene://middleauth https://data.proofreading.zetta.ai/segmentation/table/kronauer_ant_x1"}]}`
	var asked []RemoteState
	isolateRemoteLoad(t, testShareLink, fakeResolver(legacy, nil, &asked))

	result, err := LoadState("", false)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	layer := result.State["layers"].([]interface{})[0].(map[string]interface{})
	if got, want := layer["source"], "graphene://middleauth+https://data.proofreading.zetta.ai/segmentation/table/kronauer_ant_x1"; got != want {
		t.Fatalf("source = %#v, want %q", got, want)
	}
}

func TestLoadStateShareLinkFetchFailure(t *testing.T) {
	boom := errors.New("the state server has no such state (HTTP 404)")
	var asked []RemoteState
	warnings := isolateRemoteLoad(t, testShareLink, fakeResolver("", boom, &asked))

	t.Run("clipboard", func(t *testing.T) {
		result, err := LoadState("", false)
		want := `the clipboard holds a Neuroglancer Share link, but the state stored at "` + testShareLocation +
			`" could not be loaded: the state server has no such state (HTTP 404); to load the view without fetching it, copy the URL from the viewer's address bar instead`
		if err == nil || err.Error() != want {
			t.Fatalf("LoadState() = %+v, %v; want error %q", result, err, want)
		}
		if !errors.Is(err, boom) {
			t.Fatalf("error %v does not wrap the resolver's error", err)
		}
	})

	t.Run("--state", func(t *testing.T) {
		_, err := LoadState(testShareLink, false)
		want := `decoding URL: the state stored at "` + testShareLocation + `" could not be loaded: the state server has no such state (HTTP 404)`
		if err == nil || err.Error() != want {
			t.Fatalf("LoadState() error = %v, want %q", err, want)
		}
	})

	if warnings.Len() != 0 {
		t.Fatalf("a failed fetch fell back with warning %q", warnings.String())
	}
}

func TestLoadStateShareLinkUnsupportedLocation(t *testing.T) {
	// The resolver declines schemes it cannot fetch, and the error is the one
	// loading a Share link gives without a resolver.
	const location = "gs://bucket/state.json"
	const link = "https://spelunker.cave-explorer.org/#!" + location
	var asked []RemoteState
	isolateRemoteLoad(t, link, fakeResolver("", fmt.Errorf("%w: not https", errors.ErrUnsupported), &asked))

	_, err := LoadState("", false)
	want := `the clipboard holds a Neuroglancer Share link to "gs://bucket/state.json", whose state crantcli cannot load; copy the URL from the viewer's address bar instead, or pass --state`
	if err == nil || err.Error() != want {
		t.Fatalf("LoadState() error = %v, want %q", err, want)
	}

	_, err = LoadState(link, false)
	want = `decoding URL: the state is stored at "gs://bucket/state.json" (a Neuroglancer Share link), not in the URL, and crantcli cannot load it from there`
	if err == nil || err.Error() != want {
		t.Fatalf("LoadState(link) error = %v, want %q", err, want)
	}
	if len(asked) != 2 {
		t.Fatalf("resolver asked %d times, want 2", len(asked))
	}
}

func TestLoadStateShareLinkWithoutStateObject(t *testing.T) {
	for body, want := range map[string]string{
		"null":  "it is not a Neuroglancer state: expected a JSON object, got null",
		"[1,2]": "it is not a Neuroglancer state: json: cannot unmarshal array",
	} {
		t.Run(body, func(t *testing.T) {
			var asked []RemoteState
			isolateRemoteLoad(t, testShareLink, fakeResolver(body, nil, &asked))
			_, err := LoadState("", false)
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("LoadState() error = %v, want it to contain %q", err, want)
			}
		})
	}
}

func TestLoadStateGenerateSkipsShareLink(t *testing.T) {
	var asked []RemoteState
	isolateRemoteLoad(t, testShareLink, fakeResolver(testStoredState, nil, &asked))

	result, err := LoadState("", true)
	if err != nil {
		t.Fatalf("LoadState: %v", err)
	}
	if result.Source != SourceTemplate || result.Remote != nil || len(asked) != 0 {
		t.Fatalf("LoadState(generate) = source %q, remote %+v, %d fetches; want the template without a fetch", result.Source, result.Remote, len(asked))
	}
}

func TestSourceDescriptionWithoutShareLink(t *testing.T) {
	result := &LoadResult{Source: SourceClipboard, OriginalURL: "https://spelunker.cave-explorer.org/#!{}"}
	if got := result.SourceDescription(); got != "clipboard" {
		t.Fatalf("SourceDescription() = %q, want clipboard", got)
	}
}
