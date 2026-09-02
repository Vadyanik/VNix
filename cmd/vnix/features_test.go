package main

import (
	"os"
	"strings"
	"testing"
)

func TestManagedPackagesAndProfiles(t *testing.T) {
	setupFeatureProject(t)
	if err := writeManagedPackages([]string{"git", "neovim"}); err != nil {
		t.Fatal(err)
	}
	packages, err := readManagedPackages()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(packages, ",") != "git,neovim" {
		t.Fatalf("unexpected packages: %v", packages)
	}
	if err := saveProfile("minimal", packages); err != nil {
		t.Fatal(err)
	}
	if err := writeManagedPackages([]string{"firefox"}); err != nil {
		t.Fatal(err)
	}
	if err := applyProfile("minimal"); err != nil {
		t.Fatal(err)
	}
	packages, err = readManagedPackages()
	if err != nil || strings.Join(packages, ",") != "git,neovim" {
		t.Fatalf("profile was not applied: %v, %v", packages, err)
	}
}

func TestCreateAndRestoreBackup(t *testing.T) {
	setupFeatureProject(t)
	backup, err := createBackup()
	if err != nil {
		t.Fatal(err)
	}
	if err := writeManagedPackages([]string{"changed"}); err != nil {
		t.Fatal(err)
	}
	if err := restoreBackup(backup.Name); err != nil {
		t.Fatal(err)
	}
	packages, err := readManagedPackages()
	if err != nil || strings.Join(packages, ",") != "git" {
		t.Fatalf("backup was not restored: %v, %v", packages, err)
	}
}

func TestRestoreCorruptBackupDoesNotChangeFiles(t *testing.T) {
	setupFeatureProject(t)
	original, err := os.ReadFile(managedPackagesPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(".vnix/backups", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".vnix/backups/corrupt.tar.gz", []byte("not a gzip archive"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := restoreBackup("corrupt.tar.gz"); err == nil {
		t.Fatal("expected corrupt backup error")
	}
	after, err := os.ReadFile(managedPackagesPath)
	if err != nil || string(after) != string(original) {
		t.Fatalf("corrupt backup changed managed packages: %q, %v", after, err)
	}
}

func TestCustomManagedPackagesFile(t *testing.T) {
	setupFeatureProject(t)
	if err := os.MkdirAll("custom", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(managedPackagesPath, "custom/packages.nix"); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".vnix/config.json", []byte(`{"managed_packages_file":"custom/packages.nix"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := InstallCommand("neovim"); err != nil {
		t.Fatal(err)
	}
	packages, err := readManagedPackages()
	if err != nil || strings.Join(packages, ",") != "git,neovim" {
		t.Fatalf("custom managed file was not used: %v, %v", packages, err)
	}
}

func TestParseGenerationsAndPatch(t *testing.T) {
	generations, err := parseGenerations(`[{"generation":42,"date":"2026-08-30","nixosVersion":"26.11","current":true}]`)
	if err != nil || len(generations) != 1 || generations[0].Number != 42 || !generations[0].Current {
		t.Fatalf("unexpected generations: %v, %v", generations, err)
	}
	patch, err := extractPatch("explanation\ndiff --git a/x b/x\n--- a/x\n+++ b/x\n@@ -1 +1 @@\n-old\n+new")
	if err != nil || !strings.HasPrefix(patch, "diff --git") {
		t.Fatalf("unexpected patch: %q, %v", patch, err)
	}
}

func TestRunSecurityScan(t *testing.T) {
	result, err := runSecurityScan("printf safe")
	if err != nil || result.Output != "safe" {
		t.Fatalf("unexpected scan result: %#v, %v", result, err)
	}
}

func TestSaveSecurityScanCommand(t *testing.T) {
	setupFeatureProject(t)
	if err := saveSecurityScanCommand("printf safe"); err != nil {
		t.Fatal(err)
	}
	config, err := readConfig()
	if err != nil || config.SecurityScanCommand != "printf safe" {
		t.Fatalf("unexpected security scan configuration: %#v, %v", config, err)
	}
}

func setupFeatureProject(t *testing.T) {
	t.Helper()
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldDir) })
	if err := os.MkdirAll("modules", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(".vnix", 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(managedPackagesPath, []byte("{ pkgs, ... }:\n{\n  environment.systemPackages = with pkgs; [\n    # vnix:start\n    git\n    # vnix:end\n  ];\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".vnix/config.json", []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}
