package cmd

import (
	"os"
	"testing"
)

// keepArgs saves and restores os.Args for tests that mutate it.
func keepArgs(t *testing.T) func() {
	t.Helper()
	orig := make([]string, len(os.Args))
	copy(orig, os.Args)
	return func() { os.Args = orig }
}

func TestMaybeDefaultToSpeak_NoArgs(t *testing.T) {
	defer keepArgs(t)()
	os.Args = []string{"sag"}
	maybeDefaultToSpeak()
	if got := len(os.Args); got != 1 {
		t.Fatalf("expected args unchanged, got %v", os.Args)
	}
}

func TestMaybeDefaultToSpeak_HelpFlags(t *testing.T) {
	cases := [][]string{
		{"sag", "--help"},
		{"sag", "-h"},
	}
	for _, args := range cases {
		t.Run(args[1], func(t *testing.T) {
			defer keepArgs(t)()
			os.Args = append([]string(nil), args...)
			maybeDefaultToSpeak()
			if got := os.Args[1]; got != args[1] {
				t.Fatalf("help flag should remain first arg, got %q", got)
			}
		})
	}
}

func TestMaybeDefaultToSpeak_KnownSubcommand(t *testing.T) {
	defer keepArgs(t)()
	os.Args = []string{"sag", "voices"}
	maybeDefaultToSpeak()
	if got := os.Args[1]; got != "voices" {
		t.Fatalf("expected subcommand preserved, got %v", os.Args)
	}
}

func TestMaybeDefaultToSpeak_DefaultsToSpeak(t *testing.T) {
	defer keepArgs(t)()
	os.Args = []string{"sag", "Hello", "world"}
	maybeDefaultToSpeak()
	want := []string{"sag", "speak", "Hello", "world"}
	if len(os.Args) != len(want) {
		t.Fatalf("args length mismatch: got %v want %v", os.Args, want)
	}
	for i := range want {
		if os.Args[i] != want[i] {
			t.Fatalf("args mismatch at %d: got %q want %q (full %v)", i, os.Args[i], want[i], os.Args)
		}
	}
}

func TestMaybeDefaultToSpeak_StripsLeadingDoubleDash(t *testing.T) {
	t.Run("help after sentinel", func(t *testing.T) {
		defer keepArgs(t)()
		os.Args = []string{"sag", "--", "--help"}
		maybeDefaultToSpeak()
		if got := os.Args; len(got) != 2 || got[1] != "--help" {
			t.Fatalf("expected sentinel removed leaving help flag, got %v", got)
		}
	})

	t.Run("text after sentinel", func(t *testing.T) {
		defer keepArgs(t)()
		os.Args = []string{"sag", "--", "Hi"}
		maybeDefaultToSpeak()
		want := []string{"sag", "speak", "Hi"}
		if len(os.Args) != len(want) {
			t.Fatalf("args length mismatch: got %v want %v", os.Args, want)
		}
		for i := range want {
			if os.Args[i] != want[i] {
				t.Fatalf("args mismatch at %d: got %q want %q (full %v)", i, os.Args[i], want[i], os.Args)
			}
		}
	})
}
