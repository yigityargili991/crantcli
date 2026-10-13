package ask

import (
	"fmt"
	"strings"
)

// Completer continues a prompt under a GBNF grammar. The model engine
// implements it; tests use a fake.
type Completer interface {
	Complete(prompt, grammar string) (string, error)
}

// Suggest asks the model for the command that fulfils the request and
// returns its argv, without the leading "crantcli".
func Suggest(m Completer, spec []*CmdSpec, v *Vocab, request string) ([]string, error) {
	q := stripPurpose(strings.Join(strings.Fields(request), " "))
	if q == "" {
		return nil, fmt.Errorf("empty request")
	}
	text, err := m.Complete(v.prompt(q), grammar(spec, v, q))
	if err != nil {
		return nil, fmt.Errorf("running the ask model: %w", err)
	}
	n, err := parseLine(spec, strings.TrimSpace(text))
	if err != nil {
		return nil, fmt.Errorf("the ask model wrote an invalid command %q: %w", strings.TrimSpace(text), err)
	}
	// Repeated identical values ("--cell-type PEG --cell-type PEG") never
	// change what a command does.
	for k, vals := range n.Flags {
		n.Flags[k] = uniq(vals)
	}
	return n.argv(spec), nil
}
