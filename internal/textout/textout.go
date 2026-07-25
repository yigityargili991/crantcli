// Package textout neutralizes untrusted strings (values from state files,
// clipboard contents, server responses, and database rows) before they are
// written to a terminal. Control characters in such strings can inject ANSI
// escape sequences (including OSC 52 clipboard writes and OSC 8 hyperlink
// spoofing), forge output with carriage returns or newlines, or reorder
// displayed text with bidirectional controls.
package textout

import "strings"

const replacement = '�'

// Sanitize returns s with every control or bidirectional-override rune
// replaced by U+FFFD. Printable text, including ordinary Unicode, is
// preserved; newlines and tabs inside the value are replaced as well (the
// caller's own formatting supplies any legitimate whitespace). Invalid UTF-8
// is normalized to U+FFFD so raw C1 bytes cannot slip through.
func Sanitize(s string) string {
	s = strings.ToValidUTF8(s, string(replacement))
	if !strings.ContainsFunc(s, needsSanitize) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if needsSanitize(r) {
			b.WriteRune(replacement)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func needsSanitize(r rune) bool {
	switch {
	case r < 0x20, r == 0x7f:
		// C0 controls (incl. ESC, CR, LF, TAB) and DEL.
		return true
	case r >= 0x80 && r <= 0x9f:
		// C1 controls.
		return true
	case r >= 0x202a && r <= 0x202e:
		// Bidi embeddings and overrides.
		return true
	case r >= 0x2066 && r <= 0x2069:
		// Bidi isolates.
		return true
	case r == 0x200e || r == 0x200f || r == 0x061c:
		// LRM/RLM and the Arabic letter mark.
		return true
	case r == 0xfeff:
		// BOM / zero-width no-break space.
		return true
	}
	return false
}
