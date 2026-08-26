package main

import (
	"flag"
	"testing"
)

// The bug this guards: flag.FlagSet stops parsing at the first non-flag
// token, so `leagues publish eng-premier-league -by=x -reason=y` parsed the
// slug and then silently discarded both flags. Because -by and -reason are
// mandatory, that turned a required-argument check into something that
// depended on argument order — and the natural order was the broken one.
func TestParseFlagsAcceptsAnyArgumentOrder(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"flags after the slug", []string{"epl", "-by=a@b.c", "-reason=why"}},
		{"flags before the slug", []string{"-by=a@b.c", "-reason=why", "epl"}},
		{"flags either side", []string{"-by=a@b.c", "epl", "-reason=why"}},
		{"space-separated values", []string{"epl", "-by", "a@b.c", "-reason", "why"}},
		{"space-separated, slug last", []string{"-by", "a@b.c", "-reason", "why", "epl"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fs := flag.NewFlagSet("test", flag.ContinueOnError)
			by := fs.String("by", "", "")
			reason := fs.String("reason", "", "")

			positional, err := parseFlags(fs, tc.args)
			if err != nil {
				t.Fatalf("parseFlags: %v", err)
			}
			if len(positional) != 1 || positional[0] != "epl" {
				t.Errorf("positional = %v, want [epl]", positional)
			}
			if *by != "a@b.c" {
				t.Errorf("-by = %q, want a@b.c — a dropped flag here would skip the operator check", *by)
			}
			if *reason != "why" {
				t.Errorf("-reason = %q, want why", *reason)
			}
		})
	}
}

// Two slugs is a typo, not a batch operation: publishing the wrong league is
// the expensive direction, so the caller is told rather than guessed at.
func TestParseFlagsReportsEveryPositional(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.String("by", "", "")

	positional, err := parseFlags(fs, []string{"epl", "-by=a@b.c", "seria-a"})
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}
	if len(positional) != 2 {
		t.Fatalf("positional = %v, want both slugs so the caller can reject the typo", positional)
	}
}

func TestParseFlagsRejectsUnknownFlag(t *testing.T) {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(discard{})
	fs.String("by", "", "")

	if _, err := parseFlags(fs, []string{"epl", "-nope=1"}); err == nil {
		t.Error("parseFlags accepted an unknown flag; a typo'd -reason must not pass silently")
	}
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }
