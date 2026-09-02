package main

import (
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
)

func PlanCommand() error {
	plan, err := buildPlan()
	if err != nil {
		return err
	}
	fmt.Println("Changes to be applied:")
	fmt.Print(colorizeChangePreview(plan.Changes))
	fmt.Println("Preflight checks:")
	for _, check := range plan.Checks {
		status := "PASS"
		if check.Err != nil {
			status = "FAIL"
		}
		fmt.Printf("%s  %s\n", status, check.Name)
		if check.Err != nil && check.Result.Output != "" {
			fmt.Println(check.Result.Output)
		}
	}
	return nil
}

func PackagesCommand(args []string) error {
	if len(args) == 0 || args[0] == "list" {
		packages, err := readManagedPackages()
		if err != nil {
			return err
		}
		for _, pkg := range packages {
			fmt.Println(pkg)
		}
		return nil
	}
	if args[0] != "set" || len(args) < 2 {
		return fmt.Errorf("usage: vnix packages [list|set <package> [package...]]")
	}
	if _, err := createBackup(); err != nil {
		return err
	}
	return writeManagedPackages(args[1:])
}

func ProfileCommand(args []string) error {
	if len(args) == 0 || args[0] == "list" {
		profiles, err := loadProfiles()
		if err != nil {
			return err
		}
		names := make([]string, 0, len(profiles))
		for name := range profiles {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			packages := profiles[name]
			fmt.Printf("%s: %s\n", name, strings.Join(packages, " "))
		}
		return nil
	}
	if len(args) != 2 {
		return fmt.Errorf("usage: vnix profile [list|save <name>|apply <name>]")
	}
	switch args[0] {
	case "save":
		packages, err := readManagedPackages()
		if err != nil {
			return err
		}
		return saveProfile(args[1], packages)
	case "apply":
		return applyProfile(args[1])
	default:
		return fmt.Errorf("usage: vnix profile [list|save <name>|apply <name>]")
	}
}

func GenerationsCommand(args []string) error {
	if len(args) == 0 || args[0] == "list" {
		generations, err := listGenerations()
		if err != nil {
			return err
		}
		for _, generation := range generations {
			current := ""
			if generation.Current {
				current = " current"
			}
			fmt.Printf("%d  %s  %s%s\n", generation.Number, generation.Date, generation.Version, current)
		}
		return nil
	}
	if len(args) != 2 || args[0] != "switch" {
		return fmt.Errorf("usage: vnix generations [list|switch <number>]")
	}
	number, err := strconv.Atoi(args[1])
	if err != nil {
		return fmt.Errorf("invalid generation number %q", args[1])
	}
	return rollbackGeneration(number)
}

func DriftCommand() error {
	drift, err := checkDrift()
	if err != nil {
		return err
	}
	fmt.Printf("Git working tree: %t\nActive system: %s\nProfile system: %s\nActivation drift: %t\n", drift.GitDirty, drift.ActiveSystem, drift.ProfileSystem, drift.NeedsActivation)
	return nil
}

func SecurityCommand(args []string) error {
	if len(args) > 0 && args[0] == "set" {
		if len(args) < 2 {
			return fmt.Errorf("usage: vnix security set <command>")
		}
		return saveSecurityScanCommand(strings.Join(args[1:], " "))
	}
	if len(args) > 1 || (len(args) == 1 && args[0] != "run") {
		return fmt.Errorf("usage: vnix security [run|set <command>]")
	}
	config, err := readConfig()
	if err != nil {
		return err
	}
	result, err := runSecurityScan(config.SecurityScanCommand)
	fmt.Println(result.Output)
	return err
}

func BackupsCommand(args []string) error {
	if len(args) == 0 || args[0] == "list" {
		backups, err := listBackups()
		if err != nil {
			return err
		}
		for _, backup := range backups {
			fmt.Printf("%s  %s\n", backup.Name, backup.CreatedAt.Local().Format("2006-01-02 15:04"))
		}
		return nil
	}
	if len(args) != 2 || args[0] != "restore" {
		return fmt.Errorf("usage: vnix backups [list|restore <name>]")
	}
	return restoreBackup(args[1])
}

func AIPatchCommand(args []string) error {
	if len(args) >= 2 && args[0] == "apply" {
		patch, err := os.ReadFile(args[1])
		if err != nil {
			return err
		}
		return applyPatch(string(patch))
	}
	if len(args) < 2 || args[0] != "propose" {
		return fmt.Errorf("usage: vnix ai-patch [propose <rebuild error>|apply <patch-file>]")
	}
	patch, err := proposeOpenCodePatch(strings.Join(args[1:], " "), "No previous diagnosis was supplied.")
	if err != nil {
		return err
	}
	path, err := os.CreateTemp("", "vnix-proposed-*.diff")
	if err != nil {
		return err
	}
	defer path.Close()
	if _, err := path.WriteString(patch); err != nil {
		return err
	}
	fmt.Println(path.Name())
	return nil
}
