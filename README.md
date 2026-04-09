# jira-cli

Minimal Jira CLI to fetch and sync tickets to markdown.

## Scaffolded structure

- `cmd/jira`: CLI entrypoint
- `internal/app`: command wiring with `kong`
- `internal/config`: persisted config in `~/.jira/config`
- `internal/env`: `.env` loader for local credentials
- `internal/jira`: stdlib HTTP Jira client skeleton

## Quick start

```sh
go mod tidy
go build ./...
go test ./...
```

## Install locally

- macOS/Linux: `./scripts/install.sh`
- Windows (PowerShell): `.\scripts\install.ps1`

Optional install target directory:
- macOS/Linux: set `JIRA_INSTALL_DIR`
- Windows: pass `-InstallDir "C:\path\to\bin"`

## Uninstall locally

- macOS/Linux: `./scripts/uninstall.sh`
- Windows (PowerShell): `.\scripts\uninstall.ps1`
