package main

import (
	"os"
	"testing"
)

func TestGeminiAPIKeyStorage(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	if err := saveGeminiAPIKey("test-key"); err != nil {
		t.Fatal(err)
	}
	key, err := geminiAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	if key != "test-key" {
		t.Fatalf("unexpected key: %q", key)
	}
	path, err := geminiAPIKeyPath()
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected key permissions 0600, got %o", info.Mode().Perm())
	}
}
