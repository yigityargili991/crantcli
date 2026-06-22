// Package segprops builds neuroglancer_segment_properties "info" files that map
// root IDs to display labels (and filterable tags) so cell types show next to
// segment IDs in the Neuroglancer Seg. panel.
package segprops

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"crantcli/internal/seatable"
)

const typeName = "neuroglancer_segment_properties"

// Options controls how segment properties are built.
type Options struct {
	// LabelField is the row field used for the visible label (default cell_type).
	LabelField string
	// LabelFallbacks are fields tried in order when LabelField is empty, so
	// neurons without a cell_type still get a meaningful label.
	LabelFallbacks []string
	// TagFields are the fields exposed as filterable tag chips. Empty disables tags.
	TagFields []string
}

// DefaultOptions returns the standard label/tag configuration: a cell_type label
// (falling back to cell_class) with cell_class, side, and super_class as
// filterable tags.
func DefaultOptions() Options {
	return Options{
		LabelField:     "cell_type",
		LabelFallbacks: []string{"cell_class"},
		TagFields:      []string{"cell_class", "side", "super_class"},
	}
}

// labelFor returns the label for a row: the LabelField value, or the first
// non-empty fallback field, or "" when nothing is set.
func labelFor(row seatable.NeuronRow, opts Options) string {
	if v := seatable.FieldValue(row, opts.LabelField); v != "" {
		return v
	}
	for _, f := range opts.LabelFallbacks {
		if v := seatable.FieldValue(row, f); v != "" {
			return v
		}
	}
	return ""
}

// fieldTagPrefix maps a field to a short prefix used to disambiguate tag values
// that originate from different fields (e.g. "class_central_brain", "side_left").
// Neuroglancer allows at most one tags property, so all tag fields share one
// vocabulary and the prefix keeps values from colliding.
var fieldTagPrefix = map[string]string{
	"super_class":   "super",
	"cell_class":    "class",
	"cell_type":     "type",
	"cell_subtype":  "subtype",
	"cell_instance": "instance",
	"side":          "side",
	"region":        "region",
	"tract":         "tract",
	"nerve":         "nerve",
	"hemilineage":   "hemilineage",
	"proofread":     "proofread",
}

// BuildSegmentProperties builds a neuroglancer_segment_properties info file from
// the given rows. Root IDs are deduplicated (first occurrence wins, empty IDs
// skipped) and ordered deterministically so regenerated output is byte-stable.
func BuildSegmentProperties(rows []seatable.NeuronRow, opts Options) ([]byte, error) {
	if opts.LabelField == "" {
		opts.LabelField = "cell_type"
	}

	seen := make(map[string]bool, len(rows))
	kept := make([]seatable.NeuronRow, 0, len(rows))
	for _, r := range rows {
		if r.RootID == "" || seen[r.RootID] {
			continue
		}
		seen[r.RootID] = true
		kept = append(kept, r)
	}

	// Deterministic order: shorter IDs first, then lexical. Root IDs are
	// fixed-width decimals, so this matches numeric order while staying safe if
	// lengths ever differ.
	sort.Slice(kept, func(i, j int) bool {
		a, b := kept[i].RootID, kept[j].RootID
		if len(a) != len(b) {
			return len(a) < len(b)
		}
		return a < b
	})

	ids := make([]string, len(kept))
	labels := make([]string, len(kept))
	for i, r := range kept {
		ids[i] = r.RootID
		labels[i] = labelFor(r, opts)
	}

	properties := make([]json.RawMessage, 0, 2)

	labelProp, err := json.Marshal(map[string]interface{}{
		"id":     "label",
		"type":   "label",
		"values": labels,
	})
	if err != nil {
		return nil, fmt.Errorf("marshaling label property: %w", err)
	}
	properties = append(properties, labelProp)

	if len(opts.TagFields) > 0 {
		tagProp, ok, err := buildTagsProperty(kept, opts.TagFields)
		if err != nil {
			return nil, err
		}
		if ok {
			properties = append(properties, tagProp)
		}
	}

	info := map[string]interface{}{
		"@type": typeName,
		"inline": map[string]interface{}{
			"ids":        ids,
			"properties": properties,
		},
	}
	return json.MarshalIndent(info, "", "  ")
}

// buildTagsProperty collects a sorted, sanitized tag vocabulary across the given
// fields and assigns each row its ascending tag indices. Returns ok=false when
// no row produced any tag.
func buildTagsProperty(rows []seatable.NeuronRow, fields []string) (json.RawMessage, bool, error) {
	tagSet := make(map[string]bool)
	perRow := make([][]string, len(rows))

	for i, r := range rows {
		var rowTags []string
		for _, field := range fields {
			tag := sanitizeTag(fieldTagPrefix[field], seatable.FieldValue(r, field))
			if tag == "" {
				continue
			}
			rowTags = append(rowTags, tag)
			tagSet[tag] = true
		}
		perRow[i] = rowTags
	}

	if len(tagSet) == 0 {
		return nil, false, nil
	}

	tagList := make([]string, 0, len(tagSet))
	for tag := range tagSet {
		tagList = append(tagList, tag)
	}
	sort.Strings(tagList)

	tagIndex := make(map[string]int, len(tagList))
	for idx, tag := range tagList {
		tagIndex[tag] = idx
	}

	values := make([][]int, len(rows))
	for i, rowTags := range perRow {
		idxSet := make(map[int]bool, len(rowTags))
		for _, tag := range rowTags {
			idxSet[tagIndex[tag]] = true
		}
		idxs := make([]int, 0, len(idxSet))
		for idx := range idxSet {
			idxs = append(idxs, idx)
		}
		sort.Ints(idxs)
		values[i] = idxs
	}

	tagProp, err := json.Marshal(map[string]interface{}{
		"id":     "tags",
		"type":   "tags",
		"tags":   tagList,
		"values": values,
	})
	if err != nil {
		return nil, false, fmt.Errorf("marshaling tags property: %w", err)
	}
	return tagProp, true, nil
}

// sanitizeTag normalizes a tag value to neuroglancer's constraints (lowercase,
// no spaces, no '#') and prefixes it with the field tag. '/' becomes '_'.
// Returns "" when the underlying value is empty.
func sanitizeTag(prefix, value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	s := strings.ToLower(value)
	s = strings.ReplaceAll(s, "#", "")
	s = strings.ReplaceAll(s, "/", "_")
	s = strings.ReplaceAll(s, " ", "_")
	if prefix != "" {
		s = prefix + "_" + s
	}
	return s
}
