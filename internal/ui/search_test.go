package ui

import (
	"testing"
)

func TestSearchRangesLiteral(t *testing.T) {
	content := "alpha beta\nBeta gamma\nbeta"
	ranges := searchRanges(content, "beta")
	if len(ranges) != 3 {
		t.Fatalf("case-insensitive literal should match 3x, got %v", ranges)
	}
	// Ranges must be ordered and point at "beta"/"Beta".
	for _, r := range ranges {
		got := content[r[0]:r[1]]
		if got != "beta" && got != "Beta" {
			t.Errorf("range %v = %q, want beta/Beta", r, got)
		}
	}
}

func TestSearchRangesRegex(t *testing.T) {
	content := "ERR-12 disk full\nok\nERR-99 retry"
	ranges := searchRanges(content, `ERR-\d+`)
	if len(ranges) != 2 {
		t.Fatalf("regex ERR-\\d+ should match 2x, got %v", ranges)
	}
	if got := content[ranges[0][0]:ranges[0][1]]; got != "ERR-12" {
		t.Errorf("first match = %q", got)
	}
}

func TestSearchRangesInvalidRegexFallsBack(t *testing.T) {
	content := "value [unclosed here"
	ranges := searchRanges(content, "[unclosed")
	if len(ranges) != 1 {
		t.Fatalf("invalid regex must fall back to literal substring, got %v", ranges)
	}
}

func TestSearchRangesEmpty(t *testing.T) {
	if r := searchRanges("anything", ""); r != nil {
		t.Errorf("empty query = %v", r)
	}
	if r := searchRanges("", "x"); r != nil {
		t.Errorf("empty content = %v", r)
	}
}

func TestSearchRangesZeroWidthSkipped(t *testing.T) {
	ranges := searchRanges("aaa", "a*") // matches empty strings too
	for _, r := range ranges {
		if r[1] <= r[0] {
			t.Errorf("zero-width range must be skipped: %v", r)
		}
	}
	if len(ranges) != 1 || ranges[0][0] != 0 || ranges[0][1] != 3 {
		t.Errorf("a* on aaa should yield one range [0 3], got %v", ranges)
	}
}

func TestSearchRangesCapped(t *testing.T) {
	content := ""
	for i := 0; i < maxSearchMatches+50; i++ {
		content += "x "
	}
	if n := len(searchRanges(content, "x")); n != maxSearchMatches {
		t.Errorf("matches should be capped at %d, got %d", maxSearchMatches, n)
	}
}

func TestLogSearchFlow(t *testing.T) {
	m := New()
	m.mode = modeLogs
	m.logLines = []string{"boot ok", "disk full", "all good", "disk again"}
	m.searchStr = "disk"
	m.applySearch()
	if m.searchMatches != 2 || m.matchPos != 1 {
		t.Fatalf("search = %d matches pos %d, want 2/1", m.searchMatches, m.matchPos)
	}
	m.nextMatch()
	if m.matchPos != 2 {
		t.Errorf("nextMatch pos = %d, want 2", m.matchPos)
	}
	m.nextMatch() // wraps
	if m.matchPos != 1 {
		t.Errorf("nextMatch wrap pos = %d, want 1", m.matchPos)
	}
	m.prevMatch() // wraps back
	if m.matchPos != 2 {
		t.Errorf("prevMatch wrap pos = %d, want 2", m.matchPos)
	}

	// New lines streaming in are folded in on the next navigation.
	m.logLines = append(m.logLines, "disk third")
	m.nextMatch()
	if m.searchMatches != 3 {
		t.Errorf("streamed line must join matches: %d", m.searchMatches)
	}

	m.clearSearch()
	if m.searching || m.searchStr != "" || m.searchMatches != 0 {
		t.Error("clearSearch must reset everything")
	}
}
