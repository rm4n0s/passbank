package archtest

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
)

// siteStrs formats tag sites as "Tag file:line Func".
func siteStrs(sites []TagSite) []string {
	out := make([]string, len(sites))
	for i, s := range sites {
		out[i] = s.Tag + " " + s.File + ":" + strconv.Itoa(s.Line) + " " + s.Func
	}
	return out
}

var tagsFixture = filepath.Join("testdata", "tags")

func TestErrorTags(t *testing.T) {
	sites, bad, err := ErrorTags(tagsFixture)
	if err != nil {
		t.Fatalf("ErrorTags: %v", err)
	}
	wantSites := []string{
		"CommentOnly internal/core/a/a.go:22 CommentOnly",
		"FakeOnly internal/core/a/a.go:17 Fake",
		"ListedOnly internal/core/a/a.go:19 Listed",
		"NeverTested internal/core/a/a.go:15 Untested",
		"OpenFailed internal/core/a/a.go:5 Open",
		"ReadFailed internal/core/a/a.go:7 Read",
		"SaveFailed internal/core/a/a.go:11 T.Save",
		"SentinelTag internal/core/a/a.go:13 Sentinel",
	}
	if got := siteStrs(sites); !slices.Equal(got, wantSites) {
		t.Errorf("sites:\n got %q\nwant %q", got, wantSites)
	}
	wantBad := []string{RuleNonLiteralTag + " internal/core/a/a.go:24"}
	if got := strs(bad); !slices.Equal(got, wantBad) {
		t.Errorf("violations: got %q, want %q", got, wantBad)
	}
}

func TestUntestedErrorTags(t *testing.T) {
	tests := []struct {
		name string
		root string
		want []string
	}{
		{
			// OpenFailed is asserted by a HasRoute literal, ReadFailed by a table
			// string and a helper, SaveFailed by a .Tag comparison and SentinelTag
			// by errors.Is. A Tag named in a comment or made by a test fake counts
			// for nothing.
			name: "fixture",
			root: tagsFixture,
			want: []string{"CommentOnly", "FakeOnly", "ListedOnly", "NeverTested"},
		},
		{name: "clean", root: filepath.Join("testdata", "tags-clean"), want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			untested, _, err := UntestedErrorTags(tt.root)
			if err != nil {
				t.Fatalf("UntestedErrorTags: %v", err)
			}
			var got []string
			for _, s := range untested {
				got = append(got, s.Tag)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("untested: got %q, want %q", got, tt.want)
			}
		})
	}
}

// TestRepositoryErrorTags is CLAUDE.md's "every error Tag has a test that asserts
// its route", checked on Passbank itself.
func TestRepositoryErrorTags(t *testing.T) {
	root := filepath.Join("..", "..")
	untested, bad, err := UntestedErrorTags(root)
	if err != nil {
		t.Fatalf("UntestedErrorTags: %v", err)
	}
	excepted, err := ReadUntestedList(filepath.Join(root, "docs", "ERROR-TAGS-UNTESTED.md"))
	if err != nil {
		t.Fatalf("ReadUntestedList: %v", err)
	}
	for _, s := range untested {
		if _, listed := excepted[s.Tag]; !listed {
			t.Errorf("%s:%d: no test asserts the route of Tag %s", s.File, s.Line, s.Tag)
		}
	}
	for _, v := range bad {
		t.Errorf("%s:%d: %s", v.File, v.Line, v.Message)
	}
}

func TestReadUntestedList(t *testing.T) {
	got, err := ReadUntestedList(filepath.Join(tagsFixture, "docs", "ERROR-TAGS-UNTESTED.md"))
	if err != nil {
		t.Fatalf("ReadUntestedList: %v", err)
	}
	if reason, ok := got["ListedOnly"]; !ok || reason != "fixture for the exceptions file" || len(got) != 1 {
		t.Errorf("got %v, want only ListedOnly with its reason", got)
	}
	missing, err := ReadUntestedList(filepath.Join(t.TempDir(), "none.md"))
	if err != nil || len(missing) != 0 {
		t.Errorf("a missing file must be an empty list, got %v, %v", missing, err)
	}
}

func TestErrorTagsRoutes(t *testing.T) {
	t.Run("ErrorTagsExceptionsReadFailed", func(t *testing.T) {
		// A folder where the file should be cannot be read as a file.
		dir := filepath.Join(t.TempDir(), "ERROR-TAGS-UNTESTED.md")
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		_, err := ReadUntestedList(dir)
		assertRoute(t, err, "ReadUntestedList.ErrorTagsExceptionsReadFailed")
	})
	t.Run("walk errors", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "missing")
		_, _, err := ErrorTags(missing)
		assertRoute(t, err, "RootWalkFailed")
		_, _, err = UntestedErrorTags(missing)
		assertRoute(t, err, "RootWalkFailed")
	})
}
