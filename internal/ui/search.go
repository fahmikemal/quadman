package ui

import (
	"regexp"
	"strings"
)

// maxSearchMatches bounds how many ranges we hand to the viewport so a
// pathological query (e.g. "e") cannot stall rendering.
const maxSearchMatches = 500

// searchRanges returns the [start,end] character ranges of every match of
// query inside content, in order. The query is treated as a case-insensitive
// regular expression when it compiles, and as a literal substring otherwise.
// Empty queries and zero-width matches yield no ranges.
func searchRanges(content, query string) [][]int {
	if query == "" || content == "" {
		return nil
	}
	var ranges [][]int
	if re, err := regexp.Compile("(?i)" + query); err == nil {
		for _, loc := range re.FindAllStringIndex(content, -1) {
			if loc[1] > loc[0] {
				ranges = append(ranges, []int{loc[0], loc[1]})
			}
		}
	} else {
		lower := strings.ToLower(content)
		q := strings.ToLower(query)
		for off := 0; ; {
			i := strings.Index(lower[off:], q)
			if i < 0 {
				break
			}
			ranges = append(ranges, []int{off + i, off + i + len(q)})
			off += i + 1
		}
	}
	if len(ranges) > maxSearchMatches {
		ranges = ranges[:maxSearchMatches]
	}
	return ranges
}

// applySearch recomputes highlights for the current search string and points
// the viewport at the first match.
func (m *Model) applySearch() {
	ranges := searchRanges(strings.Join(m.logLines, "\n"), m.searchStr)
	m.searchMatches = len(ranges)
	if len(ranges) == 0 {
		m.matchPos = 0
		m.viewport.ClearHighlights()
		return
	}
	m.matchPos = 1
	m.viewport.SetHighlights(ranges)
}

// nextMatch moves to the following match, wrapping around. New log lines
// that arrived since the search are folded in first; the position is kept.
func (m *Model) nextMatch() {
	m.refreshSearchKeepPos()
	if m.searchMatches == 0 {
		return
	}
	m.viewport.HighlightNext()
	m.matchPos = m.matchPos%m.searchMatches + 1
}

// prevMatch moves to the previous match, wrapping around.
func (m *Model) prevMatch() {
	m.refreshSearchKeepPos()
	if m.searchMatches == 0 {
		return
	}
	m.viewport.HighlightPrevious()
	m.matchPos--
	if m.matchPos < 1 {
		m.matchPos = m.searchMatches
	}
}

// refreshSearchKeepPos recomputes matches (picking up lines that streamed in
// after the search started) without resetting the current position.
func (m *Model) refreshSearchKeepPos() {
	if m.searchStr == "" {
		return
	}
	pos := m.matchPos
	ranges := searchRanges(strings.Join(m.logLines, "\n"), m.searchStr)
	m.searchMatches = len(ranges)
	if len(ranges) == 0 {
		m.matchPos = 0
		m.viewport.ClearHighlights()
		return
	}
	if pos < 1 {
		pos = 1
	}
	if pos > len(ranges) {
		pos = len(ranges)
	}
	m.matchPos = pos
	m.viewport.SetHighlights(ranges)
}

// clearSearch resets the log search state and removes highlights.
func (m *Model) clearSearch() {
	m.searching = false
	m.searchStr = ""
	m.searchMatches = 0
	m.matchPos = 0
	m.searchIn.SetValue("")
	m.searchIn.Blur()
	m.viewport.ClearHighlights()
}
