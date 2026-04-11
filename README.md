# claudette

[![Go Reference](https://pkg.go.dev/badge/github.com/dotcommander/claudette.svg)](https://pkg.go.dev/github.com/dotcommander/claudette)
[![MIT License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Lightweight CLI that surfaces relevant knowledge base entries, skills, agents, and commands for [Claude Code](https://claude.ai/code).

Claudette indexes your `~/.claude/` components (KB articles, skills, agents, commands) and scores them against natural language prompts using keyword overlap with category aliasing. It runs as a standalone CLI or as a Claude Code `UserPromptSubmit` hook that automatically injects relevant context into every conversation.

## Install

```bash
go install github.com/dotcommander/claudette/cmd/claudette@latest
```

## Usage

### CLI

```bash
claudette search goroutine patterns     # search all entry types
claudette kb sqlite connection pool     # search knowledge base only
claudette skill refactoring             # search skills only
claudette scan                          # rebuild the index
```

**Flags:** `--format json`, `--threshold 3`, `--limit 10`

### Hook Mode

Add to your Claude Code settings to auto-surface relevant entries on every prompt:

```json
{
  "hooks": {
    "UserPromptSubmit": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "claudette hook"
          }
        ]
      }
    ],
    "PostToolResult": [
      {
        "matcher": "",
        "hooks": [
          {
            "type": "command",
            "command": "claudette post-tool-result"
          }
        ]
      }
    ]
  }
}
```

**UserPromptSubmit hook** — scores the user's prompt and surfaces matching entries. Bypasses the CLI framework for sub-50ms latency.

**PostToolResult hook** — detects error signals in tool output (build failures, test errors, runtime panics) and surfaces relevant KB entries to help diagnose the problem. Produces no output on successful results, so it adds zero overhead to normal operation.

Both hooks log diagnostics to stderr showing tokens, matched entries with scores, and timing.

### Environment Variables

| Variable | Values | Description |
|----------|--------|-------------|
| `CLAUDETTE_OUTPUT` | `full` (default), `compact` | Compact mode shows entry names and descriptions only; full mode shows file paths and titles |

## How It Works

1. **Scan** — Walks `~/.claude/kb/`, skills, agents, commands, and plugin directories. Extracts metadata from YAML frontmatter and markdown headings. Pre-tokenizes keywords per entry.
2. **Cache** — Stores the index at `~/.config/claudette/index.json`. Auto-rebuilds when file count or max mtime changes.
3. **Score** — Tokenizes the prompt (removing stop words, preserving internal hyphens), then scores each entry: +1 per keyword match, +2 for category alias hits (e.g., "golang" boosts "go" entries), +1 for plural normalization.
4. **Rank** — Filters by threshold, caps by limit, sorts by score descending with alphabetical tie-breaking.

## Documentation

- [Setup & Installation](docs/setup.md)
- [Contributing](docs/CONTRIBUTING.md)
- [Changelog](docs/CHANGELOG.md)

## Dependencies

Two external dependencies: [cobra](https://github.com/spf13/cobra) (CLI framework) and [yaml.v3](https://pkg.go.dev/gopkg.in/yaml.v3) (frontmatter parsing).

## License

[MIT](LICENSE)
