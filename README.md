# claudette

[![Go Reference](https://pkg.go.dev/badge/github.com/dotcommander/claudette.svg)](https://pkg.go.dev/github.com/dotcommander/claudette)
[![MIT License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

You've spent hours building a knowledge base of hard-won debugging insights, custom skills, and specialized agents. But Claude Code doesn't know they exist unless you remember to mention them — so you keep re-discovering the same race condition fix, re-explaining the same API quirk, losing context you already captured.

Claudette fixes that. Two commands, zero maintenance:

```bash
go install github.com/dotcommander/claudette/cmd/claudette@latest
claudette init
```

Now when you type "fix the goroutine deadlock" at 11pm, Claude automatically sees the KB entry you wrote three weeks ago — the one where you spent an hour tracing that channel bug. When your build breaks, Claude surfaces the entry from last time you hit that exact error. Your past debugging sessions become automatic context for every future conversation.

## What changes

**Before:** You write `fix goroutine race condition`. Claude starts from scratch. Your KB entry about Go race patterns sits unread in `~/.claude/kb/go/`.

**After:** Claudette scores your prompt against every KB article, skill, agent, and command you've installed. Claude sees the match, reads the entry, and applies what you already know — before writing a single line of code.

This works for errors too. A test fails with `undefined: NewRouter` — claudette detects the error signal in the tool output, finds your KB entry about chi/v5 import paths, and surfaces it. You stop re-debugging solved problems.

## How it works

Claudette runs as a [Claude Code hook](https://docs.anthropic.com/en/docs/claude-code/hooks) — invisible infrastructure that fires on every prompt:

1. **UserPromptSubmit** — scores your prompt against indexed entries and surfaces the top matches. Runs in under 50ms.
2. **PostToolResult** — watches for error signals (build failures, test errors, panics) and surfaces relevant KB entries when things break.

`claudette init` wires both hooks into `~/.claude/settings.json` and builds the index. It's idempotent — safe to re-run anytime.

## CLI

You can also search your knowledge base directly:

```bash
claudette search goroutine patterns     # search all entry types
claudette kb sqlite connection pool     # KB entries only
claudette skill refactoring             # skills only
claudette scan                          # rebuild the index
```

**Flags:** `--format json`, `--threshold 3`, `--limit 10`

## What gets indexed

Everything under `~/.claude/`:

| Directory | What's there |
|-----------|-------------|
| `kb/` | Knowledge base articles — debugging insights, API quirks, patterns |
| `skills/` | Skills — specialized capabilities and domain knowledge |
| `agents/` | Agent definitions |
| `commands/` | Slash commands |
| Plugins | Anything installed via `~/.claude/plugins/` |

The index lives at `~/.config/claudette/index.json` and auto-rebuilds when files change. No manual maintenance.

## Configuration

`claudette init` writes `~/.config/claudette/config.json`:

```json
{
  "source_dirs": [
    "/home/you/.claude/kb",
    "/home/you/.claude/skills",
    "/home/you/.claude/agents",
    "/home/you/.claude/commands"
  ]
}
```

Add extra directories to index team-wide skill repos or project-specific knowledge. Plugin directories are included automatically.

| Variable | Values | Description |
|----------|--------|-------------|
| `CLAUDETTE_OUTPUT` | `full` (default), `compact` | Controls how much detail appears in surfaced entries |

## Documentation

- [Setup & Installation](docs/setup.md)
- [Contributing](docs/CONTRIBUTING.md)
- [Changelog](docs/CHANGELOG.md)

## License

[MIT](LICENSE)
