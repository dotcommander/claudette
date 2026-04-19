# claudette

[![Go Reference](https://pkg.go.dev/badge/github.com/dotcommander/claudette.svg)](https://pkg.go.dev/github.com/dotcommander/claudette)
[![MIT License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

You've spent hours building a knowledge base of hard-won debugging insights, custom skills, and specialized agents. But Claude Code doesn't know they exist unless you remember to mention them — so you keep re-discovering the same race condition fix, re-explaining the same API quirk, losing context you already captured.

Claudette fixes that. Two commands, zero maintenance:

```bash
go install github.com/dotcommander/claudette/cmd/claudette@latest
claudette install
```

`claudette install` adds two hook entries to `~/.claude/settings.json` (`UserPromptSubmit` and `PostToolUseFailure`), writes `~/.config/claudette/config.json`, and builds the initial index. It's idempotent. Reverse it any time with `claudette uninstall`.

Now when you type "fix the goroutine deadlock" at 11pm, Claude automatically sees the KB entry you wrote three weeks ago — the one where you spent an hour tracing that channel bug. When your build breaks, Claude surfaces the entry from last time you hit that exact error. Your past debugging sessions become automatic context for every future conversation.

## What changes

**Before:** You write `fix goroutine race condition`. Claude starts from scratch. Your KB entry about Go race patterns sits unread in `~/.claude/kb/go/`.

**After:** Claudette scores your prompt against every KB article, skill, agent, and command you've installed. Claude sees the match, reads the entry, and applies what you already know — before writing a single line of code.

This works for errors too. A test fails with `undefined: NewRouter` — Claude Code fires `PostToolUseFailure`, claudette tokenizes the failing tool output, finds your KB entry about chi/v5 import paths, and surfaces it. You stop re-debugging solved problems.

## How it works

Claudette runs as a [Claude Code hook](https://docs.anthropic.com/en/docs/claude-code/hooks) — invisible infrastructure that fires on every prompt:

1. **UserPromptSubmit** — scores your prompt against indexed entries and surfaces the top matches. Runs in under 50ms.
2. **PostToolUseFailure** — fires only when a tool call fails. Tokenizes the failing output and surfaces relevant KB entries so Claude sees the fix you already wrote down.

`claudette install` wires both hooks into `~/.claude/settings.json` and builds the index. It's idempotent — safe to re-run anytime. `claudette uninstall` removes every entry it added and deletes `~/.config/claudette/`; the binary itself stays in `$GOPATH/bin` so you can remove it with `rm` when you're ready.

## CLI

Search your knowledge base directly:

```bash
claudette search goroutine patterns     # search all entry types
claudette kb sqlite connection pool     # KB entries only
claudette skill refactoring             # skills only
claudette scan                          # rebuild the index
```

**Flags:** `--format json`, `--threshold 3`, `--limit 10`

## Session Analysis

Claudette can also read Claude Code's own session transcripts — useful for reviewing what happened in a past conversation, extracting files touched, or spotting where things went sideways.

```bash
# Find session transcripts for the current project
claudette sessions

# Parse a transcript and show extracted turns
claudette turns ~/.claude/projects/<encoded-path>/<uuid>/<uuid>.jsonl
```

```
Turn 1  2026-04-18 11:42
  User:    fix the goroutine race in internal/index/cache.go
  Assist:  I'll read the file first and trace the lock usage…
  Read:    internal/index/cache.go, internal/index/entry.go
  Edited:  internal/index/cache.go
  Tools:   Bash

Turn 2  2026-04-18 11:49
  User:    the test still fails
  Assist:  The race is in the read path, not the write path…
  Read:    internal/index/cache.go
  Edited:  internal/index/cache.go
  Tools:   Bash
```

Each turn shows the user prompt, the assistant's opening response, files read, files edited, and other tools used. The `--json` flag emits structured output for scripting. Parsing runs entirely offline — no network calls, never touches the hook path.

See [Session Analysis](docs/session-analysis.md) for the full command reference.

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

`claudette install` writes `~/.config/claudette/config.json`:

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

- [Installation](docs/installation.md) — install, verify, uninstall, update
- [How It Works](docs/how-it-works.md) — hook flow, index, scoring algorithm
- [Searching](docs/searching.md) — CLI search commands and flags
- [Session Analysis](docs/session-analysis.md) — browse projects, sessions, and parsed turns
- [Configuration](docs/configuration.md) — source directories, output modes, env vars
- [Writing Entries](docs/writing-entries.md) — frontmatter, keyword weights, testing your entries
- [Troubleshooting](docs/troubleshooting.md) — diagnostics, common errors, hook debugging
- [Contributing](CONTRIBUTING.md)
- [Changelog](CHANGELOG.md)

## License

[MIT](LICENSE)
