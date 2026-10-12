package ask

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// VocabFields are the table fields whose values can be matched in a request.
var VocabFields = []string{"super_class", "cell_class", "cell_type", "cell_subtype", "side", "region", "tract", "nerve"}

// Vocab indexes live CRANT table values by the surface forms a request may
// use for them.
type Vocab struct {
	values map[string]map[string]bool // field -> value set
	index  map[string][]ref           // normalized surface form -> values
}

type ref struct {
	Field, Value string
	Gloss        bool
}

// NewVocab indexes the given values (field -> values) and the glossary
// phrases whose values exist in them.
func NewVocab(values map[string][]string) *Vocab {
	v := &Vocab{values: map[string]map[string]bool{}, index: map[string][]ref{}}
	for field, vals := range values {
		set := map[string]bool{}
		for _, x := range vals {
			set[x] = true
		}
		v.values[field] = set
	}
	// Values keep their given order (most neurons first, as fetched), which
	// orders same-field candidates in the prompt the way training saw them.
	for _, field := range VocabFields {
		for _, x := range values[field] {
			if strings.ContainsAny(x, "?;") || len(x) < 2 {
				continue
			}
			v.add(x, ref{Field: field, Value: x})
			v.add(strings.ReplaceAll(x, "_", " "), ref{Field: field, Value: x})
		}
	}
	phrases := make([]string, 0, len(glossary))
	for p := range glossary {
		phrases = append(phrases, p)
	}
	sort.Strings(phrases)
	for _, p := range phrases {
		for _, r := range glossary[p] {
			if v.values[r.Field][r.Value] {
				v.add(p, ref{Field: r.Field, Value: r.Value, Gloss: true})
			}
		}
	}
	return v
}

var nonWord = regexp.MustCompile(`[^a-z0-9_/?\-]+`)

func normalize(s string) string {
	s = strings.ToLower(s)
	s = nonWord.ReplaceAllString(s, " ")
	return strings.Join(strings.Fields(s), " ")
}

func (v *Vocab) add(surface string, r ref) {
	surface = normalize(surface)
	if surface == "" {
		return
	}
	for _, form := range []string{surface, surface + "s"} {
		dup := false
		for _, x := range v.index[form] {
			if x == r {
				dup = true
			}
		}
		if !dup {
			v.index[form] = append(v.index[form], r)
		}
	}
}

// candidate is one retrieved table value for a span of the request.
type candidate struct {
	Span         string
	Field, Value string
}

// retrieve finds table values mentioned in the request, longest match first.
func (v *Vocab) retrieve(q string) []candidate {
	toks := v.queryTokens(q)
	covered := make([]bool, len(toks))
	var out []candidate
	for n := 5; n >= 1; n-- {
		for i := 0; i+n <= len(toks); i++ {
			skip := false
			for j := i; j < i+n; j++ {
				if covered[j] {
					skip = true
				}
			}
			if skip {
				continue
			}
			span := strings.Join(toks[i:i+n], " ")
			refs := v.index[span]
			if len(refs) == 0 || stopword[span] {
				continue
			}
			for j := i; j < i+n; j++ {
				covered[j] = true
			}
			for _, r := range refs {
				out = append(out, candidate{Span: span, Field: r.Field, Value: r.Value})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return fieldRank[out[i].Field] < fieldRank[out[j].Field] })
	return out
}

var fieldRank = map[string]int{"super_class": 0, "cell_class": 1, "cell_type": 2, "cell_subtype": 3,
	"side": 4, "region": 5, "tract": 6, "nerve": 7}

// English words that collide with short codes in the table.
var stopword = map[string]bool{"an": true, "a": true, "in": true, "of": true, "or": true, "on": true,
	"all": true, "the": true, "and": true, "is": true, "it": true}

// queryTokens normalizes a request for lookup. "?" and "-" stay inside tokens
// because some table values contain them ("PEN?", "OL-POTu"), but sentence
// punctuation ("ER2?") and hyphenated phrases ("left-side") are split off when
// the whole token is not a known value.
func (v *Vocab) queryTokens(q string) []string {
	var out []string
	for _, t := range strings.Fields(normalize(q)) {
		if len(v.index[t]) > 0 {
			out = append(out, t)
			continue
		}
		t = strings.TrimRight(t, "?")
		if len(v.index[t]) == 0 && strings.Contains(t, "-") {
			for _, part := range strings.Split(t, "-") {
				if part != "" {
					out = append(out, part)
				}
			}
			continue
		}
		if t != "" {
			out = append(out, t)
		}
	}
	return out
}

// prompt is the compact input the fine-tuned model sees: the request and the
// table values retrieval linked to it.
func (v *Vocab) prompt(q string) string {
	var b strings.Builder
	b.WriteString("request: " + q + "\ntable:")
	seen := map[string]bool{}
	for _, c := range v.retrieve(q) {
		key := c.Field + "=" + c.Value
		if seen[key] {
			continue
		}
		seen[key] = true
		b.WriteString(" " + key + " (" + strconv.Quote(c.Span) + ");")
	}
	b.WriteString("\ncommand:")
	return b.String()
}
