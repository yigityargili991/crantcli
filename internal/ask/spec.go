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
	Path    string // "add", "completion bash"
	ArgsUse string
	ArgsMin int // 1 when the command's Args validator rejects zero arguments
	// ArgOrFlag names flags that stand in for the required positional
	// argument: the command needs the argument or exactly one of these flags
	// (root-info takes a root_id or --pos, change-def-state a JSON state or one
	// of its mode flags). Cobra does not export its Args validators, so
	// requiredArgs derives this by probing the validator.
	ArgOrFlag []string
	// ExclusiveFlags names flags of which at most one may be used, read from
	// the command's "mutuallyExclusive" annotation: Cobra does not export the
	// groups passed to MarkFlagsMutuallyExclusive.
	ExclusiveFlags []string
	Flags          []FlagSpec
}

var classificationFlags = map[string]string{
	"super-class": "super_class", "cell-class": "cell_class", "cell-type": "cell_type",
	"cell-subtype": "cell_subtype", "side": "side", "region": "region", "bundle": "region",
	"tract": "tract", "nerve": "nerve", "hemilineage": "hemilineage", "proofread": "proofread",
}

// BuildSpec lists the runnable commands under root, sorted by path. Commands
// named in skip (such as "ask" itself) are left out and so are hidden,
// deprecated, help, and completion commands: ask must never suggest a retired
// command, even one that is merely deprecated without being hidden.
func BuildSpec(root *cobra.Command, skip ...string) []*CmdSpec {
	skipped := map[string]bool{}
	for _, s := range skip {
		skipped[s] = true
	}
	var out []*CmdSpec
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, child := range c.Commands() {
			if child.Hidden || child.Deprecated != "" || child.Name() == "help" || child.Name() == "completion" {
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

// Commands may carry these annotations for ask, in addition to requiresToken:
//
//	mutuallyExclusive = "builtin,show,reset"
//	    At most one of the named flags may be on the command line. Cobra's
//	    MarkFlagsMutuallyExclusive does not export its groups.
//
// The Args validators are read by probing them in requiredArgs, since Cobra
// does not export those either.
func specFor(c *cobra.Command) *CmdSpec {
	s := &CmdSpec{Path: commandPath(c), ExclusiveFlags: exclusiveFlags(c)}
	s.ArgsMin, s.ArgOrFlag = requiredArgs(c)
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

// exclusiveFlags reads the "mutuallyExclusive" annotation: comma-separated
// flag names of which at most one may be used.
func exclusiveFlags(c *cobra.Command) []string {
	value := strings.TrimSpace(c.Annotations["mutuallyExclusive"])
	if value == "" {
		return nil
	}
	var names []string
	for _, name := range strings.Split(value, ",") {
		if name = strings.TrimSpace(name); name != "" {
			names = append(names, name)
		}
	}
	return names
}

// requiredArgs reports whether the command's Args validator requires at least
// one positional argument and, when it does, which flags can stand in for that
// argument instead. A flag stands in when the validator accepts the command
// with no arguments and only that flag set (root-info's --pos, change-def-
// state's mode flags).
//
// Validators only inspect the command and arguments, so calling one is safe;
// RunE is never called. Only Changed is set while probing: a validator that
// needs the flag's value (rather than its presence) to accept the command is
// not discovered. Changed is restored afterwards, so the command keeps the
// state it had before BuildSpec.
func requiredArgs(c *cobra.Command) (int, []string) {
	if c.Args == nil || c.Args(c, nil) == nil {
		return 0, nil
	}
	var alternatives []string
	c.LocalFlags().VisitAll(func(f *pflag.Flag) {
		if f.Name == "help" || f.Hidden {
			return
		}
		changed := f.Changed
		f.Changed = true
		noArgs := c.Args(c, nil)
		f.Changed = changed
		if noArgs == nil {
			alternatives = append(alternatives, f.Name)
		}
	})
	return 1, alternatives
}

// commandPath is the command's path below the root ("completion bash").
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
