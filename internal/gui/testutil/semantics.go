package testutil

import (
	"strings"
	"testing"

	"github.com/rm4n0s/glutter/gluttertest"
	"github.com/rm4n0s/glutter/ui"
)

// SemanticNodes returns the nodes of the semantics tree as the last updates left it,
// root first, in traversal order. Call ts.EnsureSemantics before the widget is shown
// and pump after each change.
func SemanticNodes(ts *gluttertest.Tester) []ui.SemanticsNodeUpdate {
	latest := map[int]ui.SemanticsNodeUpdate{}
	for _, u := range ts.SemanticsUpdates() {
		for _, n := range u.Nodes {
			latest[n.ID] = n
		}
	}
	var out []ui.SemanticsNodeUpdate
	seen := map[int]bool{}
	var walk func(id int)
	walk = func(id int) {
		n, ok := latest[id]
		if !ok || seen[id] {
			return
		}
		seen[id] = true
		out = append(out, n)
		for _, c := range n.ChildrenInTraversalOrder {
			walk(c)
		}
	}
	walk(0)
	return out
}

// CheckSemantics fails the test when a node that the user can act on has no name for a
// screen reader, or when a label, value or hint holds one of the secrets. It returns
// the nodes.
func CheckSemantics(t *testing.T, ts *gluttertest.Tester, secrets ...string) []ui.SemanticsNodeUpdate {
	t.Helper()
	nodes := SemanticNodes(ts)
	if len(nodes) == 0 {
		t.Fatal("the semantics tree is empty; call EnsureSemantics before showing the widget")
	}
	for _, n := range nodes {
		if n.Flags.IsHidden {
			continue
		}
		acts := n.Actions&ui.SemanticsActionTap != 0 || n.Flags.IsButton || n.Flags.IsTextField
		if acts && strings.TrimSpace(n.Label) == "" && strings.TrimSpace(n.Value) == "" {
			t.Errorf("semantics node %d (%v, actions %v) can be used but has no label", n.ID, n.Flags, n.Actions)
		}
		for _, s := range secrets {
			if s == "" {
				continue
			}
			for _, text := range []string{n.Label, n.Value, n.Hint, n.Tooltip} {
				if strings.Contains(text, s) {
					t.Errorf("semantics node %d exposes a secret in %q", n.ID, text)
				}
			}
		}
	}
	return nodes
}

// FindNode returns the node with the label, or false.
func FindNode(nodes []ui.SemanticsNodeUpdate, label string) (ui.SemanticsNodeUpdate, bool) {
	for _, n := range nodes {
		if n.Label == label {
			return n, true
		}
	}
	return ui.SemanticsNodeUpdate{}, false
}

// FindTextField returns the text field node with the label, or false.
func FindTextField(nodes []ui.SemanticsNodeUpdate, label string) (ui.SemanticsNodeUpdate, bool) {
	for _, n := range nodes {
		if n.Label == label && n.Flags.IsTextField {
			return n, true
		}
	}
	return ui.SemanticsNodeUpdate{}, false
}
