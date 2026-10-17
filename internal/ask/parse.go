package ask

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// command is a parsed command line: the command path, its positional
// arguments, and its non-default flag values.
type command struct {
	Cmd   string
	Args  []string
	Flags map[string][]string
}

func shellSplit(s string) ([]string, error) {
	var out []string
	var cur strings.Builder
	inTok, quote, escaped := false, rune(0), false
	for _, r := range s {
		switch {
		case escaped:
			cur.WriteRune(r)
			inTok, escaped = true, false
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\\':
			// Outside quotes, consume an escape such as shellQuote's '\''.
			escaped = true
		case r == '\'' || r == '"':
			quote, inTok = r, true
		case r == ' ' || r == '\t' || r == '\n':
			if inTok {
				out = append(out, cur.String())
				cur.Reset()
				inTok = false
			}
		default:
			cur.WriteRune(r)
			inTok = true
		}
	}
	if escaped {
		return nil, errors.New("trailing escape")
	}
	if quote != 0 {
		return nil, errors.New("unterminated quote")
	}
	if inTok {
		out = append(out, cur.String())
	}
	return out, nil
}

func parseLine(spec []*CmdSpec, line string) (command, error) {
	toks, err := shellSplit(line)
	if err != nil {
		return command{}, err
	}
	return parseArgv(spec, toks)
}

func parseArgv(spec []*CmdSpec, toks []string) (command, error) {
	if len(toks) > 0 && toks[0] == "crantcli" {
		toks = toks[1:]
	}
	if len(toks) == 0 {
		return command{}, errors.New("empty command")
	}
	var c *CmdSpec
	if len(toks) >= 2 {
		c = findCmd(spec, toks[0]+" "+toks[1])
		if c != nil {
			toks = toks[2:]
		}
	}
	if c == nil {
		c = findCmd(spec, toks[0])
		if c == nil {
			return command{}, fmt.Errorf("unknown command %q", toks[0])
		}
		toks = toks[1:]
	}
	n := command{Cmd: c.Path, Flags: map[string][]string{}}
	for i := 0; i < len(toks); i++ {
		t := toks[i]
		if !strings.HasPrefix(t, "-") || len(t) == 1 {
			n.Args = append(n.Args, t)
			continue
		}
		name, val, hasVal := strings.Cut(strings.TrimLeft(t, "-"), "=")
		f := c.flag(name)
		if f == nil {
			return command{}, fmt.Errorf("unknown flag %s for %s", t, c.Path)
		}
		if f.Type == "bool" {
			if !hasVal {
				val = "true"
			}
		} else if !hasVal {
			if i+1 >= len(toks) {
				return command{}, fmt.Errorf("flag --%s needs a value", f.Name)
			}
			i++
			val = toks[i]
		}
		if isDefault(f, val) {
			continue
		}
		key := f.Name
		if key == "bundle" { // documented alias of --region
			key = "region"
		}
		if f.Type == "stringArray" {
			n.Flags[key] = append(n.Flags[key], val)
		} else {
			n.Flags[key] = []string{val}
		}
	}
	return n, nil
}

func isDefault(f *FlagSpec, val string) bool {
	if val == f.Default {
		return true
	}
	switch f.Type {
	case "bool":
		a, err1 := strconv.ParseBool(val)
		b, err2 := strconv.ParseBool(f.Default)
		return err1 == nil && err2 == nil && a == b
	case "duration":
		a, err1 := time.ParseDuration(val)
		b, err2 := time.ParseDuration(f.Default)
		return err1 == nil && err2 == nil && a == b
	}
	return false
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if strings.IndexFunc(s, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("-_./,:=+%@", r))
	}) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// argv renders a parsed command in one fixed form: command, args, then flags
// in name order. This is the form the model was trained to write.
func (n command) argv(spec []*CmdSpec) []string {
	c := findCmd(spec, n.Cmd)
	out := strings.Fields(n.Cmd)
	out = append(out, n.Args...)
	var names []string
	for k := range n.Flags {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		f := c.flag(k)
		for _, v := range n.Flags[k] {
			if f.Type == "bool" {
				if v == "true" {
					out = append(out, "--"+k)
				} else {
					out = append(out, "--"+k+"=false")
				}
				continue
			}
			out = append(out, "--"+k, v)
		}
	}
	return out
}

// QuoteArgv renders argv as a shell command line.
func QuoteArgv(argv []string) string {
	q := make([]string, len(argv))
	for i, a := range argv {
		q[i] = shellQuote(a)
	}
	return strings.Join(q, " ")
}
