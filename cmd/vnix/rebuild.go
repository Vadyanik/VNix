package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	RebuildCommand string `json:"rebuild_command"`
}

type Stats struct {
	TotalRebuilds            int            `json:"total_rebuilds"`
	SuccessfulRebuilds       int            `json:"successful_rebuilds"`
	FailedRebuilds           int            `json:"failed_rebuilds"`
	ConsecutiveSuccesses     int            `json:"consecutive_successes"`
	ConsecutiveFailures      int            `json:"consecutive_failures"`
	FirstRebuildTime         string         `json:"first_rebuild_time"`
	LastRebuildTime          string         `json:"last_rebuild_time"`
	LastSuccessTime          string         `json:"last_success_time"`
	LastFailureTime          string         `json:"last_failure_time"`
	LastRebuildDurationMs    int64          `json:"last_rebuild_duration_ms"`
	BestRebuildDurationMs    int64          `json:"best_rebuild_duration_ms"`
	WorstRebuildDurationMs   int64          `json:"worst_rebuild_duration_ms"`
	AverageRebuildDurationMs int64          `json:"average_rebuild_duration_ms"`
	LastExitCode             *int           `json:"last_exit_code,omitempty"`
	LastStatus               string         `json:"last_status"`
	LastCommand              string         `json:"last_command"`
	LastError                string         `json:"last_error"`
	RebuildHistory           []RebuildEntry `json:"rebuild_history"`
	HistoryLimit             int            `json:"history_limit"`
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
	return runRebuildCommand(config.RebuildCommand)
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

func runRebuildCommand(command string) error {
	fmt.Printf("Executing rebuild command:\n %s\n", command)
	startedAt := time.Now()
	beforeDiff, _ := gitDiffNumstat()
	cmd := exec.Command("sh", "-c", command)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
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
		fmt.Println("Rebuild command executed successfully.")
	}
	if statsErr := updateStats(entry); statsErr != nil {
		return statsErr
	}
	return nil
}

func updateStats(entry RebuildEntry) error {
	data, err := os.ReadFile(".vnix/stats.json")
	if err != nil {
		return err
	}

	var stats Stats
	if err := json.Unmarshal(data, &stats); err != nil {
		return err
	}

	if stats.HistoryLimit < 0 {
		stats.HistoryLimit = 0
	}
	if stats.TotalRebuilds == 0 || stats.BestRebuildDurationMs == 0 || entry.DurationMs < stats.BestRebuildDurationMs {
		stats.BestRebuildDurationMs = entry.DurationMs
	}
	if entry.DurationMs > stats.WorstRebuildDurationMs {
		stats.WorstRebuildDurationMs = entry.DurationMs
	}
	stats.TotalRebuilds++
	stats.LastRebuildTime = entry.FinishedAt
	stats.LastRebuildDurationMs = entry.DurationMs
	stats.LastCommand = entry.Command
	stats.LastStatus = statusFor(entry.Success)
	stats.LastError = entry.ErrorMessage
	stats.LastExitCode = entry.ExitCode
	if stats.FirstRebuildTime == "" {
		stats.FirstRebuildTime = entry.StartedAt
	}
	if entry.Success {
		stats.SuccessfulRebuilds++
		stats.ConsecutiveSuccesses++
		stats.ConsecutiveFailures = 0
		stats.LastSuccessTime = entry.FinishedAt
	} else {
		stats.FailedRebuilds++
		stats.ConsecutiveFailures++
		stats.ConsecutiveSuccesses = 0
		stats.LastFailureTime = entry.FinishedAt
	}
	if stats.TotalRebuilds == 1 {
		stats.AverageRebuildDurationMs = entry.DurationMs
	} else {
		stats.AverageRebuildDurationMs = ((stats.AverageRebuildDurationMs * int64(stats.TotalRebuilds-1)) + entry.DurationMs) / int64(stats.TotalRebuilds)
	}
	stats.RebuildHistory = append([]RebuildEntry{entry}, stats.RebuildHistory...)
	if stats.HistoryLimit > 0 && len(stats.RebuildHistory) > stats.HistoryLimit {
		stats.RebuildHistory = stats.RebuildHistory[:stats.HistoryLimit]
	}

	updated, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(".vnix/stats.json", updated, 0o644)
}

func statusFor(success bool) string {
	if success {
		return "success"
	}
	return "failure"
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
