package main

import (
	"fmt"
	"os"
	"os/exec"
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

	_, err = os.Stat(".vnix/stats.db")
	if os.IsNotExist(err) {
		if err := CreateStatsDB(); err != nil {
			return err
		}
	} else {
		fmt.Println("stats.db already exists, skipping...")
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
	content := `{
  "managed_packages_file": "modules/vnix_packages.nix",
  "rebuild_command": "sudo nixos-rebuild switch --flake ."
}`
	return os.WriteFile(".vnix/config.json", []byte(content), 0644)
}

func CreateStatsDB() error {
	fmt.Println("Creating stats.db...")
	schema := `
CREATE TABLE IF NOT EXISTS rebuilds (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  started_at TEXT NOT NULL,
  finished_at TEXT NOT NULL,
  duration_ms INTEGER NOT NULL,
  success INTEGER NOT NULL,
  exit_code INTEGER,
  command TEXT NOT NULL,
  error_message TEXT,
  diff_files_changed INTEGER NOT NULL,
  diff_added_lines INTEGER NOT NULL,
  diff_deleted_lines INTEGER NOT NULL,
  diff_total_lines INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_rebuilds_started_at ON rebuilds(started_at);
CREATE INDEX IF NOT EXISTS idx_rebuilds_success ON rebuilds(success);
`
	cmd := exec.Command("sqlite3", ".vnix/stats.db")
	cmd.Stdin = strings.NewReader(schema)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
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
