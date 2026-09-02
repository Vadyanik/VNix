package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const managedPackagesPath = "modules/vnix_packages.nix"

func managedPackagesFile() (string, error) {
	config, err := readConfig()
	if os.IsNotExist(err) {
		return managedPackagesPath, nil
	}
	if err != nil {
		return "", err
	}
	path := strings.TrimSpace(config.ManagedPackagesFile)
	if path == "" {
		return managedPackagesPath, nil
	}
	if filepath.IsAbs(path) || path == "." || strings.HasPrefix(filepath.Clean(path), ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("managed_packages_file must be a path inside the project")
	}
	return path, nil
}

type CommandResult struct {
	Command string
	Output  string
}

type PreflightCheck struct {
	Name   string
	Result CommandResult
	Err    error
}

type Plan struct {
	Changes string
	Checks  []PreflightCheck
}

type Backup struct {
	Name      string
	CreatedAt time.Time
}

type Generation struct {
	Number  int
	Current bool
	Date    string
	Version string
}

type Drift struct {
	GitDirty        bool
	ActiveSystem    string
	ProfileSystem   string
	NeedsActivation bool
}

type Profiles map[string][]string

func runCombinedCommand(name string, args ...string) (CommandResult, error) {
	result := CommandResult{Command: strings.Join(append([]string{name}, args...), " ")}
	output, err := exec.Command(name, args...).CombinedOutput()
	result.Output = strings.TrimSpace(string(output))
	return result, err
}

func buildPlan() (Plan, error) {
	changes, err := gitChangePreview()
	if err != nil {
		return Plan{}, err
	}
	checks := make([]PreflightCheck, 0, 3)
	for _, check := range []struct {
		name string
		bin  string
		args []string
	}{
		{"Flake evaluation", "nix", []string{"flake", "check", "--no-build", "--no-write-lock-file"}},
		{"System dry build", "nixos-rebuild", []string{"dry-build", "--flake", "."}},
		{"Activation preview", "nixos-rebuild", []string{"dry-activate", "--flake", "."}},
	} {
		result, checkErr := runCombinedCommand(check.bin, check.args...)
		checks = append(checks, PreflightCheck{Name: check.name, Result: result, Err: checkErr})
	}
	return Plan{Changes: changes, Checks: checks}, nil
}

func runSecurityScan(command string) (CommandResult, error) {
	if strings.TrimSpace(command) == "" {
		return CommandResult{}, fmt.Errorf("set security_scan_command in .vnix/config.json")
	}
	return runCombinedCommand("bash", "-c", command)
}

func saveSecurityScanCommand(command string) error {
	data, err := os.ReadFile(".vnix/config.json")
	if err != nil {
		return err
	}
	var config map[string]json.RawMessage
	if err := json.Unmarshal(data, &config); err != nil {
		return err
	}
	value, err := json.Marshal(strings.TrimSpace(command))
	if err != nil {
		return err
	}
	config["security_scan_command"] = value
	data, err = json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(".vnix/config.json", append(data, '\n'), 0o600)
}

func readManagedPackages() ([]string, error) {
	path, err := managedPackagesFile()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	_, block, _, err := managedPackageSections(string(data), path)
	if err != nil {
		return nil, err
	}
	var packages []string
	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") {
			packages = append(packages, line)
		}
	}
	return packages, nil
}

func writeManagedPackages(packages []string) error {
	for _, pkg := range packages {
		if !packageNamePattern.MatchString(pkg) {
			return fmt.Errorf("invalid package name %q", pkg)
		}
	}
	path, err := managedPackagesFile()
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	prefix, _, suffix, err := managedPackageSections(string(data), path)
	if err != nil {
		return err
	}
	indent := "    "
	if firstLine := strings.Split(suffix, "\n")[0]; strings.TrimSpace(firstLine) != "" {
		indent = firstLine[:len(firstLine)-len(strings.TrimLeft(firstLine, " \t"))]
	}
	var lines []string
	for _, pkg := range packages {
		lines = append(lines, indent+pkg)
	}
	content := prefix + "\n" + strings.Join(lines, "\n") + "\n" + suffix
	return os.WriteFile(path, []byte(content), 0o644)
}

func managedPackageSections(content, path string) (string, string, string, error) {
	start := strings.Index(content, "# vnix:start")
	end := strings.Index(content, "# vnix:end")
	if start < 0 || end < 0 || end <= start {
		return "", "", "", fmt.Errorf("invalid vnix markers in %s", path)
	}
	start += len("# vnix:start")
	lineStart := strings.LastIndex(content[:end], "\n") + 1
	return content[:start], content[start:lineStart], content[lineStart:], nil
}

func createBackup() (Backup, error) {
	managedFile, err := managedPackagesFile()
	if err != nil {
		return Backup{}, err
	}
	if err := os.MkdirAll(".vnix/backups", 0o700); err != nil {
		return Backup{}, err
	}
	createdAt := time.Now().UTC()
	name := createdAt.Format("20060102T150405.000000000Z") + ".tar.gz"
	path := filepath.Join(".vnix/backups", name)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return Backup{}, err
	}
	defer file.Close()
	gzipWriter := gzip.NewWriter(file)
	tarWriter := tar.NewWriter(gzipWriter)
	for _, source := range []string{managedFile, ".vnix/config.json"} {
		data, readErr := os.ReadFile(source)
		if os.IsNotExist(readErr) {
			continue
		}
		if readErr != nil {
			return Backup{}, readErr
		}
		if err := tarWriter.WriteHeader(&tar.Header{Name: source, Mode: 0o600, Size: int64(len(data)), ModTime: createdAt}); err != nil {
			return Backup{}, err
		}
		if _, err := tarWriter.Write(data); err != nil {
			return Backup{}, err
		}
	}
	if err := tarWriter.Close(); err != nil {
		return Backup{}, err
	}
	if err := gzipWriter.Close(); err != nil {
		return Backup{}, err
	}
	return Backup{Name: name, CreatedAt: createdAt}, nil
}

func listBackups() ([]Backup, error) {
	entries, err := os.ReadDir(".vnix/backups")
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	backups := make([]Backup, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".tar.gz") {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return nil, infoErr
		}
		backups = append(backups, Backup{Name: entry.Name(), CreatedAt: info.ModTime()})
	}
	sort.Slice(backups, func(i, j int) bool { return backups[i].CreatedAt.After(backups[j].CreatedAt) })
	return backups, nil
}

func restoreBackup(name string) error {
	if filepath.Base(name) != name || !strings.HasSuffix(name, ".tar.gz") {
		return fmt.Errorf("invalid backup name")
	}
	file, err := os.Open(filepath.Join(".vnix/backups", name))
	if err != nil {
		return err
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzipReader.Close()
	tarReader := tar.NewReader(gzipReader)
	managedFile, err := managedPackagesFile()
	if err != nil {
		return err
	}
	allowed := map[string]bool{managedFile: true, ".vnix/config.json": true}
	entries := make(map[string][]byte)
	for {
		header, nextErr := tarReader.Next()
		if nextErr == io.EOF {
			break
		}
		if nextErr != nil {
			return nextErr
		}
		if !allowed[header.Name] || header.Typeflag != tar.TypeReg {
			return fmt.Errorf("invalid backup entry %q", header.Name)
		}
		if _, exists := entries[header.Name]; exists {
			return fmt.Errorf("duplicate backup entry %q", header.Name)
		}
		data, readErr := io.ReadAll(io.LimitReader(tarReader, header.Size+1))
		if readErr != nil || int64(len(data)) != header.Size {
			return fmt.Errorf("cannot read backup entry %q", header.Name)
		}
		entries[header.Name] = data
	}
	if len(entries) == 0 {
		return fmt.Errorf("backup contains no restorable files")
	}
	if _, err := createBackup(); err != nil {
		return fmt.Errorf("create safety backup: %w", err)
	}
	for path, data := range entries {
		if err := writeFileAtomically(path, data, 0o600); err != nil {
			return fmt.Errorf("restore backup: %w", err)
		}
	}
	return nil
}

func writeFileAtomically(path string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".vnix-restore-*")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	if err := file.Chmod(mode); err != nil {
		file.Close()
		return err
	}
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

func loadProfiles() (Profiles, error) {
	data, err := os.ReadFile(".vnix/profiles.json")
	if os.IsNotExist(err) {
		return Profiles{}, nil
	}
	if err != nil {
		return nil, err
	}
	profiles := Profiles{}
	if err := json.Unmarshal(data, &profiles); err != nil {
		return nil, err
	}
	return profiles, nil
}

func saveProfile(name string, packages []string) error {
	name = strings.TrimSpace(name)
	if name == "" || !packageNamePattern.MatchString(name) {
		return fmt.Errorf("invalid profile name %q", name)
	}
	profiles, err := loadProfiles()
	if err != nil {
		return err
	}
	profiles[name] = packages
	data, err := json.MarshalIndent(profiles, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(".vnix", 0o700); err != nil {
		return err
	}
	return os.WriteFile(".vnix/profiles.json", append(data, '\n'), 0o600)
}

func applyProfile(name string) error {
	profiles, err := loadProfiles()
	if err != nil {
		return err
	}
	packages, ok := profiles[name]
	if !ok {
		return fmt.Errorf("profile %q does not exist", name)
	}
	if _, err := createBackup(); err != nil {
		return err
	}
	return writeManagedPackages(packages)
}

func listGenerations() ([]Generation, error) {
	result, err := runCombinedCommand("nixos-rebuild", "list-generations", "--json")
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, result.Output)
	}
	return parseGenerations(result.Output)
}

func parseGenerations(output string) ([]Generation, error) {
	var raw []struct {
		Generation   int    `json:"generation"`
		Date         string `json:"date"`
		NixOSVersion string `json:"nixosVersion"`
		Current      bool   `json:"current"`
	}
	if err := json.Unmarshal([]byte(output), &raw); err != nil {
		return nil, err
	}
	generations := make([]Generation, len(raw))
	for i, item := range raw {
		generations[i] = Generation{Number: item.Generation, Date: item.Date, Version: item.NixOSVersion, Current: item.Current}
	}
	return generations, nil
}

func rollbackGeneration(number int) error {
	if number < 1 {
		return fmt.Errorf("invalid generation")
	}
	path := fmt.Sprintf("/nix/var/nix/profiles/system-%d-link/bin/switch-to-configuration", number)
	if _, err := os.Stat(path); err != nil {
		return err
	}
	return runCommand("sudo", path, "switch")
}

func checkDrift() (Drift, error) {
	dirty, err := gitHasChanges()
	if err != nil {
		return Drift{}, err
	}
	active, err := filepath.EvalSymlinks("/run/current-system")
	if err != nil {
		return Drift{}, err
	}
	profile, err := filepath.EvalSymlinks("/nix/var/nix/profiles/system")
	if err != nil {
		return Drift{}, err
	}
	return Drift{GitDirty: dirty, ActiveSystem: active, ProfileSystem: profile, NeedsActivation: active != profile}, nil
}

func loadTimeline() ([]RebuildRecord, error) {
	db, err := sql.Open("sqlite", ".vnix/stats.db")
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return loadRebuildRecords(db)
}

func parseGenerationNumber(path string) int {
	base := filepath.Base(path)
	base = strings.TrimSuffix(strings.TrimPrefix(base, "system-"), "-link")
	number, _ := strconv.Atoi(base)
	return number
}

func proposeOpenCodePatch(rebuildErr, diagnosis string) (string, error) {
	if openCodeBinary == "" {
		return "", fmt.Errorf("OpenCode is disabled")
	}
	if _, err := exec.LookPath(openCodeBinary); err != nil {
		return "", fmt.Errorf("OpenCode is not available")
	}
	prompt := fmt.Sprintf(`A VNix NixOS rebuild failed. Do not modify files, execute commands, or use tools. Propose the smallest safe fix as a unified Git diff only. Do not include Markdown fences, explanation, or commands. If a safe patch cannot be determined, output exactly: NO_PATCH.

Rebuild error:
%s

Diagnosis:
%s`, rebuildErr, truncateDiagnostic(diagnosis, 8000))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	output, err := exec.CommandContext(ctx, openCodeBinary, "run", "--pure", "--title", "VNix proposed fix", prompt).CombinedOutput()
	if err != nil {
		return "", err
	}
	return extractPatch(string(output))
}

func extractPatch(output string) (string, error) {
	output = strings.TrimSpace(strings.Trim(output, "`"))
	if output == "NO_PATCH" {
		return "", fmt.Errorf("OpenCode could not propose a safe patch")
	}
	start := strings.Index(output, "diff --git ")
	if start < 0 {
		return "", fmt.Errorf("OpenCode did not return a unified Git diff")
	}
	return strings.TrimSpace(output[start:]) + "\n", nil
}

func applyPatch(patch string) error {
	file, err := os.CreateTemp("", "vnix-patch-*.diff")
	if err != nil {
		return err
	}
	path := file.Name()
	defer os.Remove(path)
	if _, err := file.WriteString(patch); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if _, err := runCombinedCommand("git", "apply", "--check", path); err != nil {
		return fmt.Errorf("patch validation failed: %w", err)
	}
	_, err = runCombinedCommand("git", "apply", path)
	return err
}
