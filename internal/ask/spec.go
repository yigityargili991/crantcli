// Package ask turns a plain-English request into a crantcli command line with
// a small fine-tuned language model. Table values found in the request are
// shown to the model, and a per-request grammar only lets it write commands,
// flags and values that the request gives evidence for.
package ask

import (
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// FlagSpec describes one flag of a runnable command.
type FlagSpec struct {
	Name, Short, Type, Default string
	Field                      string // classification field the flag filters on
}

// CmdSpec describes one runnable command.
type CmdSpec struct {
	Path    string // "add", "labels clean"
	ArgsUse string
	ArgsMin int // 1 when the command's Args validator rejects zero arguments
	Flags   []FlagSpec
}

var classificationFlags = map[string]string{
	"super-class": "super_class", "cell-class": "cell_class", "cell-type": "cell_type",
	"cell-subtype": "cell_subtype", "side": "side", "region": "region", "bundle": "region",
	"tract": "tract", "nerve": "nerve", "hemilineage": "hemilineage", "proofread": "proofread",
}

// BuildSpec lists the runnable commands under root, sorted by path. Commands
// named in skip (such as "ask" itself) are left out.
func BuildSpec(root *cobra.Command, skip ...string) []*CmdSpec {
	skipped := map[string]bool{}
	for _, s := range skip {
		skipped[s] = true
	}
	var out []*CmdSpec
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, child := range c.Commands() {
			if child.Hidden || child.Name() == "help" || child.Name() == "completion" {
				continue
			}
			s := specFor(child)
			if skipped[s.Path] {
				continue
			}
			if child.Runnable() {
				out = append(out, s)
			}
			walk(child)
		}
	}
	walk(root)
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func specFor(c *cobra.Command) *CmdSpec {
	s := &CmdSpec{Path: commandPath(c), ArgsMin: minArgs(c)}
	if parts := strings.SplitN(c.Use, " ", 2); len(parts) == 2 {
		s.ArgsUse = strings.TrimSpace(strings.ReplaceAll(parts[1], "[flags]", ""))
	}
	c.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if f.Name == "help" || f.Hidden {
			return
		}
		s.Flags = append(s.Flags, FlagSpec{Name: f.Name, Short: f.Shorthand, Type: f.Value.Type(),
			Default: f.DefValue, Field: classificationFlags[f.Name]})
	})
	return s
}

// minArgs reports whether the command's Args validator requires at least one
// positional argument. Validators only inspect the command and arguments, so
// calling one with no arguments is safe; RunE is never called.
func minArgs(c *cobra.Command) int {
	if c.Args != nil && c.Args(c, nil) != nil {
		return 1
	}
	return 0
}

// commandPath is the command's path below the root ("labels clean").
func commandPath(c *cobra.Command) string {
	var names []string
	for ; c != nil && c.HasParent(); c = c.Parent() {
		names = append([]string{c.Name()}, names...)
	}
	return strings.Join(names, " ")
}

func (s *CmdSpec) flag(name string) *FlagSpec {
	for i := range s.Flags {
		if s.Flags[i].Name == name || (s.Flags[i].Short != "" && s.Flags[i].Short == name) {
			return &s.Flags[i]
		}
	}
	return nil
}

func findCmd(spec []*CmdSpec, path string) *CmdSpec {
	for _, s := range spec {
		if s.Path == path {
			return s
		}
	}
	return nil
}
