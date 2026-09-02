package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	_ "modernc.org/sqlite"
)

func InitCommand() error {
	return initCommand("")
}

func initCommand(branch string) error {
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
		if branch == "" {
			err = CreateConfig()
		} else {
			err = CreateConfigWithBranch(branch)
		}
		if err != nil {
			return err
		}
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

	managedFile, err := managedPackagesFile()
	if err != nil {
		return err
	}
	managedDir := filepath.Dir(managedFile)
	info, err = os.Stat(managedDir)
	if err == nil && !info.IsDir() {
		return fmt.Errorf("%q exists but is not a directory", managedDir)
	}
	if os.IsNotExist(err) {
		if err := os.MkdirAll(managedDir, 0o755); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	_, err = os.Stat(managedFile)
	if os.IsNotExist(err) {
		if err := CreateVNIXPackageFileAt(managedFile); err != nil {
			return err
		}
	} else {
		data, err := os.ReadFile(managedFile)
		if err != nil {
			return err
		}
		if strings.Contains(string(data), "# vnix:start") && strings.Contains(string(data), "# vnix:end") {
			fmt.Printf("%s already exists and contains the required markers, skipping...\n", managedFile)
		} else {
			fmt.Printf("%s already exists but does not contain the required markers. Please ensure that the file contains the following lines:\n", managedFile)
			fmt.Println("# vnix:start")
			fmt.Println("# vnix:end")
		}
	}

	return nil
}

func CreateConfig() error {
	fmt.Println("Creating config.json...")
	branch, err := detectNixpkgsBranch()
	if err != nil {
		return err
	}
	return CreateConfigWithBranch(branch)
}

func CreateConfigWithBranch(branch string) error {
	branch = normalizeNixpkgsBranch(branch)
	content := fmt.Sprintf(`{
  "managed_packages_file": "modules/vnix_packages.nix",
	  "rebuild_command": "nixos-rebuild switch --flake . --quiet",
  "nixpkgs_branch": %q,
	  "git_add": false,
	  "git_commit": false,
	  "git_push": false,
  "commit_message_prefix": "rebuild",
  "security_scan_command": "",
  "hooks": {
    "before_rebuild": [],
    "after_rebuild": [],
    "before_commit": [],
    "after_commit": [],
    "after_push": []
  }
}`, branch)
	return os.WriteFile(".vnix/config.json", []byte(content), 0o600)
}

func detectNixpkgsBranch() (string, error) {
	for _, path := range []string{"flake.nix", "flake.lock", "configuration.nix", "/etc/nixos/flake.nix", "/etc/nixos/flake.lock", "/etc/nixos/configuration.nix"} {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if branch := nixpkgsBranchFromText(string(data)); branch != "" {
			fmt.Printf("Detected nixpkgs branch: %s\n", branch)
			return branch, nil
		}
	}

	fmt.Print("Nixpkgs branch (example: unstable, 26.05): ")
	var branch string
	if _, err := fmt.Scanln(&branch); err != nil {
		return "", err
	}
	return normalizeNixpkgsBranch(branch), nil
}

func nixpkgsBranchFromText(text string) string {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`github:NixOS/nixpkgs/([A-Za-z0-9._-]+)`),
		regexp.MustCompile(`"owner"\s*:\s*"NixOS"[\s\S]*?"repo"\s*:\s*"nixpkgs"[\s\S]*?"ref"\s*:\s*"([A-Za-z0-9._-]+)"`),
	}
	for _, pattern := range patterns {
		match := pattern.FindStringSubmatch(text)
		if len(match) > 1 {
			return normalizeNixpkgsBranch(match[1])
		}
	}
	return ""
}

func normalizeNixpkgsBranch(branch string) string {
	branch = strings.TrimSpace(branch)
	if branch == "unstable" {
		return "nixos-unstable"
	}
	if matched, _ := regexp.MatchString(`^[0-9]{2}\.[0-9]{2}$`, branch); matched {
		return "nixos-" + branch
	}
	return branch
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
	db, err := sql.Open("sqlite", ".vnix/stats.db")
	if err != nil {
		return err
	}
	defer db.Close()

	_, err = db.Exec(schema)
	return err
}

func CreateVNIXPackageFile() error {
	return CreateVNIXPackageFileAt(managedPackagesPath)
}

func CreateVNIXPackageFileAt(path string) error {
	fmt.Printf("Creating %s...\n", path)
	err := os.MkdirAll(filepath.Dir(path), 0o755)
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

	err = os.WriteFile(path, []byte(content), 0o644)
	if err != nil {
		return err
	}
	InstructUser(path)

	return nil
}

func InstructUser(path string) {
	fmt.Printf(`%s installed successfully. For it to work you need to add:

imports = [

  ./%s

];

to your nixos config.`, path, path)
}
