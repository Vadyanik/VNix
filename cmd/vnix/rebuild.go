package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type Config struct {
	ManagedPackagesFile string `json:"managed_packages_file"`
	RebuildCommand      string `json:"rebuild_command"`
	NixpkgsBranch       string `json:"nixpkgs_branch"`
	GitAdd              *bool  `json:"git_add"`
	GitCommit           *bool  `json:"git_commit"`
	GitPush             *bool  `json:"git_push"`
	CommitMessagePrefix string `json:"commit_message_prefix"`
	Hooks               Hooks  `json:"hooks"`
}

type Hooks struct {
	BeforeRebuild []string `json:"before_rebuild"`
	AfterRebuild  []string `json:"after_rebuild"`
	BeforeCommit  []string `json:"before_commit"`
	AfterCommit   []string `json:"after_commit"`
	AfterPush     []string `json:"after_push"`
}

type GitSteps struct {
	Add    bool
	Commit bool
	Push   bool
}

type RebuildEntry struct {
	StartedAt        string `json:"started_at"`
	FinishedAt       string `json:"finished_at"`
	DurationMs       int64  `json:"duration_ms"`
	Success          bool   `json:"success"`
	ExitCode         *int   `json:"exit_code,omitempty"`
	Command          string `json:"command"`
	ErrorMessage     string `json:"error_message,omitempty"`
	DiffFilesChanged int    `json:"diff_files_changed"`
	DiffAddedLines   int    `json:"diff_added_lines"`
	DiffDeletedLines int    `json:"diff_deleted_lines"`
	DiffTotalLines   int    `json:"diff_total_lines"`
}

func RebuildCommand() error {
	config, err := readConfig()
	if err != nil {
		return err
	}
	return runRebuildCommand(config)
}

func readConfig() (Config, error) {
	data, err := os.ReadFile(".vnix/config.json")
	if err != nil {
		return Config{}, err
	}
	var config Config
	err = json.Unmarshal(data, &config)
	if err != nil {
		return Config{}, err
	}
	return config, nil
}

func runRebuildCommand(config Config) error {
	hasChanges, err := gitHasChanges()
	if err != nil {
		return err
	}
	if !hasChanges {
		fmt.Println("No changes found, rebuild skipped.")
		return nil
	}

	command := "nixos-rebuild switch --flake . --quiet"
	fmt.Printf("Executing rebuild command:\n %s\n", command)
	startedAt := time.Now()
	beforeDiff, _ := gitDiffNumstat()
	gitSteps := resolveGitSteps(config)

	err = runHooks("before_rebuild", config.Hooks.BeforeRebuild)
	if err == nil {
		err = runCommand("nixos-rebuild", "switch", "--flake", ".", "--quiet")
	}
	if err == nil {
		err = runHooks("after_rebuild", config.Hooks.AfterRebuild)
	}
	if err == nil && gitSteps.Add {
		err = runCommand("git", "add", ".")
	}
	if err == nil && gitSteps.Commit {
		err = runHooks("before_commit", config.Hooks.BeforeCommit)
	}
	if err == nil && gitSteps.Commit {
		message := aicCommitMessage(config.CommitMessagePrefix)
		printCommitMessage(message)
		err = runCommand("git", "commit", "-m", message)
	}
	if err == nil && gitSteps.Commit {
		err = runHooks("after_commit", config.Hooks.AfterCommit)
	}
	if err == nil && gitSteps.Push {
		err = runCommand("git", "push")
	}
	if err == nil && gitSteps.Push {
		err = runHooks("after_push", config.Hooks.AfterPush)
	}

	finishedAt := time.Now()
	afterDiff, _ := gitDiffNumstat()

	entry := RebuildEntry{
		StartedAt:        startedAt.Format(time.RFC3339),
		FinishedAt:       finishedAt.Format(time.RFC3339),
		DurationMs:       finishedAt.Sub(startedAt).Milliseconds(),
		Success:          err == nil,
		Command:          command,
		DiffFilesChanged: diffFilesChanged(beforeDiff, afterDiff),
		DiffAddedLines:   diffAddedLines(beforeDiff, afterDiff),
		DiffDeletedLines: diffDeletedLines(beforeDiff, afterDiff),
		DiffTotalLines:   diffTotalLines(beforeDiff, afterDiff),
	}
	if err != nil {
		entry.ErrorMessage = err.Error()
		if exitErr, ok := err.(*exec.ExitError); ok {
			code := exitErr.ExitCode()
			entry.ExitCode = &code
			fmt.Println("Rebuild failed! Error:", err)
			fmt.Println("Error code:", code)
		} else {
			fmt.Println("Rebuild failed! Error:", err)
		}
	} else {
		fmt.Println("Rebuild completed successfully.")
	}
	if statsErr := updateStats(entry); statsErr != nil {
		return statsErr
	}
	return err
}

func enabled(value *bool) bool {
	return value == nil || *value
}

func resolveGitSteps(config Config) GitSteps {
	steps := GitSteps{
		Add:    enabled(config.GitAdd),
		Commit: enabled(config.GitCommit),
		Push:   enabled(config.GitPush),
	}
	if !steps.Add {
		if steps.Commit {
			fmt.Fprintln(os.Stderr, "Warning: git_add is disabled, so git_commit is disabled too.")
		}
		if steps.Push {
			fmt.Fprintln(os.Stderr, "Warning: git_add is disabled, so git_push is disabled too.")
		}
		steps.Commit = false
		steps.Push = false
	}
	if !steps.Commit && steps.Push {
		fmt.Fprintln(os.Stderr, "Warning: git_commit is disabled, so git_push is disabled too.")
		steps.Push = false
	}
	return steps
}

func gitHasChanges() (bool, error) {
	output, err := exec.Command("git", "status", "--porcelain").CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("git status failed: %w\n%s", err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)) != "", nil
}

func runHooks(name string, hooks []string) error {
	for _, hook := range hooks {
		hook = strings.TrimSpace(hook)
		if hook == "" {
			continue
		}
		fmt.Printf("Running %s hook: %s\n", name, hook)
		if err := runCommand("bash", "-c", hook); err != nil {
			return err
		}
	}
	return nil
}

func runCommand(name string, args ...string) error {
	fmt.Println("$", name, strings.Join(args, " "))
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func aicCommitMessage(prefix string) string {
	if _, err := exec.LookPath("aic"); err != nil {
		fmt.Fprintln(os.Stderr, "aic is not installed or not in PATH; using fallback commit message")
		return fallbackCommitMessage(prefix)
	}

	output, err := exec.Command("aic", "-p").CombinedOutput()
	message := strings.TrimSpace(string(output))
	if err != nil || message == "" {
		fmt.Fprintf(os.Stderr, "aic failed; using fallback commit message: %v\n%s\n", err, message)
		return fallbackCommitMessage(prefix)
	}
	return message
}

func printCommitMessage(message string) {
	fmt.Println()
	fmt.Println("========================================")
	fmt.Println("Commit message:")
	fmt.Println(message)
	fmt.Println("========================================")
	fmt.Println()
}

func fallbackCommitMessage(prefix string) string {
	if strings.TrimSpace(prefix) == "" {
		prefix = "rebuild"
	}
	return fmt.Sprintf("%s: %s", prefix, time.Now().Format("2006-01-02 15:04:05"))
}

func updateStats(entry RebuildEntry) error {
	db, err := sql.Open("sqlite", ".vnix/stats.db")
	if err != nil {
		return err
	}
	defer db.Close()

	var exitCode any
	if entry.ExitCode != nil {
		exitCode = *entry.ExitCode
	}

	_, err = db.Exec(`
		INSERT INTO rebuilds (
			started_at,
			finished_at,
			duration_ms,
			success,
			exit_code,
			command,
			error_message,
			diff_files_changed,
			diff_added_lines,
			diff_deleted_lines,
			diff_total_lines
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		entry.StartedAt,
		entry.FinishedAt,
		entry.DurationMs,
		boolToInt(entry.Success),
		exitCode,
		entry.Command,
		entry.ErrorMessage,
		entry.DiffFilesChanged,
		entry.DiffAddedLines,
		entry.DiffDeletedLines,
		entry.DiffTotalLines,
	)
	return err
}

func gitDiffNumstat() (map[string][2]int, error) {
	cmd := exec.Command("git", "diff", "--numstat", "--no-ext-diff", "--ignore-submodules=dirty", "HEAD", "--")
	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	result := make(map[string][2]int)
	lines := strings.Split(strings.TrimSpace(string(output)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 3 {
			continue
		}
		added, _ := parseNumstatField(fields[0])
		deleted, _ := parseNumstatField(fields[1])
		result[fields[2]] = [2]int{added, deleted}
	}
	return result, nil
}

func parseNumstatField(value string) (int, error) {
	if value == "-" {
		return 0, nil
	}
	return strconv.Atoi(value)
}

func diffFilesChanged(before, after map[string][2]int) int {
	return len(diffKeys(before, after))
}

func diffAddedLines(before, after map[string][2]int) int {
	return diffLineSum(after, 0)
}

func diffDeletedLines(before, after map[string][2]int) int {
	return diffLineSum(after, 1)
}

func diffTotalLines(before, after map[string][2]int) int {
	return diffAddedLines(before, after) + diffDeletedLines(before, after)
}

func diffKeys(before, after map[string][2]int) map[string]struct{} {
	keys := make(map[string]struct{})
	for key := range before {
		keys[key] = struct{}{}
	}
	for key := range after {
		keys[key] = struct{}{}
	}
	return keys
}

func diffLineSum(stats map[string][2]int, index int) int {
	total := 0
	for _, value := range stats {
		total += value[index]
	}
	return total
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
