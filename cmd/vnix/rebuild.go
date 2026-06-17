package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"
)

type Config struct {
	RebuildCommand string `json:"rebuild_command"`
}

type Stats struct {
	TotalRebuilds      int    `json:"total_rebuilds"`
	SuccessfulRebuilds int    `json:"successful_rebuilds"`
	FailedRebuilds     int    `json:"failed_rebuilds"`
	FirstRebuildTime   string `json:"first_rebuild_time"`
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
	cmd := exec.Command("sh", "-c", command)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			fmt.Println("Rebuild failed! Error:", err)
			fmt.Println("Error code:", exitErr.ExitCode())
		} else {
			fmt.Println("Rebuild failed! Error:", err)
		}
		if statsErr := updateStats(false); statsErr != nil {
			return statsErr
		}
	} else {
		fmt.Println("Rebuild command executed successfully.")
		if statsErr := updateStats(true); statsErr != nil {
			return statsErr
		}
	}
	return nil
}

func updateStats(success bool) error {
	data, err := os.ReadFile(".vnix/stats.json")
	if err != nil {
		return err
	}

	var stats Stats
	if err := json.Unmarshal(data, &stats); err != nil {
		return err
	}

	stats.TotalRebuilds++
	if success {
		stats.SuccessfulRebuilds++
	} else {
		stats.FailedRebuilds++
	}
	if stats.FirstRebuildTime == "" {
		stats.FirstRebuildTime = time.Now().Format(time.RFC3339)
	}

	updated, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(".vnix/stats.json", updated, 0o644)
}
