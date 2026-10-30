package ask

import (
	"regexp"
	"strings"
)

// Evidence gating: per request, a value may only be written if it was found
// in the request text or retrieved from the CRANT table for a span of it, and
// some flags need words that ask for them. Flags without evidence are left
// out of the grammar, so the model cannot invent filters, files, IDs, or
// options the user never asked for.

type evidence struct {
	ids, files, positions, quoted, durations, ints []string
	layers                                         []string            // unquoted layer names ("the seg v2 layer")
	values                                         map[string][]string // field -> retrieved values
	words                                          string
	boolOK                                         map[string]bool // gated bool flags -> the request asks for them
	column                                         bool            // the request asks for the nearest or containing column
	colorFields                                    map[string]bool // fields named near a color word
	cleanAll                                       bool            // the request asks to delete every label source
}

var (
	reRootID   = regexp.MustCompile(`\b\d{15,20}\b`)
	reJSON     = regexp.MustCompile(`[\w./-]+\.json\b`)
	rePos      = regexp.MustCompile(`-?\d+(?:\.\d+)?,-?\d+(?:\.\d+)?,-?\d+(?:\.\d+)?`)
	reQuoted   = regexp.MustCompile(`'([^']+)'|"([^"]+)"`)
	reDuration = regexp.MustCompile(`\b\d+(?:h|m|s|ms)\b`)
	reSmallInt = regexp.MustCompile(`\b\d{1,4}\b`)

	// "in the viewer" means open (cave-history); "viewer state" does not.
	reOpenWord    = regexp.MustCompile(`(?i)\bopen|browser|\blaunch|\bpop (?:it |that |them )?up\b|\bviewer\b`)
	reViewerState = regexp.MustCompile(`(?i)\bviewer state`)
	reOpenNeg     = regexp.MustCompile(`(?i)\b(?:without|don'?t|do not|no need to|not)\b[^.,;?!]{0,20}\bopen\w*`)
	// Cleaning words mean deleting label sources; a negated mention ("don't
	// clean") does not, and "wipe"/"erase"/"purge" count as cleaning too.
	reCleanWord = regexp.MustCompile(`(?i)\b(?:clean(?:up)?|delete|remove|prune|wipe|erase|purge)\w*`)
	reCleanNeg  = regexp.MustCompile(`(?i)\b(?:without|don'?t|do not|no need to|not|never)\b[^.,;?!]{0,20}\b(?:clean(?:up)?|delete|remove|prune|wipe|erase|purge)\w*`)
	// Deleting every source regardless of age is the only evidence for --all:
	// it is destructive, so a negated "all" does not count either.
	reAllWord = regexp.MustCompile(`(?i)\ball\b|\bevery\b|\beverything\b|regardless of (?:their |its |the )?age|\bany age\b`)
	reAllNeg  = regexp.MustCompile(`(?i)\b(?:without|don'?t|do not|no need to|not|never)\b[^.,;?!]{0,20}\b(?:all|every(?:thing)?)\b`)
	// Other bool flags the model tends to add unasked need words like these.
	boolEvidence = map[string]*regexp.Regexp{
		"count":      regexp.MustCompile(`(?i)\bcount|how many|number of|tally|per (?:value|type)`),
		"unfiltered": regexp.MustCompile(`(?i)unfilter|filter|every edit|all edits|\braw\b`),
		"intersect":  regexp.MustCompile(`(?i)\bboth\b|at once|intersect|match(?:es|ing)? (?:all|every)|all (?:of )?(?:the )?(?:given |these )?(?:classifiers|filters|criteria|conditions)|that are .* and `),
	}
	reColumn = regexp.MustCompile(`(?i)\b(?:nearest|closest|which|what)\b(?:\W+\w+){0,4}?\W+columns?\b`)
	// "layer named seg v2", "layer called merge" ...
	reLayerNamed = regexp.MustCompile(`(?i)\blayers? (?:named|called) ([^,.?!;]+?)(?: layer)?(?:[,.?!;]|\s+(?:and|with|colou?r\w*|in|on|for|to)\b|$)`)
	// ... and "on the seg v2 layer", "in the merge layer".
	reLayerBefore = regexp.MustCompile(`(?i)\b(?:on|in|into|to|onto)\s+(?:the\s+|my\s+|a\s+)?((?:[\w-]+\s+){0,2}?[\w-]+)\s+layer\b`)
	// Purpose clauses ("so I can check them") name verbs the user does not
	// ask for, so they are cut before the model sees the request.
	rePurpose = regexp.MustCompile(`(?i),?\s*\b(?:so (?:that )?(?:i|we) (?:can|could|may|might)|so i'?m|in order to|because|since (?:i|we)(?:'m|'re| am| are)?)\b[^.?!]*`)
)

func stripPurpose(q string) string {
	return strings.TrimSpace(rePurpose.ReplaceAllString(q, ""))
}

// unquotedLayers returns layer-name candidates from a request without quotes,
// each with and without a trailing " layer".
func unquotedLayers(q string) []string {
	var out []string
	for _, re := range []*regexp.Regexp{reLayerNamed, reLayerBefore} {
		for _, m := range re.FindAllStringSubmatch(q, -1) {
			if name := strings.TrimSpace(m[1]); name != "" {
				out = append(out, name, name+" layer")
			}
		}
	}
	return out
}

var (
	reNonAlnum  = regexp.MustCompile(`[^a-z0-9]+`)
	reColorWord = regexp.MustCompile(`^(?:colou?r\w*|hues?|tones?|shades?)$`)
	// How a request names each --color-by field.
	colorFieldWords = map[string]*regexp.Regexp{
		"super_class":   regexp.MustCompile(`\bsuper ?class(?:es)?\b`),
		"cell_class":    regexp.MustCompile(`\bclass(?:es)?\b`),
		"cell_type":     regexp.MustCompile(`\btypes?\b`),
		"cell_subtype":  regexp.MustCompile(`\bsub ?types?\b`),
		"cell_instance": regexp.MustCompile(`\binstances?\b`),
		"column":        regexp.MustCompile(`\bcolumns?\b`),
		"side":          regexp.MustCompile(`\bsides?\b`),
		"region":        regexp.MustCompile(`\bregions?\b`),
		"tract":         regexp.MustCompile(`\btracts?\b`),
		"nerve":         regexp.MustCompile(`\bnerves?\b`),
		"hemilineage":   regexp.MustCompile(`\b(?:hemi ?)?lineages?\b`),
		"proofread":     regexp.MustCompile(`\bproofread`),
		"group":         regexp.MustCompile(`\bgroups?\b`),
	}
)

// colorFieldsNamed returns the fields named within a few words of a color
// word ("colored by group and side", "each group a hue and subtypes as
// tones"). A two-field --color-by needs both fields named there, so the
// model cannot add a second field the user never asked for.
func colorFieldsNamed(q string) map[string]bool {
	toks := strings.Fields(reNonAlnum.ReplaceAllString(strings.ToLower(q), " "))
	out := map[string]bool{}
	for i, t := range toks {
		if !reColorWord.MatchString(t) {
			continue
		}
		window := strings.Join(toks[max(0, i-3):min(len(toks), i+7)], " ")
		for f, re := range colorFieldWords {
			if re.MatchString(window) {
				out[f] = true
			}
		}
	}
	return out
}

func colorByAllowed(named map[string]bool) []string {
	var out []string
	for _, v := range colorByValues() {
		a, b, pair := strings.Cut(v, ",")
		if !pair || named[a] && named[b] {
			out = append(out, v)
		}
	}
	return out
}

func gatherEvidence(v *Vocab, q string) evidence {
	e := evidence{values: map[string][]string{}, words: " " + normalize(q) + " "}
	e.ids = reRootID.FindAllString(q, -1)
	e.files = reJSON.FindAllString(q, -1)
	e.positions = rePos.FindAllString(q, -1)
	for _, m := range reQuoted.FindAllStringSubmatch(q, -1) {
		e.quoted = append(e.quoted, m[1]+m[2])
	}
	if len(e.quoted) == 0 {
		e.layers = unquotedLayers(q)
	}
	e.boolOK = map[string]bool{"open": reOpenWord.MatchString(reViewerState.ReplaceAllString(reOpenNeg.ReplaceAllString(q, ""), ""))}
	for name, re := range boolEvidence {
		e.boolOK[name] = re.MatchString(q)
	}
	// "clean" is gated like a bool flag but its evidence is negation-aware, and
	// --all additionally needs an un-negated word for deleting everything.
	e.boolOK["clean"] = reCleanWord.MatchString(reCleanNeg.ReplaceAllString(q, ""))
	e.cleanAll = e.boolOK["clean"] && reAllWord.MatchString(reAllNeg.ReplaceAllString(q, ""))
	e.column = reColumn.MatchString(q)
	e.colorFields = colorFieldsNamed(q)
	e.durations = reDuration.FindAllString(q, -1)
	e.ints = append([]string{"0"}, reSmallInt.FindAllString(q, -1)...)
	seen := map[string]bool{}
	for _, c := range v.retrieve(q) {
		key := c.Field + "=" + c.Value
		if !seen[key] {
			seen[key] = true
			e.values[c.Field] = append(e.values[c.Field], c.Value)
		}
	}
	// Binary fields are evidenced by naming them.
	if strings.Contains(e.words, "proofread") {
		e.values["proofread"] = []string{"true", "false"}
	}
	return e
}

func uniq(xs []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, x := range xs {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

var colorValues = []string{"colored", "blue", "red", "green", "turquoise", "orange", "purple", "yellow",
	"pink", "brown", "indigo", "teal", "lime"}
var colorByFields = []string{"super_class", "cell_class", "cell_type", "cell_subtype", "cell_instance", "column",
	"side", "region", "tract", "nerve", "hemilineage", "proofread", "pos_x", "pos_y", "pos_z", "root_id", "group"}
var labelFields = []string{"super_class", "cell_class", "cell_type", "cell_subtype", "cell_instance", "side",
	"region", "tract", "nerve", "hemilineage", "proofread"}

func colorByValues() []string {
	out := append([]string(nil), colorByFields...)
	continuous := map[string]bool{"pos_x": true, "pos_y": true, "pos_z": true, "root_id": true}
	for _, a := range colorByFields {
		if continuous[a] {
			continue
		}
		for _, b := range colorByFields {
			if a == b || continuous[b] || b == "group" {
				continue
			}
			out = append(out, a+","+b)
		}
	}
	return out
}

func labelListValues(standalone ...string) []string {
	out := append([]string(nil), standalone...)
	out = append(out, labelFields...)
	for _, a := range labelFields {
		for _, b := range labelFields {
			if a != b {
				out = append(out, a+","+b)
			}
		}
	}
	return out
}

// gatedValues returns the values the grammar offers for one non-bool flag,
// or nil to leave the flag out. Free-form strings are never offered.
func gatedValues(f FlagSpec, e evidence) []string {
	var vals []string
	switch {
	case f.Field != "":
		vals = e.values[f.Field]
	case f.Name == "state" || f.Name == "output":
		vals = e.files
	case f.Name == "pos":
		vals = e.positions
	case f.Name == "layer":
		vals = append(append([]string{}, e.quoted...), e.layers...)
	case f.Name == "labels-hook":
		vals = e.quoted
	case f.Name == "color-by":
		vals = colorByAllowed(e.colorFields)
	case f.Type == "duration":
		vals = e.durations
	case f.Type == "int":
		vals = e.ints
	case f.Name == "color":
		vals = colorValues
	case f.Name == "label-by":
		vals = labelListValues()
	case f.Name == "label-tags":
		vals = labelListValues("none")
	}
	return uniq(vals)
}

// argRule is the positional arguments a command may take under the evidence.
type argRule struct {
	vals     []string
	min, max int // max 0 means up to four
}

// gatedArgs returns the argument rule for a command, or false when the
// command is impossible for this request (it needs a root ID and there is
// none, or it needs an argument that no evidence provides).
func gatedArgs(c *CmdSpec, e evidence) (argRule, bool) {
	var rule argRule
	switch {
	case c.ArgsUse == "<field>":
		// list's argument names a classification field.
		rule = argRule{vals: labelFields, min: 1, max: 1}
	case strings.Contains(c.ArgsUse, "root_id...]"):
		rule = argRule{vals: uniq(e.ids)}
	case strings.HasPrefix(c.ArgsUse, "<root_id>"):
		if len(e.ids) == 0 && len(c.ArgOrFlag) == 0 {
			return argRule{}, false
		}
		rule = argRule{vals: uniq(e.ids), min: 1, max: 1}
	case strings.HasPrefix(c.ArgsUse, "[root_id]"):
		rule = argRule{vals: uniq(e.ids), max: 1}
	case c.ArgsUse != "":
		rule = argRule{vals: uniq(e.files), max: 1}
	}
	// Honor the command's Args validator for every command: one that rejects
	// zero arguments needs an argument, unless a flag can stand in for it
	// (root-info --pos).
	if c.ArgsMin > 0 {
		if len(rule.vals) == 0 {
			if len(c.ArgOrFlag) == 0 {
				return argRule{}, false
			}
		} else {
			rule.min = 1
		}
	}
	if len(rule.vals) == 0 {
		return argRule{}, true
	}
	return rule, true
}
