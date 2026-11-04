package main

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	fixture := filepath.Join("..", "..", "testdata", "tags")
	clean := filepath.Join("..", "..", "testdata", "tags-clean")
	tests := []struct {
		name       string
		args       []string
		wantStatus int
		wantOut    []string // the first two tab-separated columns of each line
		wantErr    string   // a substring of stderr
	}{
		{
			name:       "every Tag",
			args:       []string{fixture},
			wantStatus: 1, // the fixture has a Tag that is not a literal
			wantOut: []string{
				"CommentOnly\tinternal/core/a/a.go:22", "FakeOnly\tinternal/core/a/a.go:17",
				"ListedOnly\tinternal/core/a/a.go:19", "NeverTested\tinternal/core/a/a.go:15",
				"OpenFailed\tinternal/core/a/a.go:5", "ReadFailed\tinternal/core/a/a.go:7",
				"SaveFailed\tinternal/core/a/a.go:11", "SentinelTag\tinternal/core/a/a.go:13",
			},
			wantErr: "internal/core/a/a.go:24: the Tag is not a string literal: tag",
		},
		{
			name:       "untested without the listed ones",
			args:       []string{"--untested", fixture},
			wantStatus: 1,
			wantOut: []string{
				"CommentOnly\tinternal/core/a/a.go:22", "FakeOnly\tinternal/core/a/a.go:17",
				"NeverTested\tinternal/core/a/a.go:15",
			},
			wantErr: "not a string literal",
		},
		{
			name:       "untested with --all",
			args:       []string{"--untested", "--all", fixture},
			wantStatus: 1,
			wantOut: []string{
				"CommentOnly\tinternal/core/a/a.go:22", "FakeOnly\tinternal/core/a/a.go:17",
				"ListedOnly\tinternal/core/a/a.go:19", "NeverTested\tinternal/core/a/a.go:15",
			},
			wantErr: "not a string literal",
		},
		{name: "nothing untested", args: []string{"--untested", clean}, wantStatus: 0},
		{name: "unreadable root", args: []string{"--untested", filepath.Join(t.TempDir(), "missing")}, wantStatus: 2, wantErr: "error-tags:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errs strings.Builder
			status := run(tt.args, &out, &errs)
			if status != tt.wantStatus {
				t.Errorf("status = %d, want %d (stderr: %s)", status, tt.wantStatus, errs.String())
			}
			var got []string
			for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
				if line == "" {
					continue
				}
				cols := strings.SplitN(line, "\t", 3)
				got = append(got, cols[0]+"\t"+cols[1])
			}
			if !slices.Equal(got, tt.wantOut) {
				t.Errorf("stdout:\n got %q\nwant %q", got, tt.wantOut)
			}
			if !strings.Contains(errs.String(), tt.wantErr) {
				t.Errorf("stderr = %q, want it to contain %q", errs.String(), tt.wantErr)
			}
		})
	}
}
