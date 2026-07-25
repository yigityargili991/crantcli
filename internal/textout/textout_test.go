package textout

import (
	"strings"
	"testing"
)

func TestSanitize(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain ascii", "hello world", "hello world"},
		{"empty", "", ""},
		{"unicode preserved", "Δ7_γείτων 日本語", "Δ7_γείτων 日本語"},
		{"osc52 clipboard write", "evil\x1b]52;c;SGFjZ2Vk\x07", "evil�]52;c;SGFjZ2Vk�"},
		{"osc8 hyperlink", "\x1b]8;;https://evil.example\x07link\x1b]8;;\x07", "�]8;;https://evil.example�link�]8;;�"},
		{"csi color", "\x1b[31mred\x1b[0m", "�[31mred�[0m"},
		{"newline forged log", "ok\nfake: prompt", "ok�fake: prompt"},
		{"carriage return overwrite", "real\rfake", "real�fake"},
		{"tab", "a\tb", "a�b"},
		{"bell", "a\ab", "a�b"},
		{"delete", "a\x7fb", "a�b"},
		{"c1 control", "a\u0085b", "a�b"},
		{"invalid utf8 byte", "a\x85b", "a�b"},
		{"bidi override", "abc\u202edef", "abc�def"},
		{"bidi isolate", "abc\u2067def", "abc�def"},
		{"zero width bom", "a\ufeffb", "a�b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Sanitize(tt.input); got != tt.want {
				t.Errorf("Sanitize(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestSanitizeNeverEmitsEscapes(t *testing.T) {
	in := "\x1b\x9b\x00\x1f\x7f\x80\x9f\r\n\t\a\u202a\u202e\u2066\u2069"
	out := Sanitize(in)
	if strings.ContainsRune(out, '\x1b') {
		t.Fatalf("output contains ESC: %q", out)
	}
	for _, r := range out {
		if r < 0x20 || (r >= 0x7f && r <= 0x9f) {
			t.Fatalf("output contains control rune %U: %q", r, out)
		}
	}
}
