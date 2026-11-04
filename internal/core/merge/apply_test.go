package merge

import (
	"testing"

	"github.com/rm4n0s/errors"

	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/core/plan"
)

func TestApplyChoices(t *testing.T) {
	type tc struct {
		name    string
		a, b    func() *model.Database
		choices map[byte]plan.Choice
		want    string
	}
	tests := []tc{
		{
			name:    "only-a entry: keep",
			a:       func() *model.Database { return database(grp(1, "Root", grp(2, "Mail", ent(11, "X", "p", 1))), 0) },
			b:       func() *model.Database { return database(grp(1, "Root", grp(2, "Mail")), 0) },
			choices: map[byte]plan.Choice{11: plan.Keep},
			want:    "Root{Mail{11=X/p}}",
		},
		{
			name: "only-a entry: drop",
			a: func() *model.Database {
				return database(grp(1, "Root", grp(2, "Mail", ent(11, "X", "p", 1), ent(12, "Y", "p", 1))), 0)
			},
			b:       func() *model.Database { return database(grp(1, "Root", grp(2, "Mail")), 0) },
			choices: map[byte]plan.Choice{11: plan.Drop},
			want:    "Root{Mail{12=Y/p}}",
		},
		{
			name:    "only-b entry: keep goes to the group B had it in, after A's",
			a:       func() *model.Database { return database(grp(1, "Root", grp(2, "Mail", ent(11, "X", "p", 1))), 0) },
			b:       func() *model.Database { return database(grp(1, "Root", grp(2, "Mail", ent(21, "Y", "p", 1))), 0) },
			choices: map[byte]plan.Choice{21: plan.Keep},
			want:    "Root{Mail{11=X/p 21=Y/p}}",
		},
		{
			name:    "only-b entry: drop",
			a:       func() *model.Database { return database(grp(1, "Root", grp(2, "Mail")), 0) },
			b:       func() *model.Database { return database(grp(1, "Root", grp(2, "Mail", ent(21, "Y", "p", 1))), 0) },
			choices: map[byte]plan.Choice{21: plan.Drop},
			want:    "Root{Mail{}}",
		},
		{
			name: "only-b group with nested groups is created with its ancestors",
			a:    func() *model.Database { return database(grp(1, "Root", grp(2, "Mail")), 0) },
			b: func() *model.Database {
				return database(grp(1, "Root", grp(2, "Mail", grp(4, "Sub", grp(5, "Deep", ent(21, "E", "p", 1)))), grp(3, "Games")), 0)
			},
			want: "Root{Mail{Sub{Deep{21=E/p}}} Games{}}",
		},
		{
			name: "only-b group dropped with everything in it",
			a:    func() *model.Database { return database(grp(1, "Root", grp(2, "Mail")), 0) },
			b: func() *model.Database {
				return database(grp(1, "Root", grp(2, "Mail"), grp(3, "Games", grp(4, "Old", ent(21, "E", "p", 1)), ent(22, "F", "p", 1))), 0)
			},
			choices: map[byte]plan.Choice{3: plan.Drop, 4: plan.Drop, 21: plan.Drop, 22: plan.Drop},
			want:    "Root{Mail{}}",
		},
		{
			name: "only-b group dropped but its inner group kept elsewhere is not possible; a dropped inner group alone is fine",
			a:    func() *model.Database { return database(grp(1, "Root", grp(2, "Mail")), 0) },
			b: func() *model.Database {
				return database(grp(1, "Root", grp(2, "Mail"), grp(3, "Games", grp(4, "Old", ent(21, "E", "p", 1)))), 0)
			},
			choices: map[byte]plan.Choice{4: plan.Drop, 21: plan.Drop},
			want:    "Root{Mail{} Games{}}",
		},
		{
			name: "only-a group dropped with everything in it",
			a: func() *model.Database {
				return database(grp(1, "Root", grp(2, "Mail"), grp(3, "Games", grp(4, "Old", ent(21, "E", "p", 1)), ent(22, "F", "p", 1))), 0)
			},
			b:       func() *model.Database { return database(grp(1, "Root", grp(2, "Mail")), 0) },
			choices: map[byte]plan.Choice{3: plan.Drop, 4: plan.Drop, 21: plan.Drop, 22: plan.Drop},
			want:    "Root{Mail{}}",
		},
		{
			name: "differs take-a keeps A's entry where it is",
			a: func() *model.Database {
				return database(grp(1, "Root", grp(2, "Mail", ent(11, "X", "a", 1)), grp(3, "Work")), 0)
			},
			b: func() *model.Database {
				return database(grp(1, "Root", grp(2, "Mail"), grp(3, "Work", ent(11, "X", "b", 2))), 0)
			},
			choices: map[byte]plan.Choice{11: plan.TakeA},
			want:    "Root{Mail{11=X/a} Work{}}",
		},
		{
			name: "differs take-b moves the entry to B's group",
			a: func() *model.Database {
				return database(grp(1, "Root", grp(2, "Mail", ent(11, "X", "a", 1)), grp(3, "Work")), 0)
			},
			b: func() *model.Database {
				return database(grp(1, "Root", grp(2, "Mail"), grp(3, "Work", ent(11, "X", "b", 2))), 0)
			},
			choices: map[byte]plan.Choice{11: plan.TakeB},
			want:    "Root{Mail{} Work{11=X/b}}",
		},
		{
			name: "differs take-b in the same group keeps the place",
			a: func() *model.Database {
				return database(grp(1, "Root", grp(2, "Mail", ent(11, "X", "a", 1), ent(12, "Y", "a", 1), ent(13, "Z", "a", 1))), 0)
			},
			b: func() *model.Database {
				return database(grp(1, "Root", grp(2, "Mail", ent(12, "Y", "b", 2), ent(13, "Z", "a", 1), ent(11, "X", "a", 1))), 0)
			},
			choices: map[byte]plan.Choice{12: plan.TakeB},
			want:    "Root{Mail{11=X/a 12=Y/b 13=Z/a}}",
		},
		{
			name: "differs both keeps A's and adds B's copy in B's group",
			a: func() *model.Database {
				return database(grp(1, "Root", grp(2, "Mail", ent(11, "X", "a", 1)), grp(3, "Work")), 0)
			},
			b: func() *model.Database {
				return database(grp(1, "Root", grp(2, "Mail"), grp(3, "Work", ent(11, "X", "b", 2))), 0)
			},
			choices: map[byte]plan.Choice{11: plan.Both},
			want:    "Root{Mail{11=X/a} Work{*=X/b}}",
		},
		{
			name:    "differs drop removes the entry",
			a:       func() *model.Database { return database(grp(1, "Root", grp(2, "Mail", ent(11, "X", "a", 1))), 0) },
			b:       func() *model.Database { return database(grp(1, "Root", grp(2, "Mail", ent(11, "X", "b", 2))), 0) },
			choices: map[byte]plan.Choice{11: plan.Drop},
			want:    "Root{Mail{}}",
		},
		{
			name:    "deleted-in-a keep brings B's entry back",
			a:       func() *model.Database { return database(grp(1, "Root", grp(2, "Mail")), 0, deleted(11, 5)) },
			b:       func() *model.Database { return database(grp(1, "Root", grp(2, "Mail", ent(11, "X", "b", 1))), 0) },
			choices: map[byte]plan.Choice{11: plan.Keep},
			want:    "Root{Mail{11=X/b}}",
		},
		{
			name:    "deleted-in-a drop leaves it out",
			a:       func() *model.Database { return database(grp(1, "Root", grp(2, "Mail")), 0, deleted(11, 5)) },
			b:       func() *model.Database { return database(grp(1, "Root", grp(2, "Mail", ent(11, "X", "b", 1))), 0) },
			choices: map[byte]plan.Choice{11: plan.Drop},
			want:    "Root{Mail{}}",
		},
		{
			name:    "deleted-in-b keep keeps A's entry",
			a:       func() *model.Database { return database(grp(1, "Root", grp(2, "Mail", ent(11, "X", "a", 1))), 0) },
			b:       func() *model.Database { return database(grp(1, "Root", grp(2, "Mail")), 0, deleted(11, 5)) },
			choices: map[byte]plan.Choice{11: plan.Keep},
			want:    "Root{Mail{11=X/a}}",
		},
		{
			name:    "deleted-in-b drop removes A's entry",
			a:       func() *model.Database { return database(grp(1, "Root", grp(2, "Mail", ent(11, "X", "a", 1))), 0) },
			b:       func() *model.Database { return database(grp(1, "Root", grp(2, "Mail")), 0, deleted(11, 5)) },
			choices: map[byte]plan.Choice{11: plan.Drop},
			want:    "Root{Mail{}}",
		},
		{
			name: "group-differs take-a keeps A's name and place",
			a: func() *model.Database {
				return database(grp(1, "Root", grp(2, "Old name", grp(3, "In")), grp(4, "Other")), 0)
			},
			b: func() *model.Database {
				return database(grp(1, "Root", grp(2, "New name"), grp(4, "Other", grp(3, "In"))), 0)
			},
			choices: map[byte]plan.Choice{2: plan.TakeA, 3: plan.TakeA},
			want:    "Root{Old name{In{}} Other{}}",
		},
		{
			name: "group-differs take-b takes B's name and parent",
			a: func() *model.Database {
				return database(grp(1, "Root", grp(2, "Old name", grp(3, "In")), grp(4, "Other")), 0)
			},
			b: func() *model.Database {
				return database(grp(1, "Root", grp(2, "New name"), grp(4, "Other", grp(3, "In"))), 0)
			},
			choices: map[byte]plan.Choice{2: plan.TakeB, 3: plan.TakeB},
			want:    "Root{New name{} Other{In{}}}",
		},
		{
			name: "recycle bins with different UUIDs hold the entries of both",
			a:    func() *model.Database { return database(grp(1, "Root", grp(8, "Bin", ent(11, "A gone", "p", 1))), 8) },
			b:    func() *model.Database { return database(grp(1, "Root", grp(9, "Bin", ent(21, "B gone", "p", 1))), 9) },
			want: "Root{Bin{11=A gone/p 21=B gone/p}}",
		},
		{
			name: "roots with different UUIDs are one group",
			a:    func() *model.Database { return database(grp(1, "Root", ent(11, "A", "p", 1)), 0) },
			b:    func() *model.Database { return database(grp(2, "Root", ent(21, "B", "p", 1)), 0) },
			want: "Root{11=A/p 21=B/p}",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, b := tt.a(), tt.b()
			p := planFor(t, a, b, tt.choices)
			res := mustApply(t, a, b, p)
			// The copy `both` makes has a new UUID, written * in the expectation.
			got := renderKnown(res, entryUUIDs(a, b))
			if got != tt.want {
				t.Errorf("tree = %s\nwant   %s", got, tt.want)
			}
		})
	}
}

func TestApplyErrors(t *testing.T) {
	// A: X > Y, B: Y > X (the moves cross): taking B's parents for both is a cycle.
	a := database(grp(1, "Root", grp(2, "X", grp(3, "Y"))), 0)
	b := database(grp(1, "Root", grp(3, "Y", grp(2, "X"))), 0)

	t.Run("a cycle", func(t *testing.T) {
		p := planFor(t, a, b, map[byte]plan.Choice{2: plan.TakeB, 3: plan.TakeB})
		_, err := Apply(a, b, p)
		appErr, ok := errors.FromError(err)
		if !ok || !appErr.HasRoute("builder.moveGroup.MergeGroupCycle") {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("one of the two moves is fine", func(t *testing.T) {
		p := planFor(t, a, b, map[byte]plan.Choice{2: plan.TakeA, 3: plan.TakeB})
		res := mustApply(t, a, b, p)
		if got := render(res); got != "Root{X{} Y{}}" {
			t.Errorf("tree = %s", got)
		}
	})
	t.Run("a plan for other databases", func(t *testing.T) {
		other := database(grp(1, "Root", ent(11, "X", "p", 1)), 0)
		p := planFor(t, other, database(grp(1, "Root"), 0), nil)
		_, err := Apply(a, b, p)
		appErr, ok := errors.FromError(err)
		if !ok || !appErr.HasRoute("sameChanges.PlanMismatch") {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("a plan with a missing choice", func(t *testing.T) {
		p := planFor(t, a, b, nil)
		p.Choices = p.Choices[:len(p.Choices)-1]
		_, err := Apply(a, b, p)
		appErr, ok := errors.FromError(err)
		if !ok || !appErr.HasRoute("sameChanges.PlanMismatch") {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("something kept in a dropped group", func(t *testing.T) {
		a := database(grp(1, "Root"), 0)
		b := database(grp(1, "Root", grp(3, "Games", ent(21, "E", "p", 1))), 0)
		p := planFor(t, a, b, map[byte]plan.Choice{3: plan.Drop})
		_, err := apply(a, b, p) // Validate would refuse this plan first
		appErr, ok := errors.FromError(err)
		if !ok || !appErr.HasRoute("builder.groupOfB.PlanGroupNotEmpty") {
			t.Errorf("err = %v", err)
		}
	})
}
