package seatable

import "strings"

// FieldValue returns the string value of a classification field for a neuron
// row. Field names match the CRANTb_meta column names used in filters. For
// "region" it prefers the matched region (set when a query filtered on regions)
// and falls back to the row's resolved region string. Unknown fields return "".
func FieldValue(row NeuronRow, field string) string {
	switch field {
	case "super_class":
		return row.SuperClass
	case "cell_class":
		return row.CellClass
	case "cell_type":
		return row.CellType
	case "cell_subtype":
		return row.CellSubtype
	case "cell_instance":
		return row.CellInstance
	case "side":
		return row.Side
	case "region":
		if len(row.MatchedRegions) > 0 {
			return row.MatchedRegions[0]
		}
		return row.Region
	case "tract":
		return row.Tract
	case "nerve":
		return row.Nerve
	case "hemilineage":
		return row.Hemilineage
	case "proofread":
		return row.Proofread
	default:
		return ""
	}
}

// FieldValues returns every value a field holds for a row, for callers that
// treat each annotation separately rather than displaying one string. Only
// "region" is multi-valued (SeaTable stores it as a multi-select), and it
// returns every region the neuron is annotated to, not just the ones a query
// filtered on. Every other field yields at most one value, and unknown or empty
// fields yield none.
func FieldValues(row NeuronRow, field string) []string {
	if field != "region" {
		if value := FieldValue(row, field); value != "" {
			return []string{value}
		}
		return nil
	}

	if len(row.Regions) > 0 {
		return row.Regions
	}
	if len(row.MatchedRegions) > 0 {
		return row.MatchedRegions
	}
	// A row carrying only the display join -- built before Regions existed, or
	// by hand -- splits back into the values that join was made from.
	return splitDisplayList(row.Region)
}

// splitDisplayList inverts the ", " join used for multi-valued display strings.
func splitDisplayList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}

	parts := strings.Split(value, ",")
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			values = append(values, part)
		}
	}
	return values
}
