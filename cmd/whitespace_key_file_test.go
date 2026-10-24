package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveSixtyDBKey_KeyFile(t *testing.T) {
	cases := []struct {
		name     string
		contents string
		want     string
		wantErr  string
	}{
		{name: "trimmed key", contents: "  sd-file-key \n", want: "sd-file-key"},
		{name: "empty file", contents: "", wantErr: "is empty"},
		{name: "whitespace only", contents: " \n\t\n", wantErr: "is empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resetProviderEnv(t)
			path := filepath.Join(t.TempDir(), "sixtydb.key")
			if err := os.WriteFile(path, []byte(tc.contents), 0o600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("SIXTYDB_API_KEY_FILE", path)

			key, err := resolveSixtyDBKey()
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want one containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if key != tc.want {
				t.Fatalf("key = %q, want %q", key, tc.want)
			}
		})
	}
}
