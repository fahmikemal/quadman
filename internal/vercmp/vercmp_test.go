package vercmp

import "testing"

func TestAtLeast(t *testing.T) {
	cases := []struct {
		have, want string
		ok         bool
	}{
		{"6.1.1", "6.1", true},
		{"6.0.2", "6.1", false},
		{"6.1.0", "6.1", true},
		{"5.8.6", "6.0", false},
		{"6.1.1", "5.3", true},
		{"6.1", "6.1.1", false}, // 6.1 == 6.1.0 < 6.1.1
		{"", "6.1", false},
		{"6.1.1", "", true},
		{"6.1.x", "6.1.0", true},  // non-numeric compares as 0
		{"6.1.1", "6.1.x", true},  // x == 0, so 6.1.1 > 6.1.0
		{"6.1.x", "6.1.1", false}, // x == 0, so 6.1.0 < 6.1.1
	}
	for _, tc := range cases {
		if got := AtLeast(tc.have, tc.want); got != tc.ok {
			t.Errorf("AtLeast(%q, %q) = %v, want %v", tc.have, tc.want, got, tc.ok)
		}
	}
}
