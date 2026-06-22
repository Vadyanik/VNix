package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
)

type searchResult struct {
	AttrPath    string `json:"-"`
	AttrName    string `json:"-"`
	PName       string `json:"pname"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Score       int    `json:"-"`
}

func SearchCommand(args []string) error {
	branch, query, err := searchArgs(args)
	if err != nil {
		return err
	}
	if branch == "" {
		config, _ := readConfig()
		branch = config.NixpkgsBranch
	}
	if branch == "" {
		branch, err = detectNixpkgsBranch()
		if err != nil {
			return err
		}
	}
	branch = normalizeNixpkgsBranch(branch)

	results, err := nixSearch(branch, query)
	if err != nil {
		return err
	}
	if len(results) == 0 {
		return fmt.Errorf("no packages found for %q", query)
	}

	selected, err := fzfSelect(results)
	if err != nil {
		return err
	}
	if len(selected) == 0 {
		return nil
	}
	return InstallCommand(selected...)
}

func searchArgs(args []string) (string, string, error) {
	var branch string
	var queryParts []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--branch", "-b":
			if i+1 >= len(args) {
				return "", "", fmt.Errorf("Usage: vnix search [--branch <branch>] <package>")
			}
			branch = args[i+1]
			i++
		default:
			queryParts = append(queryParts, args[i])
		}
	}
	query := strings.TrimSpace(strings.Join(queryParts, " "))
	if query == "" {
		return "", "", fmt.Errorf("Usage: vnix search [--branch <branch>] <package>")
	}
	return branch, query, nil
}

func nixSearch(branch, query string) ([]searchResult, error) {
	cmd := exec.Command("nix", "search", "github:NixOS/nixpkgs/"+branch, query, "--json")
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("nix search failed: %s", strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, err
	}

	var raw map[string]searchResult
	if err := json.Unmarshal(output, &raw); err != nil {
		return nil, err
	}
	results := make([]searchResult, 0, len(raw))
	for path, result := range raw {
		result.AttrPath = trimNixAttrPath(path)
		result.AttrName = result.AttrPath
		result.Score = scoreSearchResult(query, result)
		results = append(results, result)
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			return results[i].AttrName < results[j].AttrName
		}
		return results[i].Score > results[j].Score
	})
	return results, nil
}

func trimNixAttrPath(path string) string {
	parts := strings.Split(path, ".")
	if len(parts) > 2 && (parts[0] == "legacyPackages" || parts[0] == "packages") {
		return strings.Join(parts[2:], ".")
	}
	return path
}

func scoreSearchResult(query string, result searchResult) int {
	query = strings.ToLower(query)
	attr := strings.ToLower(result.AttrName)
	pname := strings.ToLower(result.PName)
	desc := strings.ToLower(result.Description)
	score := 0

	if attr == query {
		score += 1000
	}
	if pname == query {
		score += 900
	}
	if strings.HasPrefix(attr, query) {
		score += 700
	}
	if strings.HasPrefix(pname, query) {
		score += 600
	}
	if strings.Contains(attr, query) {
		score += 300
	}
	if strings.Contains(pname, query) {
		score += 200
	}
	if strings.Contains(desc, query) {
		score += 50
	}

	for needle, penalty := range map[string]int{
		"unwrapped":          300,
		"tests.":             200,
		"python":             150,
		"gnomeExtensions.":   150,
		"vscode-extensions.": 100,
	} {
		if strings.Contains(attr, strings.ToLower(needle)) {
			score -= penalty
		}
	}
	return score
}

func fzfSelect(results []searchResult) ([]string, error) {
	var input bytes.Buffer
	for _, result := range results {
		fmt.Fprintf(&input, "%s\t%s\t%s\t%s\n", result.AttrName, result.PName, result.Version, strings.ReplaceAll(result.Description, "\n", " "))
	}

	cmd := exec.Command("fzf", "--multi", "--with-nth=1,2,3,4", "--delimiter=\t")
	cmd.Stdin = &input
	cmd.Stderr = os.Stderr
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 130 {
			return nil, nil
		}
		return nil, err
	}

	var selected []string
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line == "" {
			continue
		}
		selected = append(selected, strings.Split(line, "\t")[0])
	}
	return selected, nil
}
