package main

import (
	"os"
	"strings"
	"testing"
)

func TestInitCommandWithBranch(t *testing.T) {
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldDir) })

	if err := initCommand("26.05"); err != nil {
		t.Fatal(err)
	}
	config, err := os.ReadFile(".vnix/config.json")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(config), `"nixpkgs_branch": "nixos-26.05"`) {
		t.Fatalf("unexpected config: %s", config)
	}
	if _, err := os.Stat("modules/vnix_packages.nix"); err != nil {
		t.Fatal(err)
	}
}
