# VNix

CLI manager for managing packages in NixOS via marker blocks with `nixos-rebuild` history tracking.

## Commands

| Command | Description |
|---------|----------|
| `vnix init` | Creates project structure: `.vnix/` (config + SQLite), `modules/vnix_packages.nix` with marker blocks |
| `vnix search [--branch <branch>] <pkg>` | Searches for a package via `nix search`, ranks results and pipes through `fzf` |
| `vnix install <pkg...>` | Validates packages and adds them to the `# vnix:start` / `# vnix:end` marker block |
| `vnix rebuild` | Runs `nixos-rebuild`, captures `git diff` before/after, saves result to SQLite |
| `vnix stats` | Shows rebuild analytics: success rate, duration, file changes |

## Project Structure

```
VNix/
├── cmd/vnix/
│   ├── main.go         # CLI dispatcher
│   ├── init.go         # Initialization
│   ├── install.go      # Package installation
│   ├── rebuild.go      # Rebuild + SQLite
│   ├── stats.go        # Statistics
├── .github/workflows/  # CI (build + test)
├── go.mod / go.sum
└── LICENSE
```

## Usage

```bash
vnix init
vnix search firefox
vnix install htop ripgrep
vnix rebuild
vnix stats
```

## Technologies

- **Go 1.25** — pure Go, no CGO, no external CLI frameworks
- **SQLite** (`modernc.org/sqlite`) — rebuild history storage
- **Marker block** — inserting packages into Nix files via marker comments
- **Git diff** — tracking file changes during rebuild

## CI

GitHub Actions: build and test on every push/PR to `main`.

## License

MIT
