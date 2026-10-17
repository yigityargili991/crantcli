package ask

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestGatedValues(t *testing.T) {
	e := gatherEvidence(testVocab(), "PEN from scene.json at 1,2,3 older than 48h, last 12 edits, layer 'seg v2', proofread ones")
	tests := []struct {
		flag FlagSpec
		want []string
	}{
		{FlagSpec{Name: "cell-type", Field: "cell_type"}, []string{"PEN"}},
		{FlagSpec{Name: "proofread", Field: "proofread"}, []string{"true", "false"}},
		{FlagSpec{Name: "state"}, []string{"scene.json"}},
		{FlagSpec{Name: "output"}, []string{"scene.json"}},
		{FlagSpec{Name: "pos"}, []string{"1,2,3"}},
		{FlagSpec{Name: "layer"}, []string{"seg v2"}},
		{FlagSpec{Name: "labels-hook"}, []string{"seg v2"}},
		{FlagSpec{Name: "older-than", Type: "duration"}, []string{"48h"}},
		{FlagSpec{Name: "history-limit", Type: "int"}, []string{"0", "1", "2", "3", "12"}},
		{FlagSpec{Name: "note", Type: "string"}, nil},
	}
	for _, tt := range tests {
		if got := gatedValues(tt.flag, e); !reflect.DeepEqual(got, tt.want) {
			t.Errorf("gatedValues(%s) = %q, want %q", tt.flag.Name, got, tt.want)
		}
	}
	for _, name := range []string{"color", "label-by", "label-tags", "color-by"} {
		if got := gatedValues(FlagSpec{Name: name}, e); len(got) == 0 {
			t.Errorf("gatedValues(%s) is empty", name)
		}
	}
	if got := gatedValues(FlagSpec{Name: "label-tags"}, e); got[0] != "none" {
		t.Errorf("label-tags should offer none first, got %q", got[0])
	}
}

func TestGatedArgs(t *testing.T) {
	withID := gatherEvidence(testVocab(), "check 576460752669867599 and 720575940610453042 in a.json")
	without := gatherEvidence(testVocab(), "check everything")
	tests := []struct {
		name    string
		cmd     CmdSpec
		e       evidence
		want    argRule
		allowed bool
	}{
		{"list field", CmdSpec{Path: "list", ArgsUse: "<field>"}, without, argRule{vals: labelFields, min: 1, max: 1}, true},
		{"many ids", CmdSpec{Path: "check-cave", ArgsUse: "[root_id...]"}, withID, argRule{vals: []string{"576460752669867599", "720575940610453042"}}, true},
		{"many ids, none given", CmdSpec{Path: "check-cave", ArgsUse: "[root_id...]"}, without, argRule{}, true},
		{"required many ids", CmdSpec{Path: "cave-history", ArgsUse: "[root_id...]", ArgsMin: 1}, withID, argRule{vals: []string{"576460752669867599", "720575940610453042"}, min: 1}, true},
		{"required many ids, none given", CmdSpec{Path: "cave-history", ArgsUse: "[root_id...]", ArgsMin: 1}, without, argRule{}, false},
		{"required id", CmdSpec{Path: "root-info", ArgsUse: "<root_id>"}, withID, argRule{vals: []string{"576460752669867599", "720575940610453042"}, min: 1, max: 1}, true},
		{"required id missing", CmdSpec{Path: "root-info", ArgsUse: "<root_id>"}, without, argRule{}, false},
		{"optional id", CmdSpec{Path: "lookup-column", ArgsUse: "[root_id]"}, withID, argRule{vals: []string{"576460752669867599", "720575940610453042"}, max: 1}, true},
		{"file", CmdSpec{Path: "change-def-state", ArgsUse: "[file]"}, withID, argRule{vals: []string{"a.json"}, max: 1}, true},
		{"no args", CmdSpec{Path: "generate"}, withID, argRule{}, true},
	}
	for _, tt := range tests {
		got, ok := gatedArgs(&tt.cmd, tt.e)
		if ok != tt.allowed || !reflect.DeepEqual(got, tt.want) {
			t.Errorf("%s: gatedArgs = %+v, %v; want %+v, %v", tt.name, got, ok, tt.want, tt.allowed)
		}
	}
}

func TestGrammarArgs(t *testing.T) {
	g := &gbnf{memo: map[string]string{}}
	if got := grammarArgs(g, argRule{}); got != "" {
		t.Errorf("no values: %q", got)
	}
	one := grammarArgs(g, argRule{vals: []string{"x"}, min: 1, max: 1})
	if got := grammarArgs(g, argRule{vals: []string{"x"}, max: 1}); got != one+"?" {
		t.Errorf("optional arg = %q", got)
	}
	if got := grammarArgs(g, argRule{vals: []string{"x"}}); got != one+"{0,4}" {
		t.Errorf("repeated arg = %q", got)
	}
	if got := grammarArgs(g, argRule{vals: []string{"x"}, min: 1}); got != one+"{1,4}" {
		t.Errorf("required repeated arg = %q", got)
	}
	if !slices.Contains(g.rules, one+` ::= " " ( "x" )`) {
		t.Errorf("rules = %q", g.rules)
	}
}

func TestGrammarDefaultTrueBool(t *testing.T) {
	spec := []*CmdSpec{{Path: "add", Flags: []FlagSpec{{Name: "labels", Type: "bool", Default: "true"}}}}
	if g := grammar(spec, testVocab(), "add things"); !strings.Contains(g, `" --labels=false"`) {
		t.Errorf("grammar lacks --labels=false: %s", g)
	}
}

func TestLoadVocabErrors(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vocab.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	fetch := func() (map[string][]string, error) { return map[string][]string{"cell_type": {"PEN"}}, nil }
	if _, err := LoadVocab(path, time.Hour, fetch); err != nil {
		t.Errorf("a damaged cache should be refetched: %v", err)
	}
	if _, err := LoadVocab(path, time.Hour, func() (map[string][]string, error) { return nil, errors.New("offline") }); err != nil {
		t.Errorf("the rewritten cache should be reused: %v", err)
	}

	blocker := filepath.Join(dir, "file")
	os.WriteFile(blocker, nil, 0o600)
	if _, err := LoadVocab(filepath.Join(blocker, "vocab.json"), time.Hour, fetch); err == nil {
		t.Error("expected an error when the cache directory cannot be created")
	}
}
