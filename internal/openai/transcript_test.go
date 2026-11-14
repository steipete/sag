package openai

import (
	"strings"
	"testing"
)

func TestCheckTranscript(t *testing.T) {
	for _, tc := range []struct {
		input, spoken string
		matches       bool
	}{
		{"Hello world!", "hello, WORLD.", true},
		{"Don't stop.", "Don’t stop!", true},
		{"AI-generated voice", "AI generated voice", true},
		{"你好，世界！", "你好 世界", true},
		{"Read every word", "Read word", false},
		{"Turn left", "Turn right", false},
		{"Call 1234", "Call 4321", false},
		{"42", "forty two", false},
		{"Hello", "Hello. Have a nice day!", false},
		{"Hello", "", false},
	} {
		if got := CheckTranscript(tc.input, tc.spoken); (got == "") != tc.matches {
			t.Errorf("%q / %q: %q", tc.input, tc.spoken, got)
		}
	}
	if got := CheckTranscript("private input", "private output"); strings.Contains(got, "private") {
		t.Fatal("warning disclosed narration")
	}
}
