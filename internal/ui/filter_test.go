package ui

import "testing"

func TestFuzzyMatch(t *testing.T) {
	for _, tc := range []struct {
		text, pattern string
		want          bool
	}{
		{"webapp", "web", true},
		{"webapp", "wap", true},
		{"webapp", "wax", false},
		{"WebApp", "webapp", true},
		{"anything", "", true},
		{"", "x", false},
		// Multi-byte text and patterns match per rune, not per byte.
		{"wébapp-日本語", "é本語", true},
		{"wébapp-日本語", "é語x", false},
		{"日本語", "本語", true},
	} {
		if got := fuzzyMatch(tc.text, tc.pattern); got != tc.want {
			t.Errorf("fuzzyMatch(%q, %q) = %v, want %v", tc.text, tc.pattern, got, tc.want)
		}
	}
}
