// Package shellwords splits a command line into argv words, honoring
// single quotes, double quotes, and backslash escapes — without invoking a
// shell. It is used to turn user-typed input (podlet generate, custom
// commands) into exec argv safely.
package shellwords

import (
	"fmt"
	"strings"
)

// Split parses s into words. Double quotes allow backslash escapes;
// single quotes preserve everything literally; a lone backslash escapes
// the next character. Unclosed quotes and a trailing backslash are errors.
func Split(s string) ([]string, error) {
	var words []string
	var cur strings.Builder
	inWord := false
	inSingle := false
	inDouble := false
	escaped := false
	flush := func() {
		if inWord {
			words = append(words, cur.String())
			cur.Reset()
			inWord = false
		}
	}
	for _, r := range s {
		switch {
		case escaped:
			cur.WriteRune(r)
			escaped = false
		case inSingle:
			if r == '\'' {
				inSingle = false
			} else {
				cur.WriteRune(r)
			}
		case inDouble:
			switch r {
			case '"':
				inDouble = false
			case '\\':
				escaped = true
			default:
				cur.WriteRune(r)
			}
		case r == '\'':
			inSingle = true
			inWord = true
		case r == '"':
			inDouble = true
			inWord = true
		case r == '\\':
			escaped = true
			inWord = true
		case r == ' ' || r == '\t' || r == '\n':
			flush()
		default:
			cur.WriteRune(r)
			inWord = true
		}
	}
	if escaped {
		return nil, fmt.Errorf("trailing backslash")
	}
	if inSingle || inDouble {
		return nil, fmt.Errorf("unclosed quote")
	}
	flush()
	return words, nil
}
