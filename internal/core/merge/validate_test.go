package merge

import (
	"testing"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/passbank/internal/core/diff"
	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/core/plan"
)

func TestValidateAcceptsTheRules(t *testing.T) {
	for seed := uint64(1); seed <= 40; seed++ {
		a, b := related(seed)
		for _, rule := range plan.Rules {
			p := planFor(t, a, b, nil)
			plan.ApplyRule(p, rule, nil)
			if err := Validate(p); err != nil {
				t.Fatalf("seed %d, rule %s: %v", seed, rule, err)
			}
		}
	}
}

func routeOf(t *testing.T, err error) *errors.Error {
	t.Helper()
	appErr, ok := errors.FromError(err)
	if !ok {
		t.Fatalf("expected *errors.Error, got %T (%v)", err, err)
	}
	return appErr
}

func TestValidateErrors(t *testing.T) {
	a := database(grp(1, "Root", grp(2, "Mail", ent(11, "X", "a", 1))), 0)
	b := database(grp(1, "Root", grp(2, "Mail", ent(11, "X", "b", 2))), 0)

	t.Run("a choice the kind does not allow", func(t *testing.T) {
		p := planFor(t, a, b, nil)
		p.Choices[0] = plan.Keep // differs
		appErr := routeOf(t, Validate(p))
		if !appErr.HasRoute("Validate.PlanInvalidChoice") || appErr.Metadata["item"] != u(11).String() {
			t.Errorf("err = %v", appErr)
		}
		_, err := Apply(a, b, p)
		if !routeOf(t, err).HasRoute("Apply->Validate.PlanInvalidChoice") {
			t.Errorf("Apply: %v", err)
		}
	})
	t.Run("no choice for some change", func(t *testing.T) {
		p := planFor(t, a, b, nil)
		p.Choices = nil
		if !routeOf(t, Validate(p)).HasRoute("Validate.PlanIncomplete") {
			t.Error("not PlanIncomplete")
		}
	})
	t.Run("an empty choice", func(t *testing.T) {
		p := planFor(t, a, b, nil)
		p.Choices[0] = ""
		if !routeOf(t, Validate(p)).HasRoute("Validate.PlanInvalidChoice") {
			t.Error("not PlanInvalidChoice")
		}
	})
}

func TestGroupNotEmpty(t *testing.T) {
	type tc struct {
		name      string
		a, b      *model.Database
		choices   map[byte]plan.Choice
		conflicts []byte // the UUID bytes of the kept items that are in the way
	}
	tests := []tc{
		{
			name:      "an entry only in B is kept in a dropped group of B",
			a:         database(grp(1, "Root"), 0),
			b:         database(grp(1, "Root", grp(3, "Games", ent(21, "E", "p", 1))), 0),
			choices:   map[byte]plan.Choice{3: plan.Drop},
			conflicts: []byte{21},
		},
		{
			name:      "an entry only in A is kept in a dropped group of A",
			a:         database(grp(1, "Root", grp(3, "Games", ent(21, "E", "p", 1))), 0),
			b:         database(grp(1, "Root"), 0),
			choices:   map[byte]plan.Choice{3: plan.Drop},
			conflicts: []byte{21},
		},
		{
			name:      "a kept subgroup in a dropped group",
			a:         database(grp(1, "Root"), 0),
			b:         database(grp(1, "Root", grp(3, "Games", grp(4, "Old"))), 0),
			choices:   map[byte]plan.Choice{3: plan.Drop},
			conflicts: []byte{4},
		},
		{
			name:    "a dropped subgroup of a dropped group is fine",
			a:       database(grp(1, "Root"), 0),
			b:       database(grp(1, "Root", grp(3, "Games", grp(4, "Old"))), 0),
			choices: map[byte]plan.Choice{3: plan.Drop, 4: plan.Drop},
		},
		{
			name:      "every kept item directly in a dropped group is listed (22 is in group 4, which is kept)",
			a:         database(grp(1, "Root"), 0),
			b:         database(grp(1, "Root", grp(3, "Games", ent(21, "E", "p", 1), grp(4, "Old", ent(22, "F", "p", 1)), ent(23, "G", "p", 1))), 0),
			choices:   map[byte]plan.Choice{3: plan.Drop, 23: plan.Drop},
			conflicts: []byte{21, 4},
		},
		{
			name:      "an entry in a dropped group of B that differs and is taken from B",
			a:         database(grp(1, "Root", ent(11, "X", "a", 1)), 0),
			b:         database(grp(1, "Root", grp(3, "Games", ent(11, "X", "b", 2))), 0),
			choices:   map[byte]plan.Choice{3: plan.Drop, 11: plan.TakeB},
			conflicts: []byte{11},
		},
		{
			name:    "an entry in a dropped group of B that differs and is taken from A is fine",
			a:       database(grp(1, "Root", ent(11, "X", "a", 1)), 0),
			b:       database(grp(1, "Root", grp(3, "Games", ent(11, "X", "b", 2))), 0),
			choices: map[byte]plan.Choice{3: plan.Drop, 11: plan.TakeA},
		},
		{
			name:      "both puts a copy in B's dropped group",
			a:         database(grp(1, "Root", ent(11, "X", "a", 1)), 0),
			b:         database(grp(1, "Root", grp(3, "Games", ent(11, "X", "b", 2))), 0),
			choices:   map[byte]plan.Choice{3: plan.Drop, 11: plan.Both},
			conflicts: []byte{11},
		},
		{
			name:    "an entry that leaves a dropped group of A by take-b is fine",
			a:       database(grp(1, "Root", grp(3, "Games", ent(11, "X", "a", 1))), 0),
			b:       database(grp(1, "Root", ent(11, "X", "b", 2)), 0),
			choices: map[byte]plan.Choice{3: plan.Drop, 11: plan.TakeB},
		},
		{
			name:      "an entry B deleted is kept in a dropped group of A",
			a:         database(grp(1, "Root", grp(3, "Games", ent(11, "X", "a", 1))), 0),
			b:         database(grp(1, "Root"), 0, deleted(11, 5)),
			choices:   map[byte]plan.Choice{3: plan.Drop, 11: plan.Keep},
			conflicts: []byte{11},
		},
		{
			name:      "an entry A deleted is kept in a dropped group of B",
			a:         database(grp(1, "Root"), 0, deleted(11, 5)),
			b:         database(grp(1, "Root", grp(3, "Games", ent(11, "X", "b", 1))), 0),
			choices:   map[byte]plan.Choice{3: plan.Drop, 11: plan.Keep},
			conflicts: []byte{11},
		},
		{
			name:    "a group that both have is never a conflict",
			a:       database(grp(1, "Root", grp(3, "Games", ent(11, "X", "a", 1))), 0),
			b:       database(grp(1, "Root", grp(3, "Games")), 0),
			choices: map[byte]plan.Choice{},
		},
		{
			name:    "a group moved out of a dropped group by take-b is fine",
			a:       database(grp(1, "Root", grp(3, "Games", grp(4, "Old"))), 0),
			b:       database(grp(1, "Root", grp(4, "Old")), 0),
			choices: map[byte]plan.Choice{3: plan.Drop, 4: plan.TakeB},
		},
		{
			name:      "a group kept by take-a in a dropped group",
			a:         database(grp(1, "Root", grp(3, "Games", grp(4, "Old"))), 0),
			b:         database(grp(1, "Root", grp(4, "Old")), 0),
			choices:   map[byte]plan.Choice{3: plan.Drop, 4: plan.TakeA},
			conflicts: []byte{4},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := planFor(t, tt.a, tt.b, tt.choices)
			var got []byte
			for _, c := range Conflicts(p) {
				got = append(got, p.Changes[c.Item].UUID[15])
			}
			if string(got) != string(tt.conflicts) {
				t.Fatalf("conflicts = %v, want %v", got, tt.conflicts)
			}
			err := Validate(p)
			if len(tt.conflicts) == 0 {
				if err != nil {
					t.Fatalf("Validate: %v", err)
				}
				mustApply(t, tt.a, tt.b, p)
				return
			}
			appErr := routeOf(t, err)
			if !appErr.HasRoute("Validate.PlanGroupNotEmpty") {
				t.Fatalf("route = %s", appErr.Route())
			}
			if appErr.Metadata["item"] != u(tt.conflicts[0]).String() {
				t.Errorf("the error names the item %v, want %s", appErr.Metadata["item"], u(tt.conflicts[0]))
			}
			if _, err := Apply(tt.a, tt.b, p); !routeOf(t, err).HasRoute("Apply->Validate.PlanGroupNotEmpty") {
				t.Errorf("Apply: %v", err)
			}
		})
	}
}

// The GUI offers to drop everything in the group; doing that makes the plan valid.
func TestDroppingTheConflictsFixesThePlan(t *testing.T) {
	a := database(grp(1, "Root"), 0)
	b := database(grp(1, "Root", grp(3, "Games", ent(21, "E", "p", 1), grp(4, "Old", ent(22, "F", "p", 1), ent(23, "G", "p", 1)))), 0)
	p := planFor(t, a, b, map[byte]plan.Choice{3: plan.Drop})
	for round := 0; len(Conflicts(p)) > 0; round++ {
		if round > 5 {
			t.Fatal("the conflicts do not go away")
		}
		for _, c := range Conflicts(p) {
			p.Set(c.Item, plan.Drop)
		}
	}
	if err := Validate(p); err != nil {
		t.Fatal(err)
	}
	res := mustApply(t, a, b, p)
	if got := render(res); got != "Root{}" {
		t.Errorf("tree = %s", got)
	}
}

func TestConflictsNameTheGroupChange(t *testing.T) {
	a := database(grp(1, "Root"), 0)
	b := database(grp(1, "Root", grp(3, "Games", ent(21, "E", "p", 1))), 0)
	p := planFor(t, a, b, map[byte]plan.Choice{3: plan.Drop})
	cs := Conflicts(p)
	if len(cs) != 1 {
		t.Fatalf("conflicts = %v", cs)
	}
	if g := p.Changes[cs[0].Group]; g.Item != diff.ItemGroup || g.UUID != u(3) {
		t.Errorf("the group change is %+v", g)
	}
	appErr := routeOf(t, Validate(p))
	if appErr.Metadata["group"] != u(3).String() || appErr.Metadata["group path"] != "Games" || appErr.Metadata["item title"] != "E" {
		t.Errorf("metadata = %v", appErr.Metadata)
	}
}
