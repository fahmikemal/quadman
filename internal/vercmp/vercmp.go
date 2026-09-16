// Package vercmp compares dotted numeric versions such as "6.1.1".
// It lives in its own package so both the podman wrapper and the UI can
// share one implementation instead of maintaining parallel copies.
package vercmp

import "strconv"

// AtLeast reports whether have >= want using dotted numeric comparison,
// e.g. "6.1" <= "6.1.1". Missing components compare as 0, and non-numeric
// components compare as 0.
func AtLeast(have, want string) bool {
	hp, wp := split(have), split(want)
	for i := range wp {
		hn, wn := 0, 0
		if i < len(hp) {
			hn = atoi(hp[i])
		}
		wn = atoi(wp[i])
		if hn != wn {
			return hn > wn
		}
	}
	return true
}

func split(v string) []string {
	if v == "" {
		return nil
	}
	var out []string
	start := 0
	for i := 0; i <= len(v); i++ {
		if i == len(v) || v[i] == '.' {
			out = append(out, v[start:i])
			start = i + 1
		}
	}
	return out
}

// atoi parses a version component; non-numeric parts compare as 0.
func atoi(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}
