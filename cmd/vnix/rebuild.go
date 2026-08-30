package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	SecurityScanCommand string `json:"security_scan_command"`
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

var aiHTTPClient = &http.Client{Timeout: 30 * time.Second}
var openCodeBinary = "opencode"
var lastRebuildDiagnosis string

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
	lastRebuildDiagnosis = ""
	hasChanges, err := gitHasChanges()
	if err != nil {
		return err
	}
	if !hasChanges {
		fmt.Println("No changes found, rebuild skipped.")
		return nil
	}
	preview, err := gitChangePreview()
	if err != nil {
		return err
	}
	fmt.Println("Changes to be applied:")
	fmt.Print(colorizeChangePreview(preview))
	if backup, err := createBackup(); err != nil {
		return fmt.Errorf("create rebuild backup: %w", err)
	} else {
		fmt.Println("Backup created:", backup.Name)
	}

	command := "nixos-rebuild switch --flake . --quiet"
	fmt.Printf("Executing rebuild command:\n %s\n", command)
	startedAt := time.Now()
	beforeDiff, _ := gitDiffNumstat()
	gitSteps := resolveGitSteps(config)

	var rebuildOutput string
	err = runHooks("before_rebuild", config.Hooks.BeforeRebuild)
	if err == nil {
		rebuildOutput, err = runRebuildSystem()
	}
	if err == nil {
		err = runHooks("after_rebuild", config.Hooks.AfterRebuild)
	}
	if err == nil && strings.TrimSpace(config.SecurityScanCommand) != "" {
		result, scanErr := runSecurityScan(config.SecurityScanCommand)
		fmt.Printf("Running security scan:\n$ %s\n%s\n", result.Command, result.Output)
		err = scanErr
	}
	if err == nil && gitSteps.Add {
		err = runCommand("git", "add", ".")
	}
	if err == nil && gitSteps.Commit {
		err = runHooks("before_commit", config.Hooks.BeforeCommit)
	}
	if err == nil && gitSteps.Commit {
		message := aiCommitMessage(config.CommitMessagePrefix)
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
	if err != nil {
		lastRebuildDiagnosis = diagnoseRebuildFailure(err, rebuildOutput)
		if lastRebuildDiagnosis != "" {
			fmt.Println("\nOpenCode diagnosis:\n" + lastRebuildDiagnosis)
		}
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

func gitChangePreview() (string, error) {
	status, err := exec.Command("git", "status", "--short").Output()
	if err != nil {
		return "", fmt.Errorf("git status failed: %w", err)
	}
	diff, err := exec.Command("git", "diff", "--no-ext-diff", "--ignore-submodules=dirty", "HEAD", "--").Output()
	if err != nil {
		return "", fmt.Errorf("git diff failed: %w", err)
	}
	preview := "Files:\n" + strings.TrimSpace(string(status))
	if len(diff) > 0 {
		preview += "\n\nChanges:\n" + minimalDiff(string(diff))
	}
	return preview, nil
}

func minimalDiff(diff string) string {
	var lines []string
	for _, line := range strings.Split(diff, "\n") {
		if strings.HasPrefix(line, "diff --git ") {
			fields := strings.Fields(line)
			if len(fields) >= 4 {
				lines = append(lines, "\nFile: "+strings.TrimPrefix(fields[3], "b/"))
			}
			continue
		}
		if strings.HasPrefix(line, "@@") {
			lines = append(lines, line)
			continue
		}
		if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
			lines = append(lines, line)
			continue
		}
		if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
			lines = append(lines, line)
		}
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func colorizeChangePreview(preview string) string {
	const (
		reset  = "\033[0m"
		cyan   = "\033[36m"
		green  = "\033[32m"
		red    = "\033[31m"
		yellow = "\033[33m"
	)
	var lines []string
	for _, line := range strings.Split(preview, "\n") {
		color := ""
		switch {
		case strings.HasPrefix(line, "+") || strings.HasPrefix(line, "??"):
			color = green
		case strings.HasPrefix(line, "-") || strings.HasPrefix(line, " D") || strings.HasPrefix(line, "D "):
			color = red
		case strings.HasPrefix(line, " M") || strings.HasPrefix(line, "M "):
			color = yellow
		case strings.HasPrefix(line, "Files:") || strings.HasPrefix(line, "Changes:") || strings.HasPrefix(line, "File:") || strings.HasPrefix(line, "@@"):
			color = cyan
		}
		if color != "" {
			line = color + line + reset
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n") + "\n"
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

func runRebuildSystem() (string, error) {
	var output bytes.Buffer
	cmd := exec.Command("nixos-rebuild", "switch", "--flake", ".", "--quiet")
	cmd.Stdout = io.MultiWriter(os.Stdout, &output)
	cmd.Stderr = io.MultiWriter(os.Stderr, &output)
	err := cmd.Run()
	return output.String(), err
}

func diagnoseRebuildFailure(rebuildErr error, output string) string {
	if openCodeBinary == "" {
		return ""
	}
	if _, err := exec.LookPath(openCodeBinary); err != nil {
		fmt.Fprintln(os.Stderr, "OpenCode is not available; skipping rebuild diagnosis.")
		return ""
	}
	prompt := rebuildDiagnosisPrompt(rebuildErr, output)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	result, err := exec.CommandContext(ctx, openCodeBinary, "run", "--pure", "--title", "VNix rebuild failure", prompt).CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "OpenCode diagnosis failed: %v\n", err)
		return ""
	}
	return strings.TrimSpace(string(result))
}

func rebuildDiagnosisPrompt(rebuildErr error, output string) string {
	if strings.TrimSpace(output) == "" {
		output = "No command output was captured."
	}
	return fmt.Sprintf(`A VNix NixOS rebuild failed. Do not modify files, execute commands, or use tools. Explain the likely cause from the context below and propose numbered manual steps to fix it. If the context is insufficient, say exactly what the user should inspect.

Rebuild error:
%s

Captured nixos-rebuild output:
%s`, rebuildErr, truncateDiagnostic(output, 12000))
}

func truncateDiagnostic(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "\n[output truncated]"
}

func aiCommitMessage(prefix string) string {
	diff, err := exec.Command("git", "diff", "--cached", "--", ".").Output()
	if err != nil || len(diff) == 0 {
		return fallbackCommitMessage(prefix)
	}
	history, _ := exec.Command("git", "log", "-n", "10", "--format=%s").Output()
	message, err := generateCommitMessage(string(diff), string(history))
	if err != nil {
		fmt.Fprintf(os.Stderr, "AI commit message unavailable; using fallback: %v\n", err)
		return fallbackCommitMessage(prefix)
	}
	return message
}

func generateCommitMessage(diff, history string) (string, error) {
	prompt := fmt.Sprintf("Write a concise Conventional Commit message for this git diff. Output only the message, without quotes or explanation. Keep it under 50 words. Match this repository's recent commit style:\n%s\n\nDiff:\n%s", history, diff)
	key, err := geminiAPIKey()
	if err != nil {
		return "", err
	}
	if key != "" {
		message, err := askGemini(key, prompt)
		if err == nil {
			return message, nil
		}
		if model := os.Getenv("OLLAMA_MODEL"); model != "" {
			return askOllama(model, prompt)
		}
		return "", err
	}
	if model := os.Getenv("OLLAMA_MODEL"); model != "" {
		return askOllama(model, prompt)
	}
	return "", fmt.Errorf("configure Gemini with 'vnix key set-gemini' or set OLLAMA_MODEL")
}

func askGemini(key, prompt string) (string, error) {
	body, err := json.Marshal(map[string]any{"contents": []any{map[string]any{"parts": []any{map[string]string{"text": prompt}}}}})
	if err != nil {
		return "", err
	}
	url := "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash-lite:generateContent?key=" + key
	response, err := aiHTTPClient.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("Gemini returned %s", response.Status)
	}
	var result struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return "", err
	}
	if len(result.Candidates) == 0 || len(result.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("Gemini returned no message")
	}
	return cleanCommitMessage(result.Candidates[0].Content.Parts[0].Text)
}

func askOllama(model, prompt string) (string, error) {
	body, err := json.Marshal(map[string]any{"model": model, "prompt": prompt, "stream": false})
	if err != nil {
		return "", err
	}
	host := strings.TrimRight(os.Getenv("OLLAMA_HOST"), "/")
	if host == "" {
		host = "http://localhost:11434"
	}
	response, err := aiHTTPClient.Post(host+"/api/generate", "application/json", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("Ollama returned %s", response.Status)
	}
	var result struct {
		Response string `json:"response"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return "", err
	}
	return cleanCommitMessage(result.Response)
}

func cleanCommitMessage(message string) (string, error) {
	message = strings.TrimSpace(message)
	if message == "" {
		return "", fmt.Errorf("AI returned an empty message")
	}
	return message, nil
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
