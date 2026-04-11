# Contributing to claudette

Thanks for your interest. Here's how to get started.

## Setup

```bash
git clone https://github.com/dotcommander/claudette.git
cd claudette
go build ./...
```

Requires Go 1.26+.

## Development

```bash
go build ./...                      # build all packages
go test ./...                       # run all tests
go vet ./...                        # static analysis
```

## Pull Requests

- **Tests required.** Every PR must include tests for new behavior.
- **Conventional commits.** Use `feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `chore:` prefixes.
- **Keep the dependency list small.** We have two external deps (cobra, yaml.v3). New deps need justification.
- **No CGO.** The project must build with `CGO_ENABLED=0`.

## Architecture

- `cmd/claudette/main.go` — entry point, hooks bypass cobra for speed
- `internal/index/` — entry types, filesystem scanning, index caching
- `internal/search/` — tokenizer, scorer, aliases
- `internal/hook/` — UserPromptSubmit and PostToolResult hook protocol
- `internal/output/` — text and JSON formatters

## Reporting Issues

Open a GitHub issue with:
- What you expected
- What happened instead
- Steps to reproduce (prompt text, entry types involved)
