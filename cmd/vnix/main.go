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
		InitCommand()
	case "search":
		SearchCommand()
	case "stats":
		fmt.Println("Displaying system stats...")
	case "rebuild":
		fmt.Println("Rebuilding system configuration...")
	}
}
