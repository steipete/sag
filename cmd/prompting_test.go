package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestPromptingCommandOutputsGuide(t *testing.T) {
	resetRootCommandState()

	// Drain through Cobra's writer rather than filling an unread OS pipe.
	var output bytes.Buffer
	rootCmd.SetOut(&output)
	t.Cleanup(func() { rootCmd.SetOut(nil) })

	rootCmd.SetArgs([]string{"prompting"})
	if err := rootCmd.Execute(); err != nil {
		t.Fatalf("prompting command failed: %v", err)
	}

	out := output.String()
	if !strings.Contains(out, "# sag prompting guide") {
		t.Fatalf("unexpected output: %q", out)
	}
}
