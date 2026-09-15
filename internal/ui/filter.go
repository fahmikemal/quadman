package ui

import "strings"

// fuzzyMatch reports whether pattern appears in text as a subsequence
// (case-insensitive), which is what the `/` filter uses.
func fuzzyMatch(text, pattern string) bool {
	text = strings.ToLower(text)
	pattern = strings.ToLower(pattern)
	pi := 0
	for i := 0; i < len(text) && pi < len(pattern); i++ {
		if text[i] == pattern[pi] {
			pi++
		}
	}
	return pi == len(pattern)
}
