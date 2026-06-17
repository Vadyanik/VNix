package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func InitCommand() error {
	fmt.Println("Initializing VNIX...")

	info, err := os.Stat(".vnix")
	if err == nil && !info.IsDir() {
		return fmt.Errorf("'.vnix' exists but is not a directory")
	}
	if os.IsNotExist(err) {
		if err := os.MkdirAll(".vnix", 0o755); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	_, err = os.Stat(".vnix/config.json")
	if os.IsNotExist(err) {
		CreateConfig()
	} else {
		fmt.Println("config.json already exists, skipping...")
	}

	_, err = os.Stat(".vnix/stats.json")
	if os.IsNotExist(err) {
		CreateStats()
	} else {
		fmt.Println("stats.json already exists, skipping...")
	}

	info, err = os.Stat("modules")
	if err == nil && !info.IsDir() {
		return fmt.Errorf("'modules' exists but is not a directory")
	}
	if os.IsNotExist(err) {
		if err := os.MkdirAll("modules", 0o755); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	_, err = os.Stat("modules/vnix_packages.nix")
	if os.IsNotExist(err) {
		if err := CreateVNIXPackageFile(); err != nil {
			return err
		}
	} else {
		data, err := os.ReadFile("modules/vnix_packages.nix")
		if err != nil {
			return err
		}
		if strings.Contains(string(data), "# vnix:start") && strings.Contains(string(data), "# vnix:end") {
			fmt.Println("vnix_packages.nix already exists and contains the required markers, skipping...")
		} else {
			fmt.Println("vnix_packages.nix already exists but does not contain the required markers. Please ensure that the file contains the following lines:")
			fmt.Println("# vnix:start")
			fmt.Println("# vnix:end")
		}
	}

	return nil
}

func CreateConfig() error {
	fmt.Println("Creating config.json...")
	config := map[string]any{
		"managed_packages_file": "modules/vnix_packages.nix",
		"rebuild_command":       "sudo nixos-rebuild switch --flake .",
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	err = os.WriteFile(".vnix/config.json", data, 0644)
	if err != nil {
		return err
	}

	return nil
}

func CreateStats() error {
	fmt.Println("Creating stats.json...")
	config := map[string]any{
		"total_rebuilds":              0,
		"successful_rebuilds":         0,
		"failed_rebuilds":             0,
		"consecutive_successes":       0,
		"consecutive_failures":        0,
		"first_rebuild_time":          "",
		"last_rebuild_time":           "",
		"last_success_time":           "",
		"last_failure_time":           "",
		"last_rebuild_duration_ms":    0,
		"best_rebuild_duration_ms":    0,
		"worst_rebuild_duration_ms":   0,
		"average_rebuild_duration_ms": 0,
		"last_status":                 "",
		"last_command":                "",
		"last_error":                  "",
		"history_limit":               0,
		"rebuild_history":             []RebuildEntry{},
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	err = os.WriteFile(".vnix/stats.json", data, 0644)
	if err != nil {
		return err
	}

	return nil
}

func CreateVNIXPackageFile() error {
	fmt.Println("Creating vnix_packages.nix...")
	err := os.MkdirAll("modules", os.ModePerm)
	if err != nil {
		return err
	}

	content := `{ pkgs, ... }:

{
  environment.systemPackages = with pkgs; [
    # vnix:start
    # vnix:end
  ];
}
`

	err = os.WriteFile("modules/vnix_packages.nix", []byte(content), 0644)
	if err != nil {
		return err
	}
	InstructUser()

	return nil
}

func InstructUser() {
	fmt.Println(`modules/vnix_packages.nix installed successfully. For it to work you need to add: 

imports = [

  ./modules/vnix_packages.nix

];

to your nixos config.`)
}
