package seatable

import (
	"strings"
	"testing"
)

func TestEscapeSQL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain", "ER", "ER"},
		{"single quote doubled", "o'brien", "o''brien"},
		{"backslash escaped", `a\b`, `a\\b`},
		{"quote breakout attempt", `'; DROP TABLE x--`, `''; DROP TABLE x--`},
		{"backslash-quote pair", `a\'b`, `a\\''b`},
		{"only quote", `'`, `''`},
		{"only backslash", `\`, `\\`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := escapeSQL(tt.input); got != tt.want {
				t.Errorf("escapeSQL(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// Every quote in the escaped output must be part of a doubled pair, so the
// value cannot terminate the enclosing SQL string literal.
func TestBuildWhereQuotesAlwaysPaired(t *testing.T) {
	f := &Filters{CellType: `x' OR '1'='1`}
	where := buildWhere(f)
	if !strings.Contains(where, `x'' OR ''1''=''1'`) {
		t.Fatalf("WHERE clause not escaped: %q", where)
	}
}
