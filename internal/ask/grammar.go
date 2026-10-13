package ask

import (
	"fmt"
	"sort"
	"strings"
)

// Flags the grammar never offers: label hosting internals and --bundle,
// which aliases --region.
var skipFlags = map[string]bool{"labels-hook": true, "labels-ttl": true, "bundle": true}

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
		args, ok := gatedArgs(c, e)
		if !ok {
			continue
		}
		// "nearest column" / "which column" requests only fit the column commands.
		if e.column && c.Path != "lookup-column" && c.Path != "side-check" {
			continue
		}
		body := gLit(c.Path)
		if a := grammarArgs(g, args); a != "" {
			body += " " + a
		}
		if f := grammarFlags(g, c, e); f != "" {
			body += " " + f
		}
		cmds = append(cmds, g.rule(body))
	}
	g.rules = append([]string{`root ::= " " ( ` + strings.Join(cmds, " | ") + ` )`}, g.rules...)
	return strings.Join(g.rules, "\n") + "\n"
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
	default:
		return one + "{0,4}"
	}
}

func grammarFlags(g *gbnf, c *CmdSpec, e evidence) string {
	names := make([]string, 0, len(c.Flags))
	for _, f := range c.Flags {
		names = append(names, f.Name)
	}
	sort.Strings(names)
	var items []string
	for _, name := range names {
		f := c.flag(name)
		if ok, gated := e.boolOK[name]; skipFlags[name] || gated && !ok {
			continue
		}
		var item string
		switch {
		case f.Type == "bool" && f.Default == "true":
			item = gLit(" --" + name + "=false")
		case f.Type == "bool":
			item = gLit(" --" + name)
		default:
			vals := gatedValues(*f, e)
			if len(vals) == 0 {
				continue
			}
			one := gLit(" --"+name+" ") + " " + gAlt(vals)
			if f.Type == "stringArray" {
				item = "( " + one + " )+"
			} else {
				item = one
			}
		}
		items = append(items, g.rule(item))
	}
	if len(items) == 0 {
		return ""
	}
	// Any ordered subset of the flags, in name order.
	first := make([]string, len(items))
	for i := len(items) - 1; i >= 0; i-- {
		body := items[i]
		if i+1 < len(items) {
			body = fmt.Sprintf("%s %s? | %s", items[i], first[i+1], first[i+1])
		}
		first[i] = g.rule(body)
	}
	return first[0] + "?"
}
