package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("VNix - CLI manager for NixOS")
		fmt.Println("Usage: vnix <command>")
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
	case "search":
		SearchCommand()
	case "stats":
		fmt.Println("Displaying system stats...")
	case "rebuild":
		if err := RebuildCommand(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}
