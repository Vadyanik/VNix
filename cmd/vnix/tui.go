package main

import (
	"database/sql"
	"fmt"
	"math/rand"
	"sort"
	"strings"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const tuiDashboard = "dashboard"

var petPhrases = map[string][]string{
	"welcome":  {"Ready for a cozy session!", "Let's make Nix happy!", "I'm here to help!"},
	"init":     {"Fresh den, fresh start!", "Let's build a cozy system!", "Ready to nest!"},
	"search":   {"Sniffing nixpkgs...", "Hunting packages!", "Found a trail!"},
	"install":  {"Tucking packages in!", "One more tool for us!", "Nice addition!"},
	"rebuild":  {"Crossing paws for rebuild!", "Time to make it real!", "Building our little world!"},
	"stats":    {"Let's check our streak!", "Numbers tell stories!", "History sniffed!"},
	"key":      {"Secrets stay tucked away.", "Safe paws, safe key.", "Locked up tight!"},
	"error":    {"Oops, tiny bump.", "We'll sort this out!", "One step at a time."},
	"fallback": {"Still keeping watch!", "Tail wagging nearby!", "You got this!"},
}

var tuiPetSpeech = petPhrases["welcome"][0]
var petReactions = []string{"<3", "^_^", "*purr*", "yay!", "=^.^="}

func TUICommand() error {
	app := tview.NewApplication()
	pages := tview.NewPages()
	petSay("welcome")
	showDashboard(app, pages, "Choose an action. Changes are made in the current directory.")
	return app.SetRoot(pages, true).EnableMouse(true).Run()
}

func showDashboard(app *tview.Application, pages *tview.Pages, message string) {
	menu := tview.NewList().ShowSecondaryText(false)
	menu.AddItem("Initialize project", "", 'i', func() { petSay("init"); showInit(app, pages) })
	menu.AddItem("Search and install packages", "", 's', func() { petSay("search"); showSearch(app, pages) })
	menu.AddItem("Add package names", "", 'a', func() { petSay("install"); showAddPackages(app, pages) })
	menu.AddItem("Review rebuild plan", "", 'p', func() { petSay("rebuild"); showPlan(app, pages) })
	menu.AddItem("Rebuild system", "", 'r', func() { petSay("rebuild"); showRebuildConfirm(app, pages) })
	menu.AddItem("Edit managed packages", "", 'e', func() { petSay("install"); showPackageEditor(app, pages) })
	menu.AddItem("Manage package profiles", "", 'f', func() { petSay("install"); showProfiles(app, pages) })
	menu.AddItem("Manage system generations", "", 'g', func() { petSay("rebuild"); showGenerations(app, pages) })
	menu.AddItem("Check configuration drift", "", 'd', func() { petSay("stats"); showDrift(app, pages) })
	menu.AddItem("Run security scan", "", 'c', func() { petSay("stats"); showSecurityScan(app, pages) })
	menu.AddItem("Restore a backup", "", 'b', func() { petSay("install"); showBackups(app, pages) })
	menu.AddItem("View rebuild timeline", "", 't', func() { petSay("stats"); showStats(app, pages) })
	menu.AddItem("Set Gemini API key", "", 'k', func() { petSay("key"); showGeminiKey(app, pages) })
	menu.AddItem("Quit", "", 'q', app.Stop)
	menu.SetBorder(true).SetTitle(" Actions ").SetBorderColor(tcell.ColorMediumPurple)

	status := tview.NewTextView().SetDynamicColors(true).SetText("[lightsteelblue]" + message)
	status.SetBorder(true).SetTitle(" Status ").SetBorderColor(tcell.ColorDarkCyan)
	header := tview.NewTextView().SetDynamicColors(true).SetText("[mediumorchid::b]VNix[-::-]  [gray]NixOS package and rebuild manager[-]\n[darkcyan]i init  s search  a add  p plan  r rebuild  e edit  f profiles  g generations\n[darkcyan]d drift  c scan  b backups  t timeline  k key  q quit")
	header.SetBorder(true).SetTitle(" VNix TUI ").SetBorderColor(tcell.ColorMediumPurple)

	root := tview.NewFlex().SetDirection(tview.FlexRow).
		AddItem(header, 6, 0, false).
		AddItem(withPet(menu), 0, 1, true).
		AddItem(status, 4, 0, false)
	pages.AddAndSwitchToPage(tuiDashboard, root, true)
}

func showInit(app *tview.Application, pages *tview.Pages) {
	form := tview.NewForm()
	form.AddInputField("Nixpkgs branch", "nixos-unstable", 30, nil, nil)
	form.AddButton("Initialize", func() {
		branch := form.GetFormItemByLabel("Nixpkgs branch").(*tview.InputField).GetText()
		if err := initCommand(strings.TrimSpace(branch)); err != nil {
			petSay("error")
			showDashboard(app, pages, "[red]Initialization failed: "+err.Error())
			return
		}
		petSay("init")
		showDashboard(app, pages, "[green]Project initialized. Import modules/vnix_packages.nix in your NixOS config.")
	})
	form.AddButton("Back", func() { showDashboard(app, pages, "Initialization cancelled.") })
	showForm(pages, " Initialize ", "Choose the nixpkgs branch for a new project.", form)
}

func showAddPackages(app *tview.Application, pages *tview.Pages) {
	form := tview.NewForm()
	form.AddInputField("Packages", "", 50, nil, nil)
	form.AddButton("Add", func() {
		packages := strings.Fields(form.GetFormItemByLabel("Packages").(*tview.InputField).GetText())
		if err := InstallCommand(packages...); err != nil {
			petSay("error")
			showDashboard(app, pages, "[red]Package installation failed: "+err.Error())
			return
		}
		petSay("install")
		showDashboard(app, pages, "[green]Packages updated. Run rebuild to apply them.")
	})
	form.AddButton("Back", func() { showDashboard(app, pages, "Package installation cancelled.") })
	showForm(pages, " Add packages ", "Enter package attributes separated by spaces.", form)
}

func showSearch(app *tview.Application, pages *tview.Pages) {
	config, _ := readConfig()
	branch := config.NixpkgsBranch
	if branch == "" {
		branch = "nixos-unstable"
	}
	form := tview.NewForm()
	form.AddInputField("Search", "", 50, nil, nil)
	form.AddInputField("Nixpkgs branch", branch, 30, nil, nil)
	form.AddButton("Search", func() {
		query := strings.TrimSpace(form.GetFormItemByLabel("Search").(*tview.InputField).GetText())
		selectedBranch := normalizeNixpkgsBranch(form.GetFormItemByLabel("Nixpkgs branch").(*tview.InputField).GetText())
		if query == "" {
			petSay("error")
			showDashboard(app, pages, "[red]Enter a package name to search.")
			return
		}
		petSay("search")
		showDashboard(app, pages, "[yellow]Searching nixpkgs for "+query+"...")
		go func() {
			results, err := nixSearch(selectedBranch, query)
			app.QueueUpdateDraw(func() {
				if err != nil {
					petSay("error")
					showDashboard(app, pages, "[red]Search failed: "+err.Error())
					return
				}
				if len(results) == 0 {
					showDashboard(app, pages, "[yellow]No packages found for "+query+".")
					return
				}
				showSearchResults(app, pages, results)
			})
		}()
	})
	form.AddButton("Back", func() { showDashboard(app, pages, "Search cancelled.") })
	showForm(pages, " Search nixpkgs ", "Search results are ranked. Use Space to select several packages.", form)
}

func showSearchResults(app *tview.Application, pages *tview.Pages, results []searchResult) {
	table := tview.NewTable().SetBorders(false).SetSelectable(true, false)
	headers := []string{"", "Attribute", "Package", "Version", "Description"}
	for column, header := range headers {
		table.SetCell(0, column, tview.NewTableCell(header).SetTextColor(tcell.ColorMediumPurple).SetSelectable(false))
	}
	selected := make(map[int]bool)
	for row, result := range results {
		table.SetCell(row+1, 0, tview.NewTableCell("[ ]"))
		table.SetCell(row+1, 1, tview.NewTableCell(result.AttrName))
		table.SetCell(row+1, 2, tview.NewTableCell(result.PName))
		table.SetCell(row+1, 3, tview.NewTableCell(result.Version))
		table.SetCell(row+1, 4, tview.NewTableCell(strings.ReplaceAll(result.Description, "\n", " ")))
	}
	table.SetBorder(true).SetTitle(" Search results ").SetBorderColor(tcell.ColorMediumPurple)
	table.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		row, _ := table.GetSelection()
		switch event.Key() {
		case tcell.KeyRune:
			if event.Rune() == ' ' && row > 0 {
				selected[row-1] = !selected[row-1]
				marker := "[ ]"
				if selected[row-1] {
					marker = "[green][x][-]"
				}
				table.GetCell(row, 0).SetText(marker)
				return nil
			}
			if event.Rune() == 'q' {
				showDashboard(app, pages, "Search cancelled.")
				return nil
			}
		case tcell.KeyEnter:
			packages := make([]string, 0, len(selected))
			for index := range selected {
				packages = append(packages, results[index].AttrName)
			}
			if len(packages) == 0 {
				showDashboard(app, pages, "[yellow]Select one or more packages with Space.")
				return nil
			}
			if err := InstallCommand(packages...); err != nil {
				petSay("error")
				showDashboard(app, pages, "[red]Package installation failed: "+err.Error())
			} else {
				petSay("install")
				showDashboard(app, pages, "[green]Packages updated. Run rebuild to apply them.")
			}
			return nil
		}
		return event
	})

	help := tview.NewTextView().SetDynamicColors(true).SetText("[gray]Space select  Enter install selected  q back")
	root := tview.NewFlex().SetDirection(tview.FlexRow).AddItem(table, 0, 1, true).AddItem(help, 1, 0, false)
	pages.AddAndSwitchToPage("search-results", withPet(root), true)
}

func showPlan(app *tview.Application, pages *tview.Pages) {
	showDashboard(app, pages, "[yellow]Running rebuild preflight checks...")
	go func() {
		plan, err := buildPlan()
		app.QueueUpdateDraw(func() {
			if err != nil {
				petSay("error")
				showDashboard(app, pages, "[red]Could not build plan: "+err.Error())
				return
			}
			showPlanResult(app, pages, plan)
		})
	}()
}

func showPlanResult(app *tview.Application, pages *tview.Pages, plan Plan) {
	content := colorizeChangePreviewForTUI(plan.Changes) + "\n\n" + formatPreflight(plan.Checks)
	view := tview.NewTextView().SetDynamicColors(true).SetScrollable(true).SetWrap(false).SetText(content)
	view.SetBorder(true).SetTitle(" Rebuild plan and preflight ").SetBorderColor(tcell.ColorMediumPurple)
	buttons := tview.NewForm()
	buttons.AddButton("Switch system", func() { showRebuildConfirm(app, pages) })
	buttons.AddButton("Back", func() { showDashboard(app, pages, "Plan review closed.") })
	buttons.SetBorder(true).SetTitle(" No system changes were made by preflight ").SetBorderColor(tcell.ColorDarkCyan)
	root := tview.NewFlex().SetDirection(tview.FlexRow).AddItem(view, 0, 1, true).AddItem(buttons, 5, 0, false)
	pages.AddAndSwitchToPage("plan", withPet(root), true)
}

func formatPreflight(checks []PreflightCheck) string {
	var lines []string
	lines = append(lines, "[darkcyan]Preflight checks[-]")
	for _, check := range checks {
		status := "[green]PASS[-]"
		if check.Err != nil {
			status = "[indianred]FAIL[-]"
		}
		lines = append(lines, status+" "+check.Name+" [gray]"+check.Result.Command+"[-]")
		if check.Err != nil && check.Result.Output != "" {
			lines = append(lines, "[gray]"+tview.Escape(truncateDiagnostic(check.Result.Output, 1000))+"[-]")
		}
	}
	return strings.Join(lines, "\n")
}

func showPackageEditor(app *tview.Application, pages *tview.Pages) {
	packages, err := readManagedPackages()
	if err != nil {
		petSay("error")
		showDashboard(app, pages, "[red]Cannot read packages: "+err.Error())
		return
	}
	form := tview.NewForm()
	form.AddInputField("Packages", strings.Join(packages, " "), 70, nil, nil)
	form.AddButton("Save", func() {
		updated := strings.Fields(form.GetFormItemByLabel("Packages").(*tview.InputField).GetText())
		if _, err := createBackup(); err != nil {
			petSay("error")
			showDashboard(app, pages, "[red]Could not create backup: "+err.Error())
			return
		}
		if err := writeManagedPackages(updated); err != nil {
			petSay("error")
			showDashboard(app, pages, "[red]Could not update packages: "+err.Error())
			return
		}
		petSay("install")
		showDashboard(app, pages, "[green]Packages updated. A backup was created.")
	})
	form.AddButton("Back", func() { showDashboard(app, pages, "Package editor closed.") })
	showForm(pages, " Package editor ", "Edit package attributes separated by spaces. Save replaces the managed list and creates a backup.", form)
}

func showProfiles(app *tview.Application, pages *tview.Pages) {
	profiles, err := loadProfiles()
	if err != nil {
		petSay("error")
		showDashboard(app, pages, "[red]Cannot load profiles: "+err.Error())
		return
	}
	names := make([]string, 0, len(profiles))
	for name := range profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	form := tview.NewForm()
	form.AddInputField("Profile name", "", 30, nil, nil)
	form.AddTextView("Saved profiles", strings.Join(names, ", "), 60, 2, false, false)
	form.AddButton("Save current", func() {
		packages, err := readManagedPackages()
		if err == nil {
			err = saveProfile(form.GetFormItemByLabel("Profile name").(*tview.InputField).GetText(), packages)
		}
		if err != nil {
			petSay("error")
			showDashboard(app, pages, "[red]Could not save profile: "+err.Error())
			return
		}
		petSay("install")
		showDashboard(app, pages, "[green]Profile saved.")
	})
	form.AddButton("Apply", func() {
		if err := applyProfile(form.GetFormItemByLabel("Profile name").(*tview.InputField).GetText()); err != nil {
			petSay("error")
			showDashboard(app, pages, "[red]Could not apply profile: "+err.Error())
			return
		}
		petSay("install")
		showDashboard(app, pages, "[green]Profile applied. A backup was created.")
	})
	form.AddButton("Back", func() { showDashboard(app, pages, "Profiles closed.") })
	showForm(pages, " Package profiles ", "Save the current package set or enter a saved name to apply it.", form)
}

func showGenerations(app *tview.Application, pages *tview.Pages) {
	generations, err := listGenerations()
	if err != nil {
		petSay("error")
		showDashboard(app, pages, "[red]Cannot list generations: "+err.Error())
		return
	}
	table := tview.NewTable().SetSelectable(true, false)
	for column, header := range []string{"Generation", "Current", "Date", "NixOS"} {
		table.SetCell(0, column, tview.NewTableCell(header).SetTextColor(tcell.ColorMediumPurple).SetSelectable(false))
	}
	for row, generation := range generations {
		current := ""
		if generation.Current {
			current = "active"
		}
		table.SetCell(row+1, 0, tview.NewTableCell(fmt.Sprintf("%d", generation.Number)))
		table.SetCell(row+1, 1, tview.NewTableCell(current))
		table.SetCell(row+1, 2, tview.NewTableCell(generation.Date))
		table.SetCell(row+1, 3, tview.NewTableCell(generation.Version))
	}
	table.SetSelectedFunc(func(row, _ int) {
		if row > 0 {
			showRollbackConfirm(app, pages, generations[row-1])
		}
	})
	table.SetDoneFunc(func(key tcell.Key) { showDashboard(app, pages, "Generations closed.") })
	table.SetBorder(true).SetTitle(" System generations: Enter to switch ").SetBorderColor(tcell.ColorMediumPurple)
	pages.AddAndSwitchToPage("generations", withPet(table), true)
}

func showRollbackConfirm(app *tview.Application, pages *tview.Pages, generation Generation) {
	modal := tview.NewModal().SetText(fmt.Sprintf("Switch to NixOS generation %d now? This requires sudo.", generation.Number)).
		AddButtons([]string{"Switch", "Cancel"}).SetDoneFunc(func(_ int, label string) {
		if label != "Switch" {
			showGenerations(app, pages)
			return
		}
		var err error
		app.Suspend(func() { err = rollbackGeneration(generation.Number) })
		if err != nil {
			petSay("error")
			showDashboard(app, pages, "[red]Rollback failed: "+err.Error())
		} else {
			petSay("rebuild")
			showDashboard(app, pages, "[green]Switched to generation "+fmt.Sprintf("%d", generation.Number)+".")
		}
	})
	pages.AddAndSwitchToPage("rollback", withPet(modal), true)
}

func showDrift(app *tview.Application, pages *tview.Pages) {
	drift, err := checkDrift()
	if err != nil {
		petSay("error")
		showDashboard(app, pages, "[red]Cannot check drift: "+err.Error())
		return
	}
	content := fmt.Sprintf("Git working tree: %t\nActive system: %s\nProfile system: %s\nActivation drift: %t", drift.GitDirty, drift.ActiveSystem, drift.ProfileSystem, drift.NeedsActivation)
	showOutput(app, pages, " Configuration drift ", content)
}

func showSecurityScan(app *tview.Application, pages *tview.Pages) {
	config, err := readConfig()
	if err != nil {
		petSay("error")
		showDashboard(app, pages, "[red]Cannot read configuration: "+err.Error())
		return
	}
	form := tview.NewForm()
	form.AddInputField("Command", config.SecurityScanCommand, 70, nil, nil)
	form.AddButton("Run scan", func() {
		command := form.GetFormItemByLabel("Command").(*tview.InputField).GetText()
		if err := saveSecurityScanCommand(command); err != nil {
			petSay("error")
			showDashboard(app, pages, "[red]Could not save scan command: "+err.Error())
			return
		}
		showDashboard(app, pages, "[yellow]Running configured security scan...")
		go func() {
			result, scanErr := runSecurityScan(command)
			app.QueueUpdateDraw(func() {
				content := result.Command + "\n\n" + result.Output
				if scanErr != nil {
					petSay("error")
					content = "Scan failed: " + scanErr.Error() + "\n\n" + content
				}
				showOutput(app, pages, " Security scan ", content)
			})
		}()
	})
	form.AddButton("Back", func() { showDashboard(app, pages, "Security scan cancelled.") })
	showForm(pages, " Security scan ", "Set a trusted scanner command. A non-zero result blocks automatic commit and push during rebuild.", form)
}

func showBackups(app *tview.Application, pages *tview.Pages) {
	backups, err := listBackups()
	if err != nil {
		petSay("error")
		showDashboard(app, pages, "[red]Cannot list backups: "+err.Error())
		return
	}
	table := tview.NewTable().SetSelectable(true, false)
	table.SetCell(0, 0, tview.NewTableCell("Backup").SetTextColor(tcell.ColorMediumPurple).SetSelectable(false))
	table.SetCell(0, 1, tview.NewTableCell("Created").SetTextColor(tcell.ColorMediumPurple).SetSelectable(false))
	for row, backup := range backups {
		table.SetCell(row+1, 0, tview.NewTableCell(backup.Name))
		table.SetCell(row+1, 1, tview.NewTableCell(backup.CreatedAt.Local().Format("2006-01-02 15:04")))
	}
	table.SetSelectedFunc(func(row, _ int) {
		if row > 0 {
			showRestoreConfirm(app, pages, backups[row-1])
		}
	})
	table.SetDoneFunc(func(key tcell.Key) { showDashboard(app, pages, "Backups closed.") })
	table.SetBorder(true).SetTitle(" Backups: Enter to restore ").SetBorderColor(tcell.ColorMediumPurple)
	pages.AddAndSwitchToPage("backups", withPet(table), true)
}

func showRestoreConfirm(app *tview.Application, pages *tview.Pages, backup Backup) {
	modal := tview.NewModal().SetText("Restore " + backup.Name + "? Current package configuration will be replaced.").
		AddButtons([]string{"Restore", "Cancel"}).SetDoneFunc(func(_ int, label string) {
		if label != "Restore" {
			showBackups(app, pages)
			return
		}
		if err := restoreBackup(backup.Name); err != nil {
			petSay("error")
			showDashboard(app, pages, "[red]Restore failed: "+err.Error())
			return
		}
		petSay("install")
		showDashboard(app, pages, "[green]Backup restored. Review the plan before rebuild.")
	})
	pages.AddAndSwitchToPage("restore", withPet(modal), true)
}

func showOutput(app *tview.Application, pages *tview.Pages, title, content string) {
	view := tview.NewTextView().SetScrollable(true).SetWrap(true).SetText(content)
	view.SetBorder(true).SetTitle(title).SetBorderColor(tcell.ColorMediumPurple)
	view.SetDoneFunc(func(key tcell.Key) { showDashboard(app, pages, "View closed.") })
	pages.AddAndSwitchToPage(strings.ToLower(strings.Trim(title, " ")), withPet(view), true)
}

func showRebuildConfirm(app *tview.Application, pages *tview.Pages) {
	preview, err := gitChangePreview()
	if err != nil {
		petSay("error")
		showDashboard(app, pages, "[red]Could not preview changes: "+err.Error())
		return
	}
	diff := tview.NewTextView().SetDynamicColors(true).SetScrollable(true).SetWrap(false).SetText(colorizeChangePreviewForTUI(preview))
	diff.SetBorder(true).SetTitle(" Changes to be applied ").SetBorderColor(tcell.ColorMediumPurple)
	buttons := tview.NewForm()
	buttons.AddButton("Rebuild", func() {
		config, err := readConfig()
		if err == nil {
			app.Suspend(func() { err = runRebuildCommand(config) })
		}
		if err != nil {
			petSay("error")
			if lastRebuildDiagnosis != "" {
				showRebuildDiagnosis(app, pages, err.Error(), lastRebuildDiagnosis)
			} else {
				showDashboard(app, pages, "[red]Rebuild failed: "+err.Error())
			}
		} else {
			petSay("rebuild")
			showDashboard(app, pages, "[green]Rebuild completed. Open statistics for its record.")
		}
	})
	buttons.AddButton("Back", func() { showDashboard(app, pages, "Rebuild cancelled.") })
	buttons.SetBorder(true).SetTitle(" Review the diff, then choose an action ").SetBorderColor(tcell.ColorDarkCyan)
	root := tview.NewFlex().SetDirection(tview.FlexRow).AddItem(diff, 0, 1, true).AddItem(buttons, 5, 0, false)
	pages.AddAndSwitchToPage("rebuild-confirm", withPet(root), true)
}

func showRebuildDiagnosis(app *tview.Application, pages *tview.Pages, rebuildErr, diagnosis string) {
	content := "Rebuild failed:\n" + rebuildErr + "\n\nOpenCode diagnosis:\n" + diagnosis
	view := tview.NewTextView().SetScrollable(true).SetWrap(true).SetText(content)
	view.SetBorder(true).SetTitle(" Rebuild diagnosis ").SetBorderColor(tcell.ColorIndianRed)
	buttons := tview.NewForm()
	buttons.AddButton("Propose patch", func() {
		showDashboard(app, pages, "[yellow]Asking OpenCode for a patch...")
		go func() {
			patch, err := proposeOpenCodePatch(rebuildErr, diagnosis)
			app.QueueUpdateDraw(func() {
				if err != nil {
					petSay("error")
					showDashboard(app, pages, "[red]Could not get patch: "+err.Error())
					return
				}
				showPatchPreview(app, pages, patch)
			})
		}()
	})
	buttons.AddButton("Back", func() { showDashboard(app, pages, "[red]Rebuild failed. Review the diagnosis before retrying.") })
	buttons.SetBorder(true).SetTitle(" OpenCode never applies changes automatically ").SetBorderColor(tcell.ColorDarkCyan)
	root := tview.NewFlex().SetDirection(tview.FlexRow).AddItem(view, 0, 1, true).AddItem(buttons, 5, 0, false)
	pages.AddAndSwitchToPage("rebuild-diagnosis", withPet(root), true)
}

func showPatchPreview(app *tview.Application, pages *tview.Pages, patch string) {
	view := tview.NewTextView().SetDynamicColors(true).SetScrollable(true).SetWrap(false).SetText(colorizeChangePreviewForTUI(patch))
	view.SetBorder(true).SetTitle(" Proposed patch ").SetBorderColor(tcell.ColorMediumPurple)
	buttons := tview.NewForm()
	buttons.AddButton("Apply patch", func() {
		if err := applyPatch(patch); err != nil {
			petSay("error")
			showDashboard(app, pages, "[red]Patch was not applied: "+err.Error())
			return
		}
		petSay("rebuild")
		showDashboard(app, pages, "[green]Patch applied. Review the rebuild plan before switching.")
	})
	buttons.AddButton("Discard", func() { showDashboard(app, pages, "Patch discarded.") })
	buttons.SetBorder(true).SetTitle(" Applying changes requires this explicit confirmation ").SetBorderColor(tcell.ColorDarkCyan)
	root := tview.NewFlex().SetDirection(tview.FlexRow).AddItem(view, 0, 1, true).AddItem(buttons, 5, 0, false)
	pages.AddAndSwitchToPage("patch-preview", withPet(root), true)
}

func showStats(app *tview.Application, pages *tview.Pages) {
	db, err := sql.Open("sqlite", ".vnix/stats.db")
	if err != nil {
		petSay("error")
		showDashboard(app, pages, "[red]Cannot open rebuild statistics: "+err.Error())
		return
	}
	defer db.Close()
	records, err := loadRebuildRecords(db)
	if err != nil {
		petSay("error")
		showDashboard(app, pages, "[red]Cannot load rebuild statistics: "+err.Error())
		return
	}
	table := tview.NewTable().SetBorders(false).SetSelectable(true, false)
	columns := []string{"Result", "Started", "Duration", "Files", "Added", "Deleted", "Exit code"}
	for column, header := range columns {
		table.SetCell(0, column, tview.NewTableCell(header).SetTextColor(tcell.ColorMediumPurple))
	}
	for index := len(records) - 1; index >= 0; index-- {
		record := records[index]
		row := len(records) - index
		result, color := "Failed", tcell.ColorIndianRed
		if record.Success {
			result, color = "Success", tcell.ColorMediumSeaGreen
		}
		exitCode := "-"
		if record.ExitCode.Valid {
			exitCode = fmt.Sprintf("%d", record.ExitCode.Int64)
		}
		table.SetCell(row, 0, tview.NewTableCell(result).SetTextColor(color))
		table.SetCell(row, 1, tview.NewTableCell(record.StartedAt.Format("2006-01-02 15:04")))
		table.SetCell(row, 2, tview.NewTableCell(fmt.Sprintf("%.1fs", float64(record.DurationMs)/1000)))
		table.SetCell(row, 3, tview.NewTableCell(fmt.Sprintf("%d", record.DiffFilesChanged)))
		table.SetCell(row, 4, tview.NewTableCell(fmt.Sprintf("%d", record.DiffAddedLines)))
		table.SetCell(row, 5, tview.NewTableCell(fmt.Sprintf("%d", record.DiffDeletedLines)))
		table.SetCell(row, 6, tview.NewTableCell(exitCode))
	}
	table.SetSelectedFunc(func(row, _ int) {
		if row == 0 {
			return
		}
		record := records[len(records)-row]
		content := fmt.Sprintf("Result: %t\nStarted: %s\nFinished: %s\nDuration: %d ms\nCommand: %s\nFiles: %d\nAdded: %d\nDeleted: %d\n", record.Success, record.StartedAt, record.FinishedAt, record.DurationMs, record.Command, record.DiffFilesChanged, record.DiffAddedLines, record.DiffDeletedLines)
		if record.ErrorMessage.Valid {
			content += "\nError:\n" + record.ErrorMessage.String
		}
		showOutput(app, pages, " Rebuild timeline detail ", content)
	})
	table.SetBorder(true).SetTitle(fmt.Sprintf(" Rebuild timeline: %d records (Enter for details) ", len(records))).SetBorderColor(tcell.ColorMediumPurple)
	table.SetDoneFunc(func(key tcell.Key) { showDashboard(app, pages, "Statistics closed.") })
	pages.AddAndSwitchToPage("stats", withPet(table), true)
}

func showGeminiKey(app *tview.Application, pages *tview.Pages) {
	form := tview.NewForm()
	input := tview.NewInputField().SetLabel("Gemini API key").SetFieldWidth(50).SetMaskCharacter('*')
	form.AddFormItem(input)
	form.AddButton("Save", func() {
		if err := saveGeminiAPIKey(input.GetText()); err != nil {
			petSay("error")
			showDashboard(app, pages, "[red]Could not save Gemini API key: "+err.Error())
			return
		}
		petSay("key")
		showDashboard(app, pages, "[green]Gemini API key saved securely.")
	})
	form.AddButton("Back", func() { showDashboard(app, pages, "Key setup cancelled.") })
	showForm(pages, " Gemini API key ", "The key is masked and stored with owner-only permissions.", form)
}

func showForm(pages *tview.Pages, title, hint string, form *tview.Form) {
	form.SetBorder(true).SetTitle(title).SetBorderColor(tcell.ColorMediumPurple)
	hintView := tview.NewTextView().SetDynamicColors(true).SetText("[gray]" + hint)
	root := tview.NewFlex().SetDirection(tview.FlexRow).AddItem(form, 0, 1, true).AddItem(hintView, 2, 0, false)
	pages.AddAndSwitchToPage(strings.ToLower(strings.Trim(title, " ")), withPet(root), true)
}

func petSay(action string) {
	phrases := petPhrases[action]
	if len(phrases) == 0 {
		phrases = petPhrases["fallback"]
	}
	tuiPetSpeech = phrases[rand.Intn(len(phrases))]
}

func withPet(content tview.Primitive) tview.Primitive {
	pet := tview.NewTextView().SetDynamicColors(true).SetText(petText(tuiPetSpeech))
	pet.SetBorder(true).SetTitle(" Companion ").SetBorderColor(tcell.ColorMediumPurple)
	pet.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		if action == tview.MouseLeftClick {
			tuiPetSpeech = petReactions[rand.Intn(len(petReactions))]
			pet.SetText(petText(tuiPetSpeech))
		}
		return action, event
	})
	return tview.NewGrid().SetRows(0, 10).SetColumns(0, 30).
		AddItem(content, 0, 0, 2, 1, 0, 0, true).
		AddItem(pet, 1, 1, 1, 1, 0, 0, false)
}

func petText(speech string) string {
	return "[mediumorchid::b]  /\\_/\\\n ( o.o )\n  > ^ <[-]\n\n[lightsteelblue]\"" + speech + "\""
}

func colorizeChangePreviewForTUI(preview string) string {
	var lines []string
	for _, line := range strings.Split(preview, "\n") {
		color := ""
		switch {
		case strings.HasPrefix(line, "+") || strings.HasPrefix(line, "??"):
			color = "green"
		case strings.HasPrefix(line, "-") || strings.HasPrefix(line, " D") || strings.HasPrefix(line, "D "):
			color = "indianred"
		case strings.HasPrefix(line, " M") || strings.HasPrefix(line, "M "):
			color = "yellow"
		case strings.HasPrefix(line, "Files:") || strings.HasPrefix(line, "Changes:") || strings.HasPrefix(line, "File:") || strings.HasPrefix(line, "@@"):
			color = "darkcyan"
		}
		line = strings.ReplaceAll(line, "[", "[[]")
		if color != "" {
			line = "[" + color + "]" + line + "[-]"
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
