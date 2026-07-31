//go:build windows

package browser

import "strings"

// Windows processes receive one command line rather than an argv vector, so Go
// escapes every argument into that single string: a '"' is written as '\"'.
// rundll32 hands the raw tail of the command line to FileProtocolHandler
// without undoing the escaping, so a state URL arrives at the browser as
// #!{\"layers\":... and the viewer reports
// `Error parsing state: Expected property name or '}'`. Stage any URL holding a
// character Go escapes through the redirect file instead of the command line.
func openerArgumentIsSafe(rawURL string) bool {
	return !strings.ContainsAny(rawURL, "\"\\ \t")
}
