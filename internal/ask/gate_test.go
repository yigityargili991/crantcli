// Copyright 2026 Yigit Yargili
// SPDX-License-Identifier: Apache-2.0

package ask

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
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
		{"required id", CmdSpec{Path: "root-info", ArgsUse: "<root_id>", ArgsMin: 1}, withID, argRule{vals: []string{"576460752669867599", "720575940610453042"}, min: 1, max: 1}, true},
		{"required id missing", CmdSpec{Path: "root-info", ArgsUse: "<root_id>", ArgsMin: 1}, without, argRule{}, false},
		{"optional id", CmdSpec{Path: "root-info", ArgsUse: "[root_id]"}, withID, argRule{vals: []string{"576460752669867599", "720575940610453042"}, max: 1}, true},
		// ArgsMin is honored everywhere, but a flag that stands in for the
		// argument keeps the command possible (root-info --pos).
		{"required id via ArgsMin", CmdSpec{Path: "root-info", ArgsUse: "[root_id]", ArgsMin: 1}, withID, argRule{vals: []string{"576460752669867599", "720575940610453042"}, min: 1, max: 1}, true},
		{"required id via ArgsMin missing", CmdSpec{Path: "root-info", ArgsUse: "[root_id]", ArgsMin: 1}, without, argRule{}, false},
		{"required argument with a flag alternative", CmdSpec{Path: "root-info", ArgsUse: "[root_id]", ArgsMin: 1, ArgOrFlag: []string{"pos"}}, without, argRule{}, true},
		{"required argument no source", CmdSpec{Path: "mystery", ArgsMin: 1}, withID, argRule{}, false},
		{"required file missing", CmdSpec{Path: "state-transfer", ArgsUse: "<file>", ArgsMin: 1}, without, argRule{}, false},
		{"file", CmdSpec{Path: "change-def-state", ArgsUse: "[json-state]"}, withID, argRule{vals: []string{"a.json"}, max: 1}, true},
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

func TestGrammarCleanGateIsLabelsOnly(t *testing.T) {
	spec := []*CmdSpec{{Path: "check-cave", Flags: []FlagSpec{{Name: "all", Type: "bool", Default: "false"}}}}
	if g := grammar(spec, testVocab(), "check every neuron"); !strings.Contains(g, `" --all"`) {
		t.Errorf("grammar lacks --all for check-cave: %s", g)
	}
}

// grammarRegexp turns a grammar from grammar() into an anchored regexp by
// inlining its rules, which works because no rule refers to itself.
func grammarRegexp(g string) *regexp.Regexp {
	rules := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(g), "\n") {
		name, body, _ := strings.Cut(line, " ::= ")
		rules[name] = body
	}
	tok := regexp.MustCompile(`"(?:\\.|[^"\\])*"|\{\d+,\d+\}|\w+|\S`)
	var expand func(string) string
	expand = func(body string) string {
		var b strings.Builder
		for _, s := range tok.FindAllString(body, -1) {
			switch lit, err := strconv.Unquote(s); {
			case err == nil:
				b.WriteString(regexp.QuoteMeta(lit))
			case s == "(":
				b.WriteString("(?:")
			case rules[s] != "":
				b.WriteString("(?:" + expand(rules[s]) + ")")
			default:
				b.WriteString(s)
			}
		}
		return b.String()
	}
	return regexp.MustCompile("^(?:" + expand(rules["root"]) + ")$")
}

func TestGrammarLabelsNeedsClean(t *testing.T) {
	spec, v := testSpec(), testVocab()
	tests := []struct {
		q              string
		accept, reject []string
	}{
		// Cleaning words need --clean; --all additionally needs "all".
		{"delete label sources older than 24h",
			[]string{" labels --clean", " labels --clean --older-than 24h"},
			[]string{" labels", " labels --all", " labels --older-than 24h", " labels --all --clean", " labels --all --clean --older-than 24h"}},
		{"delete all label sources older than 24h",
			[]string{" labels --clean", " labels --all --clean", " labels --clean --older-than 24h", " labels --all --clean --older-than 24h"},
			[]string{" labels", " labels --all", " labels --older-than 24h", " labels --all --older-than 24h"}},
		{"erase every tracked label source regardless of age",
			[]string{" labels --all --clean"},
			[]string{" labels", " labels --all", " labels --clean --older-than 24h"}},
		// "wipe" counts as a cleaning word too, but not as evidence of "all".
		{"wipe label sources older than 24h",
			[]string{" labels --clean", " labels --clean --older-than 24h"},
			[]string{" labels", " labels --all --clean", " labels --older-than 24h"}},
		// A negated cleaning word is not a request to clean.
		{"don't clean the label sources",
			[]string{" labels"},
			[]string{" labels --clean", " labels --all --clean"}},
		{"show the label sources without cleaning them",
			[]string{" labels"},
			[]string{" labels --clean"}},
	}
	for _, tt := range tests {
		re := grammarRegexp(grammar(spec, v, tt.q))
		for _, line := range tt.accept {
			if !re.MatchString(line) {
				t.Errorf("grammar for %q rejects %q", tt.q, line)
			}
		}
		for _, line := range tt.reject {
			if re.MatchString(line) {
				t.Errorf("grammar for %q accepts %q", tt.q, line)
			}
		}
	}
}

func TestCleanEvidence(t *testing.T) {
	tests := []struct {
		q          string
		clean, all bool
	}{
		{"clean up old label sources", true, false},
		{"delete label sources older than 24h", true, false},
		{"wipe the old label sources", true, false},
		{"erase every label source", true, true},
		{"prune all label sources regardless of age", true, true},
		{"remove label sources at any age", true, true},
		{"don't clean the label sources", false, false},
		{"do not delete label sources", false, false},
		{"print the label sources without cleaning them", false, false},
		{"delete the old label sources but don't wipe everything", true, false},
	}
	for _, tt := range tests {
		e := gatherEvidence(testVocab(), tt.q)
		if e.boolOK["clean"] != tt.clean || e.cleanAll != tt.all {
			t.Errorf("evidence(%q) clean=%v cleanAll=%v, want %v %v",
				tt.q, e.boolOK["clean"], e.cleanAll, tt.clean, tt.all)
		}
	}
}

func TestGrammarRootInfoArgumentAlternatives(t *testing.T) {
	spec, v := testSpec(), testVocab()
	tests := []struct {
		name, q        string
		accept, reject []string
	}{
		{"root id form",
			"show the info for 576460752669867599",
			[]string{" root-info 576460752669867599", " root-info 576460752669867599 --json"},
			[]string{" root-info", " root-info --json", " root-info 576460752669867599 --pos 1,2,3"}},
		{"position form",
			"which column is 1,2,3 in",
			[]string{" root-info --pos 1,2,3"},
			[]string{" root-info", " root-info --json", " root-info 576460752669867599 --pos 1,2,3", " root-info --json --pos 1,2,3", " root-info --pos 1,2,3 --json"}},
		{"both evidenced",
			"which column is 576460752669867599 at 1,2,3",
			[]string{" root-info --pos 1,2,3", " root-info 576460752669867599"},
			[]string{" root-info", " root-info 576460752669867599 --pos 1,2,3"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			re := grammarRegexp(grammar(spec, v, tt.q))
			for _, line := range tt.accept {
				if !re.MatchString(line) {
					t.Errorf("grammar for %q rejects %q", tt.q, line)
				}
			}
			for _, line := range tt.reject {
				if re.MatchString(line) {
					t.Errorf("grammar for %q accepts %q", tt.q, line)
				}
			}
		})
	}
}

func TestGrammarMutuallyExclusiveFlagGroup(t *testing.T) {
	root := &cobra.Command{Use: "crantcli"}
	state := &cobra.Command{Use: "state", Run: func(*cobra.Command, []string) {},
		Annotations: map[string]string{"mutuallyExclusive": "builtin,show,reset"}}
	state.Flags().Bool("builtin", false, "")
	state.Flags().Bool("json", false, "")
	state.Flags().Bool("show", false, "")
	state.Flags().Bool("reset", false, "")
	root.AddCommand(state)
	spec := BuildSpec(root)
	if got := spec[0].ExclusiveFlags; !reflect.DeepEqual(got, []string{"builtin", "show", "reset"}) {
		t.Fatalf("ExclusiveFlags = %q", got)
	}
	re := grammarRegexp(grammar(spec, testVocab(), "print the default state"))
	for _, line := range []string{
		" state", " state --json", " state --builtin", " state --show", " state --reset",
		" state --builtin --json", " state --json --show",
	} {
		if !re.MatchString(line) {
			t.Errorf("grammar rejects %q", line)
		}
	}
	for _, line := range []string{
		" state --builtin --show", " state --show --reset", " state --builtin --reset",
		" state --builtin --json --show", " state --builtin --show --reset",
	} {
		if re.MatchString(line) {
			t.Errorf("grammar accepts %q", line)
		}
	}
}

// TestGrammarRequiredArgumentFlagAlternatives covers a command that needs its
// argument or one of several mode flags (change-def-state): the grammar must
// offer each valid form and never the bare command the CLI rejects.
func TestGrammarRequiredArgumentFlagAlternatives(t *testing.T) {
	spec := []*CmdSpec{{
		Path: "change-def-state", ArgsUse: "[json-state]", ArgsMin: 1,
		ArgOrFlag: []string{"builtin", "reset", "show"},
		Flags: []FlagSpec{
			{Name: "builtin", Type: "bool", Default: "false"},
			{Name: "reset", Type: "bool", Default: "false"},
			{Name: "show", Type: "bool", Default: "false"},
		},
	}}
	v := testVocab()
	tests := []struct {
		name, q        string
		accept, reject []string
	}{
		{"no json in request",
			"print the default state",
			[]string{" change-def-state --builtin", " change-def-state --show", " change-def-state --reset"},
			[]string{" change-def-state", " change-def-state --builtin --show"}},
		{"json file in request",
			"set the default state from saved.json",
			[]string{" change-def-state saved.json"},
			[]string{" change-def-state", " change-def-state saved.json --builtin", " change-def-state saved.json --show"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			re := grammarRegexp(grammar(spec, v, tt.q))
			for _, line := range tt.accept {
				if !re.MatchString(line) {
					t.Errorf("grammar for %q rejects %q", tt.q, line)
				}
			}
			for _, line := range tt.reject {
				if re.MatchString(line) {
					t.Errorf("grammar for %q accepts %q", tt.q, line)
				}
			}
		})
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
