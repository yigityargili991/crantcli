package ask

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// testSpec builds a small command tree shaped like crantcli's.
func testSpec() []*CmdSpec {
	root := &cobra.Command{Use: "crantcli"}
	run := func(*cobra.Command, []string) {}
	add := &cobra.Command{Use: "add", Run: run}
	add.Flags().StringArray("cell-type", nil, "")
	add.Flags().StringArray("cell-class", nil, "")
	add.Flags().String("side", "", "")
	add.Flags().String("color-by", "", "")
	add.Flags().String("layer", "", "")
	add.Flags().StringP("state", "s", "", "")
	add.Flags().Bool("open", false, "")
	add.Flags().Bool("intersect", false, "")
	add.Flags().Bool("labels", false, "")
	history := &cobra.Command{Use: "cave-history <root_id>", Run: run}
	history.Flags().Bool("open", false, "")
	history.Flags().Bool("unfiltered", false, "")
	list := &cobra.Command{Use: "list <field>", Run: run}
	list.Flags().Bool("count", false, "")
	list.Flags().String("cell-type", "", "")
	lookup := &cobra.Command{Use: "lookup-column [root_id]", Run: run}
	labels := &cobra.Command{Use: "labels"}
	clean := &cobra.Command{Use: "clean", Run: run}
	clean.Flags().Bool("all", false, "")
	labels.AddCommand(clean)
	hidden := &cobra.Command{Use: "secret", Run: run, Hidden: true}
	askCmd := &cobra.Command{Use: "ask <request>", Run: run}
	root.AddCommand(add, history, list, lookup, labels, hidden, askCmd)
	return BuildSpec(root, "ask")
}

func testVocab() *Vocab {
	return NewVocab(map[string][]string{
		"cell_class":   {"ER", "EBc"},
		"cell_type":    {"PEN", "EPG", "ER2", "ER"},
		"cell_subtype": {"PEN", "ER"},
		"side":         {"left", "right", "medial"},
	})
}

func TestBuildSpec(t *testing.T) {
	spec := testSpec()
	var paths []string
	for _, c := range spec {
		paths = append(paths, c.Path)
	}
	want := []string{"add", "cave-history", "labels clean", "list", "lookup-column"}
	if !reflect.DeepEqual(paths, want) {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
	if c := findCmd(spec, "cave-history"); c.ArgsUse != "<root_id>" {
		t.Errorf("cave-history ArgsUse = %q", c.ArgsUse)
	}
	if f := findCmd(spec, "add").flag("cell-type"); f == nil || f.Field != "cell_type" || f.Type != "stringArray" {
		t.Errorf("add --cell-type spec = %+v", f)
	}
	if f := findCmd(spec, "add").flag("s"); f == nil || f.Name != "state" {
		t.Errorf("shorthand -s did not resolve to --state: %+v", f)
	}
}

func TestBuildSpecCarriesRequiredArgs(t *testing.T) {
	root := &cobra.Command{Use: "crantcli"}
	runE := false
	required := &cobra.Command{Use: "required [root_id...]", Args: cobra.MinimumNArgs(1),
		RunE: func(*cobra.Command, []string) error { runE = true; return nil }}
	optional := &cobra.Command{Use: "optional [root_id...]", Run: func(*cobra.Command, []string) {}}
	root.AddCommand(required, optional)

	spec := BuildSpec(root)
	if got := findCmd(spec, "required"); got == nil || got.ArgsMin != 1 {
		t.Errorf("required ArgsMin = %+v, want 1", got)
	}
	if got := findCmd(spec, "optional"); got == nil || got.ArgsMin != 0 {
		t.Errorf("optional ArgsMin = %+v, want 0", got)
	}
	if runE {
		t.Error("BuildSpec ran the command instead of inspecting its Args validator")
	}
}

func TestRetrieve(t *testing.T) {
	v := testVocab()
	tests := []struct {
		name, q string
		want    []string
	}{
		{"trailing question mark", "show cell type ER2?", []string{"cell_type=ER2"}},
		{"hyphenated side", "the left-side PEN neurons", []string{"cell_type=PEN", "cell_subtype=PEN", "side=left"}},
		{"glossary phrase", "show the compass neurons", []string{"cell_type=EPG"}},
		{"value at three levels", "ER neurons", []string{"cell_class=ER", "cell_type=ER", "cell_subtype=ER"}},
		{"stopword is not a code", "show all of them", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, c := range v.retrieve(tt.q) {
				got = append(got, c.Field+"="+c.Value)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("retrieve(%q) = %v, want %v", tt.q, got, tt.want)
			}
		})
	}
}

func TestPrompt(t *testing.T) {
	got := testVocab().prompt("show PEN neurons on the left")
	want := "request: show PEN neurons on the left\ntable: cell_type=PEN (\"pen\"); cell_subtype=PEN (\"pen\"); side=left (\"left\");\ncommand:"
	if got != want {
		t.Errorf("prompt = %q, want %q", got, want)
	}
}

func TestStripPurpose(t *testing.T) {
	tests := []struct{ in, want string }{
		{"show ER neurons so I can check them", "show ER neurons"},
		{"list the EBc types with counts, since I'm checking dataset balance.", "list the EBc types with counts."},
		{"save it to view.json", "save it to view.json"},
	}
	for _, tt := range tests {
		if got := stripPurpose(tt.in); got != tt.want {
			t.Errorf("stripPurpose(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestGrammarGates(t *testing.T) {
	spec, v := testSpec(), testVocab()
	tests := []struct {
		name, q   string
		has, lack []string
	}{
		{"open needs a word for it", "show PEN neurons", nil, []string{`" --open"`}},
		{"open when asked", "show PEN neurons and open it in the browser", []string{`" --open"`}, nil},
		{"open negated", "print PEN neurons without opening a scene", nil, []string{`" --open"`}},
		{"viewer means open", "show every edit of 576460752669867599 in the viewer", []string{`" --open"`}, nil},
		{"count needs a word for it", "list cell types in PEN", nil, []string{`" --count"`}},
		{"count when asked", "how many neurons per cell type in PEN", []string{`" --count"`}, nil},
		{"intersect needs a word for it", "show ER and PEN", nil, []string{`" --intersect"`}},
		{"unfiltered needs a word for it", "edit history of 576460752669867599", nil, []string{`" --unfiltered"`}},
		{"only retrieved values", "show PEN neurons", []string{`"PEN"`}, []string{`"EPG"`}},
		{"root id commands need an id", "show PEN neurons", nil, []string{`"cave-history"`}},
		{"column requests", "which column is 576460752669867599 in", []string{`"lookup-column"`}, []string{`"add"`, `"list"`}},
		{"single color-by field", "show PEN colored by cell_type", []string{`"cell_type"`}, []string{`"cell_type,cell_subtype"`}},
		{"two color-by fields", "show PEN colored by group and side", []string{`"group,side"`}, []string{`"group,cell_type"`}},
		{"unquoted layer", "show PEN on the seg v2 layer", []string{`"'seg v2'"`}, nil},
		{"file from request", "show PEN from scene.json", []string{`"scene.json"`}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := grammar(spec, v, tt.q)
			for _, s := range tt.has {
				if !strings.Contains(g, s) {
					t.Errorf("grammar for %q lacks %s", tt.q, s)
				}
			}
			for _, s := range tt.lack {
				if strings.Contains(g, s) {
					t.Errorf("grammar for %q contains %s", tt.q, s)
				}
			}
		})
	}
}

type fakeModel struct {
	out             string
	err             error
	prompt, grammar string
}

func (f *fakeModel) Complete(prompt, grammar string) (string, error) {
	f.prompt, f.grammar = prompt, grammar
	return f.out, f.err
}

func TestSuggest(t *testing.T) {
	spec, v := testSpec(), testVocab()
	m := &fakeModel{out: " add --cell-type PEN --cell-type PEN --layer 'seg v2' --open"}
	argv, err := Suggest(m, spec, v, "show  PEN on the seg v2 layer and open it so I can check them")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"add", "--cell-type", "PEN", "--layer", "seg v2", "--open"}
	if !reflect.DeepEqual(argv, want) {
		t.Errorf("argv = %q, want %q", argv, want)
	}
	if !strings.HasPrefix(m.prompt, "request: show PEN on the seg v2 layer and open it\n") {
		t.Errorf("prompt was not normalized and stripped: %q", m.prompt)
	}
	if got := QuoteArgv(argv); got != "add --cell-type PEN --layer 'seg v2' --open" {
		t.Errorf("QuoteArgv = %q", got)
	}
}

func TestSuggestErrors(t *testing.T) {
	spec, v := testSpec(), testVocab()
	if _, err := Suggest(&fakeModel{out: " bogus --x"}, spec, v, "do something"); err == nil {
		t.Error("expected an error for an invalid command")
	}
	if _, err := Suggest(&fakeModel{err: errors.New("boom")}, spec, v, "show PEN"); err == nil {
		t.Error("expected the model error")
	}
	if _, err := Suggest(&fakeModel{}, spec, v, "  "); err == nil {
		t.Error("expected an error for an empty request")
	}
}

func TestSuggestAcceptsQuotedApostrophe(t *testing.T) {
	// The grammar offers values as shellQuote writes them, so an apostrophe
	// arrives as '\'' and Suggest must read it back.
	spec, v := testSpec(), testVocab()
	quoted := shellQuote("Bob's layer")
	m := &fakeModel{out: " add --layer " + quoted}
	argv, err := Suggest(m, spec, v, `add a neuron to the "Bob's layer" layer`)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(m.grammar, gLit(quoted)) {
		t.Errorf("grammar does not offer %s:\n%s", gLit(quoted), m.grammar)
	}
	if want := []string{"add", "--layer", "Bob's layer"}; !reflect.DeepEqual(argv, want) {
		t.Errorf("argv = %q, want %q", argv, want)
	}
}

func TestLoadVocab(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ask", "vocab.json")
	calls := 0
	fetch := func() (map[string][]string, error) {
		calls++
		return map[string][]string{"cell_type": {"PEN"}}, nil
	}
	fail := func() (map[string][]string, error) { return nil, errors.New("offline") }

	if _, err := LoadVocab(path, time.Hour, fail); err == nil {
		t.Fatal("expected an error without cache or network")
	}
	if _, err := LoadVocab(path, time.Hour, fetch); err != nil || calls != 1 {
		t.Fatalf("first load: err=%v calls=%d", err, calls)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// Windows has no Unix permission bits.
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("cache file mode = %v, want 0600", info.Mode().Perm())
	}
	if _, err := LoadVocab(path, time.Hour, fetch); err != nil || calls != 1 {
		t.Fatalf("fresh cache should be reused: err=%v calls=%d", err, calls)
	}
	v, err := LoadVocab(path, 0, fail)
	if err != nil {
		t.Fatalf("stale cache should be used when fetching fails: %v", err)
	}
	if got := v.retrieve("PEN neurons"); len(got) != 1 || got[0].Value != "PEN" {
		t.Errorf("stale cache values = %v", got)
	}
}
