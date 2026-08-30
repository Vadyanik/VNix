package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/term"
)

func SetGeminiKeyCommand() error {
	fmt.Print("Gemini API key: ")
	key, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	if err != nil {
		return err
	}
	if err := saveGeminiAPIKey(string(key)); err != nil {
		return err
	}
	fmt.Println("Gemini API key saved.")
	return nil
}

func geminiAPIKey() (string, error) {
	path, err := geminiAPIKeyPath()
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func saveGeminiAPIKey(key string) error {
	key = strings.TrimSpace(key)
	if key == "" {
		return fmt.Errorf("Gemini API key cannot be empty")
	}
	path, err := geminiAPIKeyPath()
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(dir, ".gemini-api-key-")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if _, err := temporary.WriteString(key + "\n"); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryName, path)
}

func geminiAPIKeyPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "vnix", "gemini-api-key"), nil
}
