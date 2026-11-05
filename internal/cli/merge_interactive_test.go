package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rm4n0s/passbank/internal/core/model/modeltest"
)

// interactive runs `passbank merge` without --plan and --prefer on the merge
// fixtures with a terminal that answers with the lines given. It returns the exit
// status, stdout, stderr, the transcript of the terminal (prompts, answers and
// the text shown) and the path of the output.
func interactive(t *testing.T, answers []string, extra ...string) (status int, stdout, stderr, transcript, out string) {
	t.Helper()
	dir := t.TempDir()
	out = filepath.Join(dir, "result.kdbx")
	term := &scriptedTerminal{tty: true, answers: answers}
	status, stdout, stderr = run(t, term, mergeArgs(t, dir, out, extra...)...)
	return status, stdout, stderr, term.out.String(), out
}

// times returns n copies of s.
func times(n int, s string) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = s
	}
	return out
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestInteractiveAcceptingTheDefaultsIsPreferNewer(t *testing.T) {
	status, stdout, stderr, _, out := interactive(t, append(times(9, ""), "y"))
	if status != ExitOK || stderr != "" {
		t.Fatalf("status %d, stderr %q", status, stderr)
	}
	db, _ := openResult(t, out, passwordCreds(modeltest.PairPasswordA))
	dir := t.TempDir()
	outRule := filepath.Join(dir, "rule.kdbx")
	if status, _, errs := run(t, &scriptedTerminal{}, mergeArgs(t, dir, outRule, "--prefer", "newer")...); status != ExitOK {
		t.Fatalf("--prefer newer: %d %q", status, errs)
	}
	want, _ := openResult(t, outRule, passwordCreds(modeltest.PairPasswordA))
	if listing(db) != listing(want) {
		t.Errorf("interactive defaults differ from --prefer newer:\n%s\n---\n%s", listing(db), listing(want))
	}
	if !strings.Contains(stdout, "written: ") {
		t.Errorf("stdout = %q", stdout)
	}
}

func TestInteractiveTranscript(t *testing.T) {
	answers := []string{
		"?", "zz", "d", // 1 Work mail: help, an invalid key, drop
		"",           // 2 group Games: the default (keep)
		"p", "", "k", // 3 back to Games, Enter, then Old forum: keep
		"v", "t", // 4 My bank: show the values, keep both
		"b",   // 5 Store: B's
		"k",   // 6 Old account: keep (bring it back)
		" D ", // 7 Stale login: drop (upper case and spaces)
		"a",   // 8 Shopping: A's
		"b",   // 9 settings: B's
		"y",
	}
	status, stdout, stderr, transcript, out := interactive(t, answers)
	if status != ExitOK || stderr != "" {
		t.Fatalf("status %d, stderr %q\n%s", status, stderr, transcript)
	}
	a, b := mergeFixtures(t)
	text := normalize(transcript, fixturesDir(t))
	text = strings.ReplaceAll(text, out, "<out>")
	_, _ = a, b
	checkGolden(t, "merge-interactive.transcript.txt", text)
	checkGolden(t, "merge-interactive.txt", strings.ReplaceAll(normalize(stdout, fixturesDir(t)), out, "<out>"))

	db, _ := openResult(t, out, passwordCreds(modeltest.PairPasswordA))
	checkGolden(t, "merge-interactive.entries.txt", listing(db))

	// The values `v` showed were printed once, on the terminal, and nowhere else.
	if n := strings.Count(transcript, "bank-password-a"); n != 1 {
		t.Errorf("the password of A is shown %d times, want once", n)
	}
	if strings.Contains(stdout, "bank-password") || strings.Contains(stderr, "bank-password") {
		t.Error("a password reached stdout or stderr")
	}
	for _, s := range pairSecrets {
		if s == "bank-password-a" || s == "bank-password-b" {
			continue
		}
		if strings.Contains(transcript, s) {
			t.Errorf("the transcript shows the secret %q", s)
		}
	}
}

func TestInteractiveLeavingWritesNothing(t *testing.T) {
	tests := []struct {
		name    string
		answers []string
	}{
		{"q at the first change", []string{"q"}},
		{"q in the middle", []string{"", "", "Q"}},
		{"the end of the input", []string{"", ""}},
		{"no at the question", append(times(9, ""), "n")},
		{"Enter at the question", append(times(9, ""), "")},
		{"the end of the input at the question", times(9, "")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, stdout, stderr, _, out := interactive(t, tt.answers)
			if status != ExitFailure || stdout != "" || !strings.Contains(stderr, "stopped; nothing was written") {
				t.Errorf("status %d, stdout %q, stderr %q", status, stdout, stderr)
			}
			if fileExists(out) {
				t.Error("a file was written")
			}
		})
	}
}

func TestInteractiveWithoutATerminal(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "result.kdbx")
	status, stdout, stderr := run(t, &scriptedTerminal{tty: false}, append(mergeArgs(t, dir, out), "--debug")...)
	if status != ExitFailure || stdout != "" || !strings.Contains(stderr, "no terminal to ask for choices; use --plan or --prefer") || !strings.Contains(stderr, "tag:   ChoicesNeedTerminal") {
		t.Errorf("status %d, stdout %q, stderr %q", status, stdout, stderr)
	}
	if fileExists(out) {
		t.Error("a file was written")
	}
}

func TestInteractiveKeysAndRules(t *testing.T) {
	// The changes in order: 1 Work mail, 2 group Games, 3 Old forum, 4 My bank, 5 Store,
	// 6 Old account, 7 Stale login, 8 group Shopping, 9 settings.
	tests := []struct {
		name    string
		answers []string
		want    []string // substrings of the listing
		not     []string
	}{
		{
			name:    "a rule for this change and the rest",
			answers: []string{"", "", "", "r", "b", "y"}, // from 4: prefer B
			want:    []string{"Banking/My bank  password=bank-password-b", "Shopping/Store (old name)", "Social/Old account", "name=Laptop"},
			not:     []string{"Social/Stale login"},
		},
		{
			name:    "the rule a, and the rule is asked again after a wrong word",
			answers: []string{"r", "nope", "a", "y"},
			want:    []string{"Banking/My bank  password=bank-password-a", "name=Personal", "Email/Work mail"},
			not:     []string{"Games", "Old forum", "Old account"},
		},
		{
			name:    "a rule can be cancelled with Enter",
			answers: append([]string{"r", ""}, append(times(9, ""), "y")...),
			want:    []string{"Banking/My bank  password=bank-password-b"},
		},
		{
			name:    "the union rule keeps both versions",
			answers: []string{"r", "u", "y"},
			want:    []string{"Banking/My bank  password=bank-password-a", "Banking/My bank  password=bank-password-b", "Games", "Old forum"},
		},
		{
			name:    "going back changes an earlier answer",
			answers: []string{"d", "p", "k", "", "", "", "", "", "", "", "", "y"}, // 1: drop, back, keep
			want:    []string{"Email/Work mail"},
		},
		{
			name:    "p at the first change stays there",
			answers: append([]string{"p"}, append(times(9, ""), "y")...),
			want:    []string{"Email/Work mail"},
		},
		{
			name:    "keys that the kind does not allow are refused",
			answers: append([]string{"t", "a", "b"}, append(times(9, ""), "y")...), // only-a: keep or drop
			want:    []string{"Email/Work mail"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, _, stderr, transcript, out := interactive(t, tt.answers)
			if status != ExitOK {
				t.Fatalf("status %d, stderr %q\n%s", status, stderr, transcript)
			}
			db, _ := openResult(t, out, passwordCreds(modeltest.PairPasswordA))
			got := listing(db)
			for _, w := range tt.want {
				if !strings.Contains(got, w) {
					t.Errorf("the result lacks %q:\n%s", w, got)
				}
			}
			for _, n := range tt.not {
				if strings.Contains(got, n) {
					t.Errorf("the result has %q:\n%s", n, got)
				}
			}
		})
	}
}

func TestInteractiveDroppingAGroup(t *testing.T) {
	t.Run("also dropping what is inside", func(t *testing.T) {
		// 2 group Games: drop, and drop the entry in it too; then Enter on the entry.
		answers := []string{"", "d", "y", "", "", "", "", "", "", "", "y"}
		status, _, stderr, transcript, out := interactive(t, answers)
		if status != ExitOK {
			t.Fatalf("status %d, stderr %q\n%s", status, stderr, transcript)
		}
		if !strings.Contains(transcript, "Also drop the 1 items inside group Games? [y/N] ") || !strings.Contains(transcript, "Enter = drop (default: keep)") {
			t.Errorf("transcript:\n%s", transcript)
		}
		db, _ := openResult(t, out, passwordCreds(modeltest.PairPasswordA))
		if got := listing(db); strings.Contains(got, "Games") || strings.Contains(got, "Old forum") {
			t.Errorf("Games is in the result:\n%s", got)
		}
	})
	t.Run("answering no keeps the group and asks again", func(t *testing.T) {
		answers := []string{"", "d", "n", "", "", "", "", "", "", "", "", "y"}
		status, _, stderr, transcript, out := interactive(t, answers)
		if status != ExitOK {
			t.Fatalf("status %d, stderr %q\n%s", status, stderr, transcript)
		}
		if !strings.Contains(transcript, "the group stays") {
			t.Errorf("transcript:\n%s", transcript)
		}
		db, _ := openResult(t, out, passwordCreds(modeltest.PairPasswordA))
		if got := listing(db); !strings.Contains(got, "group Games/") || !strings.Contains(got, "Games/Old forum") {
			t.Errorf("Games is not in the result:\n%s", got)
		}
	})
	t.Run("keeping an item in a dropped group asks to keep the group", func(t *testing.T) {
		// Games dropped with its entry; at the entry, keep: answer yes (keep the group).
		answers := []string{"", "d", "y", "k", "y", "", "", "", "", "", "", "y"}
		status, _, stderr, transcript, out := interactive(t, answers)
		if status != ExitOK {
			t.Fatalf("status %d, stderr %q\n%s", status, stderr, transcript)
		}
		if !strings.Contains(transcript, "group Games is dropped, and this choice keeps something in it. Keep the group too? [Y/n] ") {
			t.Errorf("transcript:\n%s", transcript)
		}
		db, _ := openResult(t, out, passwordCreds(modeltest.PairPasswordA))
		if got := listing(db); !strings.Contains(got, "Games/Old forum") {
			t.Errorf("the entry is not in the result:\n%s", got)
		}
	})
	t.Run("answering no leaves the entry dropped", func(t *testing.T) {
		answers := []string{"", "d", "y", "k", "n", "", "", "", "", "", "", "", "y"}
		status, _, stderr, transcript, out := interactive(t, answers)
		if status != ExitOK {
			t.Fatalf("status %d, stderr %q\n%s", status, stderr, transcript)
		}
		if !strings.Contains(transcript, "unchanged") {
			t.Errorf("transcript:\n%s", transcript)
		}
		db, _ := openResult(t, out, passwordCreds(modeltest.PairPasswordA))
		if got := listing(db); strings.Contains(got, "Games") {
			t.Errorf("Games is in the result:\n%s", got)
		}
	})
	t.Run("a rule that leaves a kept item in a dropped group is settled at the end", func(t *testing.T) {
		// Games dropped with its entry; at the entry the rule union keeps it again.
		for _, tt := range []struct {
			answer    string
			wantGames bool
		}{{"y", true}, {"", true}, {"n", false}} {
			answers := []string{"", "d", "y", "r", "union", tt.answer, "y"}
			status, _, stderr, transcript, out := interactive(t, answers)
			if status != ExitOK {
				t.Fatalf("answer %q: status %d, stderr %q\n%s", tt.answer, status, stderr, transcript)
			}
			if !strings.Contains(transcript, "Keep the group? [Y/n] ") {
				t.Errorf("transcript:\n%s", transcript)
			}
			db, _ := openResult(t, out, passwordCreds(modeltest.PairPasswordA))
			if got := listing(db); strings.Contains(got, "Games/Old forum") != tt.wantGames {
				t.Errorf("answer %q: Games in the result = %v:\n%s", tt.answer, !tt.wantGames, got)
			}
		}
	})
}

func TestInteractiveHelpAndValues(t *testing.T) {
	// ? and v on a change with no password to show.
	answers := []string{"?", "v", "d", "", "", "", "", "", "", "", "", "y"}
	status, _, stderr, transcript, _ := interactive(t, answers)
	if status != ExitOK {
		t.Fatalf("status %d, stderr %q\n%s", status, stderr, transcript)
	}
	for _, want := range []string{"q  quit; nothing is written", "no values differ here"} {
		if !strings.Contains(transcript, want) {
			t.Errorf("the transcript lacks %q:\n%s", want, transcript)
		}
	}
}

func TestInteractiveSettingsFromOverridesTheAnswer(t *testing.T) {
	status, _, stderr, transcript, out := interactive(t, append(times(8, ""), "a", "y"), "--settings-from", "b")
	if status != ExitOK {
		t.Fatalf("status %d, stderr %q\n%s", status, stderr, transcript)
	}
	db, _ := openResult(t, out, passwordCreds(modeltest.PairPasswordA))
	if got := listing(db); !strings.HasPrefix(got, "name=Laptop") {
		t.Errorf("the settings are not B's:\n%s", got)
	}
}

func TestNeedTerminalRoute(t *testing.T) {
	for _, e := range []env{{}, {term: &scriptedTerminal{tty: false}}} {
		if err := needTerminal(e); !routeIs(err, "needTerminal.ChoicesNeedTerminal") {
			t.Errorf("err = %v", err)
		}
	}
	if err := needTerminal(env{term: &scriptedTerminal{tty: true}}); err != nil {
		t.Errorf("with a terminal: %v", err)
	}
}
