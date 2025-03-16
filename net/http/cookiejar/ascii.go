package cookiejar

import (
	"strings"
	"unicode"
)

// isPrint returns whether s is ASCII and printable according to
// https://tools.ietf.org/html/rfc20#section-4.2.
func isPrint(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < ' ' || s[i] > '~' {
			return false
		}
	}
	return true
}

// Is returns whether s is ASCII.
func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] > unicode.MaxASCII {
			return false
		}
	}
	return true
}

// toLower returns the lowercase version of s if s is ASCII and printable.
func toLower(s string) (lower string, ok bool) {
	if !isPrint(s) {
		return "", false
	}
	return strings.ToLower(s), true
}
