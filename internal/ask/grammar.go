package ask

import (
	"fmt"
	"sort"
	"strings"
)

// Flags the grammar never offers: label hosting internals, --bundle (which
// aliases --region), and --clipboard-ids (which changes where add reads its
// arguments from, rather than a filter the request can evidence).
var skipFlags = map[string]bool{"labels-hook": true, "labels-ttl": true, "bundle": true, "clipboard-ids": true}

// gbnf collects named GBNF rules, reusing a rule when its body repeats.
type gbnf struct {
	rules []string
	n     int
	memo  map[string]string
}

func (g *gbnf) rule(body string) string {
	if name, ok := g.memo[body]; ok {
		return name
	}
	g.n++
	name := fmt.Sprintf("r%d", g.n)
	g.memo[body] = name
	g.rules = append(g.rules, name+" ::= "+body)
	return name
}

// grammar builds a GBNF grammar for one request that accepts only canonical
// command lines (" <command> <args> --flags in name order") whose values come
// from the request itself or from table values retrieved for it.
func grammar(spec []*CmdSpec, v *Vocab, q string) string {
	e := gatherEvidence(v, q)
	g := &gbnf{memo: map[string]string{}}
	var cmds []string
	for _, c := range spec {
		// "nearest column" / "which column" requests only fit the column commands.
		if e.column && c.Path != "root-info" && c.Path != "side-check" {
			continue
		}
		for _, body := range commandForms(g, c, e) {
			cmds = append(cmds, g.rule(body))
		}
	}
	if len(cmds) == 0 {
		// Nothing is possible for this request; accept only an empty suffix so
		// the model's answer fails to parse instead of inventing a command.
		g.rules = append(g.rules, `root ::= " "`)
	} else {
		g.rules = append([]string{`root ::= " " ( ` + strings.Join(cmds, " | ") + ` )`}, g.rules...)
	}
	return strings.Join(g.rules, "\n") + "\n"
}

// commandForms returns one canonical body per way the command can satisfy its
// required argument: with the positional argument, or with a flag that stands
// in for it (root-info takes a root_id or --pos, but not both).
func commandForms(g *gbnf, c *CmdSpec, e evidence) []string {
	args, ok := gatedArgs(c, e)
	if !ok {
		return nil
	}
	if len(c.ArgOrFlag) == 0 || c.ArgsMin == 0 {
		body := gLit(c.Path)
		if a := grammarArgs(g, args); a != "" {
			body += " " + a
		}
		if f := grammarFlags(g, c, e, nil); f != "" {
			body += " " + f
		}
		return []string{body}
	}
	var forms []string
	// The argument form, without any of the alternative flags.
	if len(args.vals) > 0 {
		body := gLit(c.Path)
		if a := grammarArgs(g, args); a != "" {
			body += " " + a
		}
		if f := grammarFlags(g, c, e, nameSet(c.ArgOrFlag)); f != "" {
			body += " " + f
		}
		forms = append(forms, body)
	}
	// One form per alternative flag: the flag alone, no positional argument and
	// no other flags. Ask cannot see how the command combines the flag with its
	// remaining flags (root-info rejects --history-limit with --pos), so the
	// form stays minimal to guarantee the CLI accepts it.
	for _, name := range c.ArgOrFlag {
		f := c.flag(name)
		if f == nil || !flagOffered(c, f, e) {
			continue
		}
		item, ok := grammarFlagItem(g, f, e)
		if !ok {
			continue // the request gives no value for the flag
		}
		forms = append(forms, gLit(c.Path)+" "+item)
	}
	return forms
}

// nameSet returns names as a set for skipping flags.
func nameSet(names []string) map[string]bool {
	if len(names) == 0 {
		return nil
	}
	set := make(map[string]bool, len(names))
	for _, name := range names {
		set[name] = true
	}
	return set
}

// gLit is a GBNF string literal.
func gLit(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

func gAlt(vals []string) string {
	var parts []string
	for _, x := range uniq(vals) {
		parts = append(parts, gLit(shellQuote(x)))
	}
	return "( " + strings.Join(parts, " | ") + " )"
}

func grammarArgs(g *gbnf, a argRule) string {
	if len(a.vals) == 0 {
		return ""
	}
	one := g.rule(`" " ` + gAlt(a.vals))
	switch {
	case a.max == 1 && a.min == 1:
		return one
	case a.max == 1:
		return one + "?"
	case a.min == 1:
		return one + "{1,4}"
	default:
		return one + "{0,4}"
	}
}

// flagItem is one flag in a command's flag grammar: the rule accepting it and
// whether that form must include it.
type flagItem struct {
	name     string
	rule     string
	required bool
}

// flagOffered reports whether the evidence lets the grammar offer f on c.
func flagOffered(c *CmdSpec, f *FlagSpec, e evidence) bool {
	if skipFlags[f.Name] {
		return false
	}
	if ok, gated := e.boolOK[f.Name]; gated && !ok {
		return false
	}
	// A command's --clean flag is its cleanup switch: its other flags only
	// apply with it, and --all deletes everything, so it needs "all" evidence.
	if c.flag("clean") != nil && f.Name != "clean" {
		return e.boolOK["clean"] && (f.Name != "all" || e.cleanAll)
	}
	return true
}

// grammarFlags returns the flags part of a command line: any ordered subset of
// the offered flags, in name order, honoring required flags and the command's
// mutually exclusive flag groups. omit leaves out flags that another command
// form owns. A command's --clean flag is required when the request asks to
// clean: the subcommand it replaced is retired, so cleaning must pass it.
func grammarFlags(g *gbnf, c *CmdSpec, e evidence, omit map[string]bool) string {
	names := make([]string, 0, len(c.Flags))
	for _, f := range c.Flags {
		names = append(names, f.Name)
	}
	sort.Strings(names)
	var items []flagItem
	for _, name := range names {
		f := c.flag(name)
		if omit[name] || !flagOffered(c, f, e) {
			continue
		}
		item, ok := grammarFlagItem(g, f, e)
		if !ok {
			continue
		}
		items = append(items, flagItem{
			name:     name,
			rule:     item,
			required: name == "clean" && e.boolOK["clean"],
		})
	}
	return grammarFlagSequence(g, items, c.ExclusiveFlags)
}

// grammarFlagItem returns the rule accepting one flag and its evidenced value.
func grammarFlagItem(g *gbnf, f *FlagSpec, e evidence) (string, bool) {
	switch {
	case f.Type == "bool" && f.Default == "true":
		return g.rule(gLit(" --" + f.Name + "=false")), true
	case f.Type == "bool":
		return g.rule(gLit(" --" + f.Name)), true
	}
	vals := gatedValues(*f, e)
	if len(vals) == 0 {
		return "", false
	}
	one := gLit(" --"+f.Name+" ") + " " + gAlt(vals)
	if f.Type == "stringArray" {
		return g.rule("( " + one + " )+"), true
	}
	return g.rule(one), true
}

// grammarFlagSequence builds a rule accepting the flags grammarFlags offered.
// Without an exclusive group that is any ordered subset of items containing
// every required item. With one, only at most one of its members may appear,
// so the sequence is the choice of no member or exactly one, each choice
// honoring the required items.
func grammarFlagSequence(g *gbnf, items []flagItem, exclusive []string) string {
	if len(items) == 0 {
		return ""
	}
	// Only members that this form actually offers constrain the sequence.
	present := map[string]bool{}
	for _, item := range items {
		present[item.name] = true
	}
	var members []string
	for _, name := range exclusive {
		if present[name] {
			members = append(members, name)
		}
	}
	if len(members) == 0 {
		return grammarRequiredSubset(g, items)
	}
	group := nameSet(members)
	var choices []string
	empty, nullable := false, false
	add := func(chosen string) {
		var kept []flagItem
		for _, item := range items {
			if !group[item.name] {
				kept = append(kept, item)
				continue
			}
			if item.name == chosen {
				item.required = true
				kept = append(kept, item)
			}
		}
		if len(kept) == 0 {
			empty = true
			return
		}
		body := grammarRequiredSubset(g, kept)
		if body == "" {
			empty = true
			return
		}
		if !hasRequired(kept) {
			// The choice body already accepts the empty suffix, so the whole
			// expression is optional without an extra "?".
			nullable = true
		}
		choices = append(choices, body)
	}
	add("")
	for _, name := range members {
		add(name)
	}
	if len(choices) == 0 {
		return ""
	}
	body := choices[0]
	if len(choices) > 1 {
		body = "( " + strings.Join(choices, " | ") + " )"
	}
	if empty && !nullable {
		body += "?"
	}
	return body
}

// grammarRequiredSubset compiles a rule accepting any ordered subset of items
// that contains every required item. It returns "" for no items.
func grammarRequiredSubset(g *gbnf, items []flagItem) string {
	last := -1
	for i, item := range items {
		if item.required {
			last = i
		}
	}
	if last < 0 {
		return grammarSubset(g, items) + "?"
	}
	body := items[last].rule
	if tail := grammarSubset(g, items[last+1:]); tail != "" {
		body = g.rule(body + " " + tail + "?")
	}
	for i := last - 1; i >= 0; i-- {
		if items[i].required {
			body = g.rule(items[i].rule + " " + body)
		} else {
			body = g.rule(items[i].rule + " " + body + " | " + body)
		}
	}
	return body
}

// grammarSubset compiles a rule accepting any ordered subset of items,
// returning "" for no items. It never accepts the empty sequence.
func grammarSubset(g *gbnf, items []flagItem) string {
	if len(items) == 0 {
		return ""
	}
	first := make([]string, len(items))
	for i := len(items) - 1; i >= 0; i-- {
		body := items[i].rule
		if i+1 < len(items) {
			body = fmt.Sprintf("%s %s? | %s", items[i].rule, first[i+1], first[i+1])
		}
		first[i] = g.rule(body)
	}
	return first[0]
}

// hasRequired reports whether any item must appear.
func hasRequired(items []flagItem) bool {
	for _, item := range items {
		if item.required {
			return true
		}
	}
	return false
}
