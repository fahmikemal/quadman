package ui

import (
	"fmt"

	"charm.land/bubbles/v2/tree"

	"github.com/kemal-labs/quadman/internal/quadlet"
)

// buildTree constructs the dependency tree: every unit as a node, its
// referenced quadlet units as children (marked when missing).
func buildTree(units []quadlet.Unit, statuses map[string]bool) *tree.Node {
	byName := map[string]quadlet.Unit{}
	for _, u := range units {
		byName[u.Name] = u
	}
	root := tree.Root("quadlets")
	for _, u := range units {
		node := tree.Root(unitLabel(u))
		if f, err := quadlet.Parse(u.Path); err == nil {
			for _, dep := range quadlet.Deps(u, f) {
				if du, ok := byName[dep]; ok {
					node.Child(unitLabel(du))
				} else {
					node.Child(dep + " (missing)")
				}
			}
		}
		root.Child(node)
	}
	return root
}

// unitLabel renders one tree line: name, kind, and the generated unit.
func unitLabel(u quadlet.Unit) string {
	return fmt.Sprintf("%s · %s · %s", u.Name, u.Kind, u.UnitName)
}
