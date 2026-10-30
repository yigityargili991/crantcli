// Copyright 2026 Yigit Yargili
// SPDX-License-Identifier: Apache-2.0

package ask

import (
	"reflect"
	"strings"
	"testing"
)

func TestShellSplit(t *testing.T) {
	tests := []struct {
		in      string
		want    []string
		wantErr bool
	}{
		{"add --cell-type PEN", []string{"add", "--cell-type", "PEN"}, false},
		{"add --layer 'seg v2'", []string{"add", "--layer", "seg v2"}, false},
		{`add --layer "seg v2"  --open`, []string{"add", "--layer", "seg v2", "--open"}, false},
		{"add --x ''", []string{"add", "--x", ""}, false},
		{"add\t--open\n", []string{"add", "--open"}, false},
		{"add --layer 'seg", nil, true},
		// Outside quotes a backslash escapes the next character, as in the
		// '\'' shellQuote writes for an apostrophe; inside quotes it is data.
		{`add --layer 'Bob'\''s layer'`, []string{"add", "--layer", "Bob's layer"}, false},
		{`add --layer Bob\'s\ layer`, []string{"add", "--layer", "Bob's layer"}, false},
		{`add --layer a\\b`, []string{"add", "--layer", `a\b`}, false},
		{`add --layer 'a\b' "c\d"`, []string{"add", "--layer", `a\b`, `c\d`}, false},
		{`add --layer x\`, nil, true},
	}
	for _, tt := range tests {
		got, err := shellSplit(tt.in)
		if (err != nil) != tt.wantErr || !reflect.DeepEqual(got, tt.want) {
			t.Errorf("shellSplit(%q) = %q, %v; want %q, err %v", tt.in, got, err, tt.want, tt.wantErr)
		}
	}
}

func parseSpec() []*CmdSpec {
	return []*CmdSpec{
		{Path: "add", Flags: []FlagSpec{
			{Name: "cell-type", Type: "stringArray"},
			{Name: "region", Type: "string"},
			{Name: "bundle", Type: "string"},
			{Name: "open", Type: "bool", Default: "false"},
			{Name: "labels", Type: "bool", Default: "true"},
			{Name: "older-than", Type: "duration", Default: "24h"},
			{Name: "state", Short: "s", Type: "string"},
		}},
		{Path: "labels", Flags: []FlagSpec{
			{Name: "all", Type: "bool", Default: "false"},
			{Name: "clean", Type: "bool", Default: "false"},
			{Name: "older-than", Type: "duration", Default: "168h"},
		}},
		{Path: "cave-history", ArgsUse: "<root_id>"},
	}
}

func TestParseLine(t *testing.T) {
	spec := parseSpec()
	tests := []struct {
		line    string
		want    command
		wantErr string
	}{
		{"crantcli add --cell-type PEN --cell-type EPG -s a.json", command{Cmd: "add", Flags: map[string][]string{"cell-type": {"PEN", "EPG"}, "state": {"a.json"}}}, ""},
		{"add --open=false --labels=true --older-than=1440m", command{Cmd: "add", Flags: map[string][]string{}}, ""},
		{"add --bundle BU", command{Cmd: "add", Flags: map[string][]string{"region": {"BU"}}}, ""},
		{"add --labels=false", command{Cmd: "add", Flags: map[string][]string{"labels": {"false"}}}, ""},
		{"labels --clean", command{Cmd: "labels", Flags: map[string][]string{"clean": {"true"}}}, ""},
		{"labels --all --clean", command{Cmd: "labels", Flags: map[string][]string{"all": {"true"}, "clean": {"true"}}}, ""},
		{"labels --clean --older-than 48h", command{Cmd: "labels", Flags: map[string][]string{"clean": {"true"}, "older-than": {"48h"}}}, ""},
		{"cave-history 576460752669867599 - x", command{Cmd: "cave-history", Args: []string{"576460752669867599", "-", "x"}, Flags: map[string][]string{}}, ""},
		{"", command{}, "empty command"},
		{"crantcli", command{}, "empty command"},
		{"bogus", command{}, "unknown command"},
		{"add --nope", command{}, "unknown flag"},
		{"add --region", command{}, "needs a value"},
		{"add --region 'x", command{}, "unterminated"},
	}
	for _, tt := range tests {
		got, err := parseLine(spec, tt.line)
		if tt.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("parseLine(%q) err = %v, want %q", tt.line, err, tt.wantErr)
			}
			continue
		}
		if err != nil || !reflect.DeepEqual(got, tt.want) {
			t.Errorf("parseLine(%q) = %+v, %v; want %+v", tt.line, got, err, tt.want)
		}
	}
}

func TestArgvRoundTrip(t *testing.T) {
	spec := parseSpec()
	n, err := parseLine(spec, "add --state a.json --labels=false --open --cell-type PEN --cell-type 'PB glia'")
	if err != nil {
		t.Fatal(err)
	}
	got := n.argv(spec)
	want := []string{"add", "--cell-type", "PEN", "--cell-type", "PB glia", "--labels=false", "--open", "--state", "a.json"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("argv = %q, want %q", got, want)
	}
	if q := QuoteArgv(got); q != "add --cell-type PEN --cell-type 'PB glia' --labels=false --open --state a.json" {
		t.Errorf("QuoteArgv = %q", q)
	}
}

func TestShellQuote(t *testing.T) {
	for in, want := range map[string]string{
		"":          "''",
		"PEN":       "PEN",
		"a/b_c-1.x": "a/b_c-1.x",
		"seg v2":    "'seg v2'",
		"it's":      `'it'\''s'`,
		"$(rm)":     "'$(rm)'",
	} {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}
