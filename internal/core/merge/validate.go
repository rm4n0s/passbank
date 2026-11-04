package merge

import (
	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/passbank/internal/core/diff"
	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/core/plan"
)

// Conflict is an item the plan keeps in a group that the plan drops: dropping a
// group drops everything in it, so the plan cannot be applied (SPEC §8).
type Conflict struct {
	// Group is the index in the plan of the change that drops the group.
	Group int
	// Item is the index of the change of the item that is kept in it.
	Item int
}

// Validate checks that a plan can be applied: it has one choice for every change,
// every choice is one its kind allows (PlanInvalidChoice), and no group that is
// dropped still holds an item that is kept (PlanGroupNotEmpty, naming the group
// and one such item). Conflicts lists all the items of the second kind.
func Validate(p *plan.Plan) error {
	if len(p.Choices) != len(p.Changes) {
		return errors.New("PlanIncomplete", "the plan has a different number of choices and changes",
			"choices", len(p.Choices), "changes", len(p.Changes))
	}
	for i, c := range p.Changes {
		if !plan.IsAllowed(c.Kind, p.Choices[i]) {
			return errors.New("PlanInvalidChoice", "the choice is not allowed for this kind of change",
				"choice", string(p.Choices[i]), "kind", c.Kind.String(), "item", itemName(c))
		}
	}
	if conflicts := Conflicts(p); len(conflicts) > 0 {
		g, it := p.Changes[conflicts[0].Group], p.Changes[conflicts[0].Item]
		return errors.New("PlanGroupNotEmpty", "a group that is dropped still holds an item that is kept",
			"group", itemName(g), "group path", groupPath(g), "item", itemName(it), "item title", it.Title, "conflicts", len(conflicts))
	}
	return nil
}

// itemName names the item of a change by its UUID (or "settings").
func itemName(c diff.Change) string {
	if c.Item == diff.ItemSettings {
		return "settings"
	}
	return c.UUID.String()
}

// groupPath is the path of a group change with the group's own name.
func groupPath(c diff.Change) string {
	if c.Path == "" {
		return c.Title
	}
	return c.Path + " / " + c.Title
}

// Conflicts lists every item that the plan keeps directly in a group that the
// plan drops, in the order of the changes (an item deeper down is named through
// the nearest dropped group, which is the one that holds it). The plan must have
// a choice for every change.
func Conflicts(p *plan.Plan) []Conflict {
	// The groups that are dropped, by the group object of the side that has them.
	dropped := map[*model.Group]int{}
	for i, c := range p.Changes {
		if c.Item != diff.ItemGroup || p.Choices[i] != plan.Drop {
			continue
		}
		switch c.Kind {
		case diff.OnlyA:
			dropped[c.Group[diff.SideA]] = i
		case diff.OnlyB:
			dropped[c.Group[diff.SideB]] = i
		}
	}
	if len(dropped) == 0 {
		return nil
	}
	var out []Conflict
	for i, c := range p.Changes {
		for _, side := range positions(c, p.Choices[i]) {
			parent := c.Parent[side]
			if parent == nil {
				continue
			}
			if g, ok := dropped[parent]; ok {
				out = append(out, Conflict{Group: g, Item: i})
				break
			}
		}
	}
	return out
}

// positions says on which sides' tree the result will place the item of a change
// under its choice: where the item will sit in the result follows the copy the
// plan took. A dropped item sits nowhere.
func positions(c diff.Change, choice plan.Choice) []diff.Side {
	a, b := []diff.Side{diff.SideA}, []diff.Side{diff.SideB}
	switch c.Kind {
	case diff.OnlyA:
		if choice == plan.Keep {
			return a
		}
	case diff.OnlyB:
		if choice == plan.Keep {
			return b
		}
	case diff.DeletedInA: // the entry is B's
		if choice == plan.Keep {
			return b
		}
	case diff.DeletedInB: // the entry is A's
		if choice == plan.Keep {
			return a
		}
	case diff.Differs:
		switch choice {
		case plan.TakeA:
			return a
		case plan.TakeB:
			return b
		case plan.Both:
			return []diff.Side{diff.SideA, diff.SideB}
		}
	case diff.GroupDiffers:
		switch choice {
		case plan.TakeA:
			return a
		case plan.TakeB:
			return b
		}
	}
	return nil
}
