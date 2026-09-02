package main

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

var packageNamePattern = regexp.MustCompile(`^[A-Za-z0-9._+-]+$`)

func InstallCommand(packageNames ...string) error {
	if len(packageNames) == 0 {
		return fmt.Errorf("please provide at least one package name to install")
	}

	seen := make(map[string]struct{}, len(packageNames))
	for _, packageName := range packageNames {
		packageName = strings.TrimSpace(packageName)
		if packageName == "" {
			return fmt.Errorf("Package name validation: package name cannot be empty")
		}
		if !packageNamePattern.MatchString(packageName) {
			return fmt.Errorf("Package name validation: invalid package name %q", packageName)
		}
		if _, ok := seen[packageName]; ok {
			return fmt.Errorf("Package name validation: duplicate package name %q in one command", packageName)
		}
		seen[packageName] = struct{}{}
	}

	path, err := managedPackagesFile()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !strings.Contains(string(data), "# vnix:start") || !strings.Contains(string(data), "# vnix:end") {
		return fmt.Errorf("no vnix markers found: add '# vnix:start' and '# vnix:end' to %s", path)
	}

	content := string(data)
	startMarker := strings.Index(content, "# vnix:start")
	endMarker := strings.Index(content, "# vnix:end")
	if startMarker < 0 || endMarker < 0 || endMarker <= startMarker {
		return fmt.Errorf("invalid vnix markers in %s", path)
	}

	installBlock := content[startMarker+len("# vnix:start") : endMarker]
	added := 0
	for _, packageName := range packageNames {
		if blockContainsPackage(installBlock, packageName) {
			fmt.Printf("Package '%s' is already installed in %s\n", packageName, path)
			continue
		}

		content = strings.Replace(content, "# vnix:end", fmt.Sprintf("    %s\n    # vnix:end", packageName), 1)
		installBlock += fmt.Sprintf("\n    %s", packageName)
		fmt.Printf("✓ Added package %s\n", packageName)
		added++
	}

	if added == 0 {
		return nil
	}
	if _, err := createBackup(); err != nil {
		return err
	}

	err = os.WriteFile(path, []byte(content), 0o644)
	if err != nil {
		return err
	}

	fmt.Println("Please run 'vnix rebuild' to apply the changes.")

	return nil
}

func blockContainsPackage(block, packageName string) bool {
	for _, line := range strings.Split(block, "\n") {
		if strings.TrimSpace(line) == packageName {
			return true
		}
	}

	return false
}
