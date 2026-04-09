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
