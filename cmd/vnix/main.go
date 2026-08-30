package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		if err := TUICommand(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}

	switch os.Args[1] {
	case "init":
		if err := InitCommand(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "install":
		if len(os.Args) >= 3 {
			if err := InstallCommand(os.Args[2:]...); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		} else {
			fmt.Println("Usage: vnix install <package> [package...]")
		}
	case "stats":
		if err := StatsCommand(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "rebuild":
		if err := RebuildCommand(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "key":
		if len(os.Args) == 3 && os.Args[2] == "set-gemini" {
			if err := SetGeminiKeyCommand(); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		} else {
			fmt.Println("Usage: vnix key set-gemini")
		}
	case "tui":
		if err := TUICommand(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "search":
		if err := SearchCommand(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}
