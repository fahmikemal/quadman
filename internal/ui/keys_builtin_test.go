package ui

import (
	"sort"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/fahmikemal/quadman/internal/config"
)

// TestBuiltinKeysCoverDispatch derives the list-mode keys from the real
// dispatch (keyChain) and checks builtinKeys names every one of them, so
// the "custom command shadowed" warning can never go stale when a new key
// is added.
func TestBuiltinKeysCoverDispatch(t *testing.T) {
	var missing []string
	for c := '!'; c <= '~'; c++ {
		key := string(c)
		m := withUnits(New(), "webapp", "db")
		m.table.SetCursor(0)
		msg := tea.KeyPressMsg{Code: c, Text: key}
		consumed := false
		for _, h := range keyChain {
			if _, _, ok := h.fn(m, msg); ok {
				consumed = true
				break
			}
		}
		if consumed && !builtinKeys[key] {
			missing = append(missing, key)
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Errorf("list-mode keys handled by quadman but missing from builtinKeys (a custom command bound there would be silently shadowed): %s", strings.Join(missing, " "))
	}
}

// TestCustomNavOverrides: a custom command on a table navigation key fires
// but takes the navigation away, which quadman reports.
func TestCustomNavOverrides(t *testing.T) {
	for _, k := range []string{"G", "j", "k", "b", "f"} {
		if !tableKeys[k] {
			t.Errorf("%q must be a table navigation key", k)
		}
	}
	got := customNavOverrides([]config.CustomCommand{{Name: "image", Key: "G"}, {Name: "sort", Key: "S"}, {Name: "ok", Key: "C"}})
	if strings.Join(got, ",") != "image (G)" {
		t.Errorf("nav overrides = %v, want [image (G)]", got)
	}
}
