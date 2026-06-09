package httperror

import (
	"fmt"
	"io"
	"regexp"
	"strings"
)

const PreviewLimit = 4096

var (
	bearerTokenRE = regexp.MustCompile(`(?i)Bearer\s+[A-Za-z0-9._~+/=-]+`)
	tokenFieldRE  = regexp.MustCompile(`(?i)("?(?:authorization|token|access[_-]?token|api[_-]?token|refresh[_-]?token|id[_-]?token|auth[_-]?token|bearer[_-]?token|cave[_-]?token)"?\s*[:=]\s*)("[^"]*"|[^\s,}&]+)`)
)

func Format(prefix string, statusCode int, body io.Reader) error {
	preview, err := Preview(body)
	if err != nil {
		return fmt.Errorf("%s failed (HTTP %d): reading error response: %w", prefix, statusCode, err)
	}
	if preview == "" {
		return fmt.Errorf("%s failed (HTTP %d)", prefix, statusCode)
	}
	return fmt.Errorf("%s failed (HTTP %d): %s", prefix, statusCode, preview)
}

func Preview(body io.Reader) (string, error) {
	if body == nil {
		return "", nil
	}
	data, err := io.ReadAll(io.LimitReader(body, PreviewLimit+1))
	if err != nil {
		return "", err
	}
	return PreviewString(string(data)), nil
}

func PreviewString(value string) string {
	truncated := len(value) > PreviewLimit
	if truncated {
		value = value[:PreviewLimit]
	}
	preview := strings.TrimSpace(value)
	if preview == "" {
		return ""
	}
	preview = Redact(preview)
	if truncated {
		preview += "... (truncated)"
	}
	return preview
}

func Redact(value string) string {
	value = bearerTokenRE.ReplaceAllString(value, "Bearer [REDACTED]")
	return tokenFieldRE.ReplaceAllString(value, `${1}"[REDACTED]"`)
}
