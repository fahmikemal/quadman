package ui

import (
	"fmt"

	"charm.land/bubbles/v2/tree"

	"github.com/fahmikemal/quadman/internal/quadlet"
)

// buildTree constructs the dependency tree: every unit as a node, its
// referenced quadlet units as children (marked when missing). Files are
// parsed from local disk.
func buildTree(units []quadlet.Unit, statuses map[string]bool) *tree.Node {
	return buildTreeWith(units, quadlet.Parse)
}

// buildTreeWith is buildTree with an injectable file parser, so remote mode
// can parse content catted over SSH instead of local disk.
func buildTreeWith(units []quadlet.Unit, parse func(path string) (*quadlet.File, error)) *tree.Node {
	byName := map[string]quadlet.Unit{}
	for _, u := range units {
		byName[u.Name] = u
	}
	root := tree.Root("quadlets")
	for _, u := range units {
		node := tree.Root(unitLabel(u))
		if f, err := parse(u.Path); err == nil {
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

// buildTreeModel builds the tree for the model's units, parsing files
// locally or over SSH depending on the mode.
func (m Model) buildTreeModel() *tree.Node {
	if !m.ssh.IsRemote() {
		return buildTree(m.units, nil)
	}
	return buildTreeWith(m.units, func(path string) (*quadlet.File, error) {
		for _, u := range m.units {
			if u.Path == path {
				return m.parseUnitFile(u)
			}
		}
		return nil, errNoSuchUnit
	})
}

// unitLabel renders one tree line: name, kind, and the generated unit.
func unitLabel(u quadlet.Unit) string {
	return fmt.Sprintf("%s · %s · %s", u.Name, u.Kind, u.UnitName)
}
