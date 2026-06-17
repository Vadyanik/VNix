package main

import("fmt"; "encoding/json"; "os")

func InitCommand() {
	fmt.Println("Initializing VNIX...")
	os.MkdirAll(".vnix", os.ModePerm)

	_, err := os.Stat(".vnix/config.json")
	if os.IsNotExist(err) {
		CreateConfig()
	} else { fmt.Println("config.json already exists, skipping...") }

	_, err = os.Stat(".vnix/stats.json")
	if os.IsNotExist(err) {
		CreateStats()
	} else { fmt.Println("stats.json already exists, skipping...") }

	_, err = os.Stat("modules/vnix_packages.nix")
	if os.IsNotExist(err) {
		CreateVNIXPackageFile()
	} else { fmt.Println("vnix_packages.nix already exists, skipping...") }
}

func CreateConfig() {
	fmt.Println("Creating config.json...")
	config := map[string]any{
		"managed_packages_file": "modules/vnix_packages.nix",
		"rebuild_command": "sudo nixos-rebuild switch --flake .",
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {	panic(err)}

	err = os.WriteFile(".vnix/config.json", data, 0644)
	if err != nil {	panic(err)}
}

func CreateStats() {
	fmt.Println("Creating stats.json...")
	config := map[string]any{
		"total_rebuilds": 0,
		"successful_rebuilds": 0,
		"failed_rebuilds": 0,
		"first_rebuild_time": "",
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {	panic(err)}

	err = os.WriteFile(".vnix/stats.json", data, 0644)
	if err != nil {	panic(err)}
}

func CreateVNIXPackageFile() {
	fmt.Println("Creating vnix_packages.nix...")
	err := os.MkdirAll("modules", os.ModePerm)
	if err != nil {	panic(err)}

	content := `{ pkgs, ... }:

{
  environment.systemPackages = with pkgs; [
    # vnix:start
    # vnix:end
  ];
}
`

	err = os.WriteFile("modules/vnix_packages.nix", []byte(content), 0644)
	if err != nil {	panic(err)}
}
