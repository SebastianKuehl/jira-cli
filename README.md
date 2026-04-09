# jira-cli

Jira CLI for syncing Jira tickets into local Markdown files, grouped by sprint folders.

## Features

- Authenticate with Jira using command-line flags, environment variables, or a local `.env`
- Configure a default Jira project, board, and local base path
- Fetch all tickets, a single sprint, or a single ticket into Markdown
- Bulk-fetch ticket details for selected sprints and write shared tickets into each matching sprint folder
- Reuse cached Jira GET responses across commands through transport-level caching
- List available sprints with their Jira sprint IDs or list the tickets inside a sprint
- Print a sprint goal or full ticket details to the terminal
- Move tickets through workflow transitions
- Assign and unassign tickets with interactive user selection when needed
- Remove local config, sprint folders, or ticket files, including fragment-based sprint selection
- Print the CLI version with `jira --version`

## Prerequisites

- Go 1.26+
- A Jira base URL and API token

## Authentication

The CLI reads Jira credentials in this order:

1. Command-line flags
2. Environment variables
3. `.env` file in the current directory

Supported variables:

```sh
JIRA_BASE_URL=https://your-domain.atlassian.net
JIRA_TOKEN=your-api-token
```

## Installation

Build locally:

```sh
go build ./...
```

Install using the helper scripts:

- macOS/Linux: `./scripts/install.sh`
- Windows (PowerShell): `.\scripts\install.ps1`

Optional install target directory:

- macOS/Linux: set `JIRA_INSTALL_DIR`
- Windows: pass `-InstallDir "C:\path\to\bin"`

## Development

```sh
go mod tidy
go build ./...
go test ./...
```

## Configuration

Persistent configuration is stored at `~/.jira/config` by default.

Run:

```sh
jira config
```

The interactive flow lets you:

- select a default Jira project
- select a default board for that project
- set an optional base path for local sprint folders and ticket files

If a config already exists, the CLI prints the current values and asks whether to overwrite them.

## Usage

Show help:

```sh
jira
jira --help
jira --version
```

Check connectivity:

```sh
jira test
```

Configure defaults:

```sh
jira config
```

Fetch everything for the configured board:

```sh
jira fetch
```

`jira fetch` first resolves the target sprint IDs, then bulk-loads ticket details for the full sprint set. While it runs, it prints:

- one line listing the selected sprints being retrieved
- one line per sprint when markdown writing starts
- one line per sprint when writing is complete
- one final summary line

Fetch one sprint by exact name or sprint ID:

```sh
jira fetch "Sprint 42"
jira fetch --sprint 1234
```

Fetch one ticket into its sprint folder:

```sh
jira fetch PROJ-123
jira fetch --ticket PROJ-123
```

Restrict fetches to sprints in a specific year:

```sh
jira fetch --year 2026
jira fetch "Sprint 42" --year 2026
```

List sprints or tickets:

```sh
jira ls
jira ls "Sprint 42"
jira ls 201
jira ls "Sprint 42" --verbose
```

`jira ls` prints sprint names as `Sprint Name (ID)`, so the displayed value can be copied back into commands that accept a sprint selector.

`jira ls <sprint>` accepts:

- the full sprint name
- the Jira sprint ID
- a numeric fragment embedded in the sprint name, such as `201` for `E51(S4).DevS201`
- if a numeric fragment matches multiple sprints, the CLI presents the matches and lets you pick one interactively

When listing tickets for a sprint, the output starts with the sprint name and ID before the ticket rows.

Print a sprint goal or ticket details:

```sh
jira cat "Sprint 42"
jira cat PROJ-123
```

Move a ticket through its workflow:

```sh
jira move PROJ-123
```

Assign or unassign a ticket:

```sh
jira assign PROJ-123
jira assign PROJ-123 "Jane Doe"
jira unassign PROJ-123
```

Remove local config, a sprint folder, or a ticket file:

```sh
jira rm config
jira rm "Sprint 42"
jira rm 201
jira rm PROJ-123
jira rm --sprint "Sprint 42"
jira rm --ticket PROJ-123
```

`jira rm <target>` accepts exact sprint names, ticket IDs, and sprint-name fragments. If a fragment is ambiguous, the CLI shows matching sprint folders and prompts you to choose one.

## Markdown output

Fetched tickets are written under the configured base path using sprint-named folders.

- file name: `<ticket-id>.md`
- folder name: sanitized sprint name
- content:
  - frontmatter with Jira metadata
  - ticket body

If a ticket belongs to more than one selected sprint, the CLI writes the same markdown file into each matching sprint folder.

## Caching

Jira GET requests are cached at the transport layer so commands can reuse the same Jira responses.

- cache root: `~/.jira/cache/http/` by default
- config-aware: the cache follows `JIRA_CONFIG_PATH` / `JIRA_CONFIG_DIR`
- scope: per Jira base URL and token
- keying: hashed request URLs for stable, safe filenames
- refresh: commands that support cache refresh can bypass and repopulate cached GET responses
- invalidation: mutating Jira operations clear the scoped GET cache

## Project structure

- `cmd/jira` - CLI entrypoint
- `internal/app` - command parsing and CLI behavior
- `internal/config` - persisted config handling
- `internal/env` - `.env` loading
- `internal/jira` - Jira API client
- `scripts/` - install and uninstall helpers

## Testing

Run the full test suite:

```sh
go test ./...
```

App and config tests use isolated temporary config paths so they do not overwrite your real `~/.jira/config`.

## License

MIT. See [LICENSE](./LICENSE).
