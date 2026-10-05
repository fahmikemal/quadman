package ui

import "strings"

// fuzzyMatch reports whether pattern appears in text as a subsequence
// (case-insensitive), which is what the `/` filter uses. Matching runs over
// runes so multi-byte patterns behave like their single-byte equivalents.
func fuzzyMatch(text, pattern string) bool {
	t := []rune(strings.ToLower(text))
	p := []rune(strings.ToLower(pattern))
	pi := 0
	for i := 0; i < len(t) && pi < len(p); i++ {
		if t[i] == p[pi] {
			pi++
		}
	}
	return pi == len(p)
}
