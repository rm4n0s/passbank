package merge

import (
	"testing"

	"github.com/rm4n0s/passbank/internal/core/diff"
)

// The related databases of the property tests have changes of every kind, so
// that the rules are really tried on all of them.
func TestRelatedDatabasesCoverTheKinds(t *testing.T) {
	counts := map[diff.Kind]int{}
	for seed := uint64(1); seed <= 40; seed++ {
		a, b := related(seed)
		for _, c := range diff.Changes(a, b) {
			counts[c.Kind]++
		}
	}
	for _, k := range diff.Kinds {
		if counts[k] == 0 {
			t.Errorf("no change of kind %s in 40 related pairs (%v)", k, counts)
		}
	}
	t.Log(counts)
}
