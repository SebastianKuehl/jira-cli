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

## Configure command behavior

`jira configure`:
- validates Jira credentials first (`JIRA_BASE_URL`, `JIRA_TOKEN`)
- fetches projects and prompts for default project selection
- prompts for project base path (leave empty to use current directory)

`jira fetch`:
- fetches all sprint tickets into sprint-named folders under the configured base path
- accepts a sprint name, sprint ID, or embedded numeric sprint fragment
- when a numeric fragment matches multiple sprints, prints the candidates and prompts for a sprint number
- accepts `--ticket <id>` to fetch a single ticket into its sprint folder

`jira rm`:
- accepts `config`, a sprint name, or a ticket ID as the positional target
- supports `--config`, `--sprint`, and `--ticket` to disambiguate scripted/non-interactive removals
