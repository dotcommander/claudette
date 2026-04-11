# claudette

Lightweight CLI tool for Claude Code knowledge and skill discovery.

## Build

```bash
go build -o claudette ./cmd/claudette/
go install ./cmd/claudette/
```

## Test

```bash
go test ./...
```

## Architecture

- `internal/index/` — Entry types, frontmatter parsing, index load/save, filesystem scanning
- `internal/search/` — Tokenizer, category aliases, keyword-overlap scorer
- `internal/hook/` — UserPromptSubmit hook mode (stdin JSON -> stdout context)
- `internal/output/` — Text and JSON formatters
- `cmd/claudette/` — Entry point with hook fast-path before cobra

## Conventions

- Zero config files. Conventions over configuration.
- Index cached at `~/.config/claudette/index.json`, auto-rebuilds on staleness.
- Hook mode bypasses cobra for speed (<50ms target).
- Two external deps only: cobra, yaml.v3.
