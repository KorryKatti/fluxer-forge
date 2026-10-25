package token

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFromEnv(t *testing.T) {
	t.Setenv("FLUXER_TOKEN", "  abc123  ")
	if got := FromEnv(); got != "abc123" {
		t.Errorf("FromEnv = %q, want abc123", got)
	}
}

func TestFromDotEnv(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	content := "# bot settings\nFLUXER_TOKEN=\"abc123\"\nOTHER=value\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := FromDotEnv(path)
	if err != nil {
		t.Fatalf("FromDotEnv: %v", err)
	}
	if got != "abc123" {
		t.Errorf("token = %q, want abc123", got)
	}
}

func TestFromDotEnvMissingFileIsEmpty(t *testing.T) {
	got, err := FromDotEnv(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatalf("FromDotEnv: %v", err)
	}
	if got != "" {
		t.Errorf("token = %q, want empty", got)
	}
}

func TestFromFileTrimsWhitespace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	if err := os.WriteFile(path, []byte("abc123\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := FromFile(path)
	if err != nil {
		t.Fatalf("FromFile: %v", err)
	}
	if got != "abc123" {
		t.Errorf("token = %q, want abc123", got)
	}
}

func TestCleanValueQuotes(t *testing.T) {
	for input, want := range map[string]string{
		`"abc"`:   "abc",
		`'abc'`:   "abc",
		` abc `:   "abc",
		`"abc`:    `"abc`,
		`abc"def`: `abc"def`,
	} {
		if got := cleanValue(input); got != want {
			t.Errorf("cleanValue(%q) = %q, want %q", input, got, want)
		}
	}
}
