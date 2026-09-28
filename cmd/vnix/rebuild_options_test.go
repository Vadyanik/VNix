package main

import (
	"os"
	"strings"
	"testing"
)

func TestRebuildStagesAllBeforeBuildingAndUsesMessageCommand(t *testing.T) {
	setupRebuildTest(t, true)
	if err := os.WriteFile("new-module.nix", []byte("new module\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	preview, err := gitChangePreview()
	if err != nil || !strings.Contains(preview, "+new module") {
		t.Fatalf("untracked contents missing: %s, %v", preview, err)
	}
	diff, err := gitDiffNumstat()
	if err != nil || diff["new-module.nix"][0] != 1 {
		t.Fatalf("untracked metrics missing: %v, %v", diff, err)
	}
	yes, no := true, false
	config := Config{
		GitAdd: &yes, GitAddAll: true, GitCommit: &yes, GitPush: &yes,
		GitPushRemote: "origin", GitPushBranch: "master", GitPushForce: true,
		CommitMessageCommand: "printf 'feat: Add module'",
		AIDiagnosis:          &no,
		RebuildCommand:       "[[ $(git ls-files new-module.nix) == new-module.nix ]] && [[ $(git diff --cached --name-only) == *configuration.nix* ]]",
	}
	if err := runRebuildCommand(config); err != nil {
		t.Fatal(err)
	}
	if message := gitOutput(t, "log", "-1", "--format=%s"); message != "feat: Add module" {
		t.Fatalf("custom message not used: %q", message)
	}
	if local, remote := gitOutput(t, "rev-parse", "HEAD"), gitOutput(t, "rev-parse", "origin/master"); local != remote {
		t.Fatalf("configured push failed: %s != %s", local, remote)
	}
	if status := gitOutput(t, "status", "--porcelain"); status != "" {
		t.Fatalf("all-file commit left changes: %s", status)
	}
}

func TestRebuildCleanRepositoryAndDisabledFeatures(t *testing.T) {
	setupRebuildTest(t, false)
	no := false
	config := Config{
		SkipUnchanged: &no, RebuildBackup: &no, RecordStats: &no,
		HooksEnabled: &no, RebuildCommand: "printf ran > rebuilt",
		Hooks: Hooks{BeforeRebuild: []string{"exit 9"}},
	}
	if err := runRebuildCommand(config); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat("rebuilt"); err != nil {
		t.Fatal("clean repository was not rebuilt:", err)
	}
	if _, err := os.Stat(".vnix/backups"); !os.IsNotExist(err) {
		t.Fatal("disabled rebuild backup was created")
	}
}

func TestCustomCommitMessageFallbackAndAIToggle(t *testing.T) {
	for _, command := range []string{"exit 4", "printf ''"} {
		message := configuredCommitMessage(Config{CommitMessageCommand: command, CommitMessagePrefix: "custom"})
		if !strings.HasPrefix(message, "custom: ") {
			t.Fatalf("missing timestamp fallback: %q", message)
		}
	}
	no := false
	message := configuredCommitMessage(Config{AICommitMessage: &no, CommitMessageCommand: "printf should-not-run"})
	if !strings.HasPrefix(message, "rebuild: ") {
		t.Fatalf("AI toggle ignored: %q", message)
	}
}

func TestConfiguredHookFailurePolicy(t *testing.T) {
	if err := runConfiguredHooks(Config{}, "after_rebuild", []string{"exit 3"}); err == nil {
		t.Fatal("hook error should stop the workflow by default")
	}
	if err := runConfiguredHooks(Config{HooksContinueOnError: true}, "after_rebuild", []string{"exit 3", "exit 0"}); err != nil {
		t.Fatal(err)
	}
}

func TestFailedRebuildLeavesStagingButSkipsHooksCommitAndPush(t *testing.T) {
	setupRebuildTest(t, true)
	yes, no := true, false
	config := Config{
		GitAdd: &yes, GitAddAll: true, GitCommit: &yes, GitPush: &yes,
		AIDiagnosis: &no, RebuildCommand: "exit 7",
		Hooks: Hooks{AfterRebuild: []string{"printf unexpected > hook-ran"}},
	}
	if err := runRebuildCommand(config); err == nil {
		t.Fatal("rebuild failure was ignored")
	}
	if staged := gitOutput(t, "diff", "--cached", "--name-only"); !strings.Contains(staged, "configuration.nix") {
		t.Fatalf("pre-build staging missing: %s", staged)
	}
	if message := gitOutput(t, "log", "-1", "--format=%s"); message != "initial" {
		t.Fatal("failed rebuild was committed")
	}
	if _, err := os.Stat("hook-ran"); !os.IsNotExist(err) {
		t.Fatal("after_rebuild hook ran after failure")
	}
}
