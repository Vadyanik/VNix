package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestProfileHookUpdatesCommitsAndPushesToLocalRemote(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("optional profile hook requires Python 3")
	}
	script, err := filepath.Abs("../../scripts/update-profile-stats.sh")
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	repo, remote := filepath.Join(root, "profile"), filepath.Join(root, "remote.git")
	git := func(args ...string) string {
		t.Helper()
		output, err := exec.Command("git", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, output)
		}
		return strings.TrimSpace(string(output))
	}
	git("init", "--initial-branch=main", repo)
	git("-C", repo, "config", "user.name", "VNix Test")
	git("-C", repo, "config", "user.email", "vnix@example.test")
	readme := filepath.Join(repo, "README.md")
	initial := "# Profile\n![Rebuilds](https://img.shields.io/badge/System%20Rebuilds-41-blue)\n![Rebuilds Per Day](old)\n![Last Rebuild](old)\nOther content\n"
	if err := os.WriteFile(readme, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}
	git("-C", repo, "add", "README.md")
	git("-C", repo, "commit", "-m", "initial")
	git("init", "--bare", remote)
	git("-C", repo, "remote", "add", "origin", remote)
	git("-C", repo, "push", "-u", "origin", "main")
	t.Setenv("PROFILE_REPO_PATH", repo)
	t.Setenv("PROFILE_BIRTH_DATE", "2026-02-13")
	t.Setenv("PROFILE_ENABLED", "1")
	t.Setenv("PROFILE_PULL", "1")
	t.Setenv("PROFILE_UPDATE_BADGES", "1")
	t.Setenv("PROFILE_COMMIT", "1")
	t.Setenv("PROFILE_PUSH", "1")
	t.Setenv("PROFILE_BRANCH", "main")
	t.Setenv("PROFILE_REMOTE", "origin")
	t.Setenv("PROFILE_README", "README.md")
	if output, err := exec.Command("bash", script).CombinedOutput(); err != nil {
		t.Fatalf("hook failed: %v\n%s", err, output)
	}
	data, _ := os.ReadFile(readme)
	if !strings.Contains(string(data), "System%20Rebuilds-42-blue") || !strings.Contains(string(data), "Other content") {
		t.Fatalf("bad README update: %s", data)
	}
	if message := git("-C", repo, "log", "-1", "--format=%s"); !strings.HasPrefix(message, "profile: rebuild #42 (") {
		t.Fatalf("bad profile commit: %s", message)
	}
	if local, pushed := git("-C", repo, "rev-parse", "HEAD"), git("--git-dir", remote, "rev-parse", "main"); local != pushed {
		t.Fatal("profile commit was not pushed")
	}
	if err := os.WriteFile(readme, []byte("user edits\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("bash", script).CombinedOutput(); err == nil || !strings.Contains(string(output), "pending changes") {
		t.Fatalf("dirty profile should be rejected: %v, %s", err, output)
	}
	t.Setenv("PROFILE_ENABLED", "0")
	if output, err := exec.Command("bash", script).CombinedOutput(); err != nil {
		t.Fatalf("disabled hook failed: %v, %s", err, output)
	}
	data, _ = os.ReadFile(readme)
	if string(data) != "user edits\n" {
		t.Fatal("hook overwrote user edits")
	}
}
