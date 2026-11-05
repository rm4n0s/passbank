//go:build passbank_large

package cli

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/rm4n0s/passbank/internal/core/diff"
	"github.com/rm4n0s/passbank/internal/core/merge"
	"github.com/rm4n0s/passbank/internal/core/model"
	"github.com/rm4n0s/passbank/internal/core/model/modeltest"
	"github.com/rm4n0s/passbank/internal/core/plan"
)

// This is a timing measurement, so it runs alone and only when asked for:
//
//	scripts/heavy.sh --alone -- go test -tags passbank_large ./internal/cli -run Large -v
//
// With 10,000 entries in each input, comparing and merging take under 2 seconds
// without the key derivation (the files here use 1,000 rounds of the AES-KDF). The
// command `diff` has the same limit; `merge` also writes the result and reads it
// back to verify it (SPEC §5), which parses the 20,000 entries a third time, so its
// limit is 5 seconds.

const (
	largeEntries    = 10000
	largeLimit      = 2 * time.Second
	largeMergeLimit = 5 * time.Second
)

func TestLargeCore(t *testing.T) {
	a, b := modeltest.LargePair(largeEntries)
	start := time.Now()
	changes := diff.Changes(a, b)
	tChanges := time.Since(start)
	p := plan.New(plan.Input{}, plan.Input{}, changes)
	start = time.Now()
	res, err := merge.Apply(a, b, p)
	if err != nil {
		t.Fatal(err)
	}
	tApply := time.Since(start)
	entries := 0
	res.WalkEntries(func(_ *model.Group, _ *model.Entry) bool { entries++; return true })
	t.Logf("%d changes in %v; applying them (%d entries) in %v", len(changes), tChanges, entries, tApply)
	if tChanges+tApply > largeLimit {
		t.Errorf("comparing and merging took %v, over %v", tChanges+tApply, largeLimit)
	}
}

func TestLargeCommands(t *testing.T) {
	dir := t.TempDir()
	a, b := writeLarge(t, dir, largeEntries)
	pw := passwordFile(t, dir, "pw", modeltest.PairPasswordA)
	planPath := filepath.Join(dir, "plan.txt")

	start := time.Now()
	status, _, errs := run(t, &scriptedTerminal{}, "diff", a, b, "--password-file-a", pw, "--password-file-b", pw, "-o", planPath)
	tDiff := time.Since(start)
	if status != ExitOK {
		t.Fatalf("diff: %d %q", status, errs)
	}
	start = time.Now()
	status, _, errs = run(t, &scriptedTerminal{}, "merge", a, b, "-o", filepath.Join(dir, "out.kdbx"), "--password-file-a", pw, "--password-file-b", pw,
		"--plan", planPath, "--encryption", "a")
	tMerge := time.Since(start)
	if status != ExitOK {
		t.Fatalf("merge: %d %q", status, errs)
	}
	t.Logf("diff %v, merge %v (each opens both files and merge also writes and verifies the result)", tDiff, tMerge)
	if tDiff > largeLimit || tMerge > largeMergeLimit {
		t.Errorf("diff took %v (limit %v) and merge %v (limit %v)", tDiff, largeLimit, tMerge, largeMergeLimit)
	}
}
