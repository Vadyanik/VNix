package main

import (
	"fmt"
	"os"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return TUICommand()
	}

	switch args[0] {
	case "init":
		return InitCommand()
	case "install":
		if len(args) < 2 {
			return fmt.Errorf("usage: vnix install <package> [package...]")
		}
		return InstallCommand(args[1:]...)
	case "stats":
		return StatsCommand()
	case "plan":
		return PlanCommand()
	case "packages":
		return PackagesCommand(args[1:])
	case "profile":
		return ProfileCommand(args[1:])
	case "generations":
		return GenerationsCommand(args[1:])
	case "drift":
		return DriftCommand()
	case "security":
		return SecurityCommand(args[1:])
	case "backups":
		return BackupsCommand(args[1:])
	case "ai-patch":
		return AIPatchCommand(args[1:])
	case "rebuild":
		return RebuildCommand()
	case "key":
		if len(args) != 2 || args[1] != "set-gemini" {
			return fmt.Errorf("usage: vnix key set-gemini")
		}
		return SetGeminiKeyCommand()
	case "tui":
		return TUICommand()
	case "search":
		return SearchCommand(args[1:])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}
