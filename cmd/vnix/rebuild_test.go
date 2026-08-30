package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestRebuildDefaults(t *testing.T) {
	if !enabled(nil) {
		t.Fatal("missing git toggles should default to enabled")
	}
}

func TestResolveGitStepsCascades(t *testing.T) {
	no := false
	yes := true

	steps := resolveGitSteps(Config{GitAdd: &no, GitCommit: &yes, GitPush: &yes})
	if steps.Add || steps.Commit || steps.Push {
		t.Fatalf("git_add=false should disable all git steps: %+v", steps)
	}

	steps = resolveGitSteps(Config{GitCommit: &no, GitPush: &yes})
	if !steps.Add || steps.Commit || steps.Push {
		t.Fatalf("git_commit=false should disable push only: %+v", steps)
	}
}

func TestFallbackCommitMessage(t *testing.T) {
	message := fallbackCommitMessage("test")
	matched, err := regexp.MatchString(`^test: [0-9]{4}-[0-9]{2}-[0-9]{2} [0-9]{2}:[0-9]{2}:[0-9]{2}$`, message)
	if err != nil {
		t.Fatal(err)
	}
	if !matched {
		t.Fatalf("unexpected fallback message: %q", message)
	}
}

func TestGenerateCommitMessageRequiresAIConfiguration(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("OLLAMA_MODEL", "")
	if _, err := generateCommitMessage("diff", "history"); err == nil {
		t.Fatal("expected missing AI configuration error")
	}
}

func TestRebuildDiagnosisPrompt(t *testing.T) {
	prompt := rebuildDiagnosisPrompt(fmt.Errorf("exit status 1"), "failure details")
	for _, expected := range []string{"exit status 1", "failure details", "Do not modify files"} {
		if !strings.Contains(prompt, expected) {
			t.Fatalf("prompt is missing %q: %s", expected, prompt)
		}
	}
}

func TestGitChangePreviewShowsDiffAndUntrackedFiles(t *testing.T) {
	setupRebuildTest(t, true)
	if err := os.WriteFile("new-package.nix", []byte("new file\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	preview, err := gitChangePreview()
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"M configuration.nix", "?? new-package.nix", "File: configuration.nix", "-initial", "+changed"} {
		if !strings.Contains(preview, expected) {
			t.Fatalf("preview is missing %q:\n%s", expected, preview)
		}
	}
}

func TestMinimalDiffOmitsGitMetadata(t *testing.T) {
	diff := "diff --git a/test.nix b/test.nix\nindex 123..456 100644\n--- a/test.nix\n+++ b/test.nix\n@@ -1 +1 @@\n-old\n+new\n unchanged\n"
	preview := minimalDiff(diff)
	for _, expected := range []string{"File: test.nix", "@@ -1 +1 @@", "-old", "+new"} {
		if !strings.Contains(preview, expected) {
			t.Fatalf("preview is missing %q: %s", expected, preview)
		}
	}
	if strings.Contains(preview, "index 123") || strings.Contains(preview, "unchanged") {
		t.Fatalf("preview should omit metadata and unchanged context: %s", preview)
	}
}

func TestRebuildFlowScenarios(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("OLLAMA_MODEL", "")
	oldOpenCodeBinary := openCodeBinary
	openCodeBinary = ""
	t.Cleanup(func() { openCodeBinary = oldOpenCodeBinary })

	t.Run("skips when git has no changes", func(t *testing.T) {
		bin := setupRebuildTest(t, false)
		writeScript(t, bin, "nixos-rebuild", ": > nixos-ran\n")

		if err := runRebuildCommand(Config{}); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat("nixos-ran"); !os.IsNotExist(err) {
			t.Fatalf("nixos-rebuild should not run without changes, stat error: %v", err)
		}
	})

	t.Run("stops when rebuild fails", func(t *testing.T) {
		bin := setupRebuildTest(t, true)
		writeScript(t, bin, "nixos-rebuild", "exit 7\n")

		if err := runRebuildCommand(Config{}); err == nil {
			t.Fatal("expected rebuild error")
		}
		if staged := gitOutput(t, "diff", "--cached", "--name-only"); staged != "" {
			t.Fatalf("git add should not run after rebuild failure, staged: %q", staged)
		}
	})

	t.Run("commits and pushes with fallback message", func(t *testing.T) {
		bin := setupRebuildTest(t, true)
		writeScript(t, bin, "nixos-rebuild", "exit 0\n")

		if err := runRebuildCommand(Config{}); err != nil {
			t.Fatal(err)
		}
		if got := gitOutput(t, "log", "-1", "--format=%s"); !strings.HasPrefix(got, "rebuild: ") {
			t.Fatalf("unexpected commit message: %q", got)
		}
		local := gitOutput(t, "rev-parse", "HEAD")
		remote := gitOutput(t, "rev-parse", "origin/master")
		if local != remote {
			t.Fatalf("push did not update origin: local=%s remote=%s", local, remote)
		}
	})

	t.Run("uses fallback message when AI is unavailable", func(t *testing.T) {
		bin := setupRebuildTest(t, true)
		writeScript(t, bin, "nixos-rebuild", "exit 0\n")
		noPush := false

		if err := runRebuildCommand(Config{GitPush: &noPush, CommitMessagePrefix: "fallback"}); err != nil {
			t.Fatal(err)
		}
		matched, err := regexp.MatchString(`^fallback: [0-9]{4}-[0-9]{2}-[0-9]{2} [0-9]{2}:[0-9]{2}:[0-9]{2}$`, gitOutput(t, "log", "-1", "--format=%s"))
		if err != nil {
			t.Fatal(err)
		}
		if !matched {
			t.Fatalf("fallback commit message was not used")
		}
	})

	t.Run("honors disabled commit push and hooks", func(t *testing.T) {
		bin := setupRebuildTest(t, true)
		writeScript(t, bin, "nixos-rebuild", "exit 0\n")
		noCommit := false
		noPush := false

		config := Config{
			GitCommit: &noCommit,
			GitPush:   &noPush,
			Hooks: Hooks{
				BeforeRebuild: []string{"printf before_rebuild >> hooks.log"},
				AfterRebuild:  []string{"printf ',after_rebuild' >> hooks.log"},
				BeforeCommit:  []string{"printf ',before_commit' >> hooks.log"},
				AfterCommit:   []string{"printf ',after_commit' >> hooks.log"},
				AfterPush:     []string{"printf ',after_push' >> hooks.log"},
			},
		}
		if err := runRebuildCommand(config); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile("hooks.log")
		if err != nil {
			t.Fatal(err)
		}
		if got := string(data); got != "before_rebuild,after_rebuild" {
			t.Fatalf("unexpected hooks: %q", got)
		}
		if got := gitOutput(t, "log", "-1", "--format=%s"); got != "initial" {
			t.Fatalf("commit should be disabled, got latest commit %q", got)
		}
	})

	t.Run("disabled git add cascades to commit and push", func(t *testing.T) {
		bin := setupRebuildTest(t, true)
		writeScript(t, bin, "nixos-rebuild", "exit 0\n")
		noAdd := false
		yesCommit := true
		yesPush := true

		if err := runRebuildCommand(Config{GitAdd: &noAdd, GitCommit: &yesCommit, GitPush: &yesPush}); err != nil {
			t.Fatal(err)
		}
		if staged := gitOutput(t, "diff", "--cached", "--name-only"); staged != "" {
			t.Fatalf("git add should be disabled, staged: %q", staged)
		}
		if got := gitOutput(t, "log", "-1", "--format=%s"); got != "initial" {
			t.Fatalf("commit should be cascade-disabled, got latest commit %q", got)
		}
		local := gitOutput(t, "rev-parse", "HEAD")
		remote := gitOutput(t, "rev-parse", "origin/master")
		if local != remote {
			t.Fatalf("push should be cascade-disabled, local=%s remote=%s", local, remote)
		}
	})
}

func setupRebuildTest(t *testing.T, dirty bool) string {
	t.Helper()
	oldDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	oldPath := os.Getenv("PATH")
	gitPath, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bashPath, err := exec.LookPath("bash")
	if err != nil {
		t.Fatal(err)
	}

	root := t.TempDir()
	dir := filepath.Join(root, "work")
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(gitPath, filepath.Join(bin, "git")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(bashPath, filepath.Join(bin, "bash")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldDir)
		_ = os.Setenv("PATH", oldPath)
	})

	runTestCommand(t, "git", "init", "--initial-branch=master")
	runTestCommand(t, "git", "config", "user.email", "vnix@example.test")
	runTestCommand(t, "git", "config", "user.name", "VNix Test")
	if err := os.WriteFile("configuration.nix", []byte("initial\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".gitignore", []byte(".vnix/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runTestCommand(t, "git", "add", ".")
	runTestCommand(t, "git", "commit", "-m", "initial")

	remote := filepath.Join(root, "remote.git")
	runTestCommand(t, "git", "init", "--bare", remote)
	runTestCommand(t, "git", "remote", "add", "origin", remote)
	runTestCommand(t, "git", "push", "-u", "origin", "master")

	if err := os.MkdirAll(".vnix", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := CreateStatsDB(); err != nil {
		t.Fatal(err)
	}
	if dirty {
		if err := os.WriteFile("configuration.nix", []byte("changed\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return bin
}

func writeScript(t *testing.T, dir, name, body string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func runTestCommand(t *testing.T, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %s failed: %v\n%s", name, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
}

func gitOutput(t *testing.T, args ...string) string {
	t.Helper()
	output, err := exec.Command("git", args...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output))
}
