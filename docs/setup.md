# Setup & Installation

## Prerequisites

- Go 1.26+ (`go version`)
- Claude Code with hooks support (`~/.claude/settings.json`)

## Install & Initialize

```bash
go install github.com/dotcommander/claudette/cmd/claudette@latest
claudette init
```

`claudette init` does three things:
1. Wires `UserPromptSubmit` and `PostToolResult` hooks into `~/.claude/settings.json`
2. Writes default config to `~/.config/claudette/config.json`
3. Builds the initial index from `~/.claude/` components

It's idempotent — safe to re-run. If hooks are already wired, it skips them and rebuilds the index.

## Add the CLAUDE.md Directive

Add this near the top of `~/.claude/CLAUDE.md` so Claude reads surfaced entries:

```markdown
# Knowledge Base

`~/.claude/kb/` contains verified technical knowledge extracted from prior sessions — gotchas, race conditions, API quirks, and patterns that cost real debugging time to discover. The `claudette` hook automatically surfaces relevant entries on each prompt. **When entries are surfaced, read them before proceeding** — they are higher-tier knowledge than what you'll derive from first principles.
```

## Verify

Test the hook end-to-end:

```bash
# Should return JSON on stdout, diagnostics on stderr
echo '{"prompt":"fix goroutine race condition"}' | claudette hook
# stderr: claudette: [goroutine race condition] -> entry1(4), entry2(3) (12ms)

# Should log skip reason on stderr (no stdout)
echo '{"prompt":"update the README"}' | claudette hook
# stderr: claudette: [update readme] -> no matches (8ms)

# Should skip slash commands
echo '{"prompt":"/commit"}' | claudette hook
# stderr: claudette: skip: slash command (0ms)
```

Benchmark:

```bash
time echo '{"prompt":"go cobra openai hook refactoring"}' | claudette hook
# Target: <50ms
```

## Configuration

Config file: `~/.config/claudette/config.json`

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

Add extra directories to index (e.g., team-wide skill repos, project-specific knowledge). Plugin directories from `~/.claude/plugins/` are included automatically.

## CLI Usage

```bash
claudette search "goroutine race"     # Search all entry types
claudette kb "bounded writer"          # KB entries only
claudette skill "refactor smells"      # Skills only
claudette scan                         # Rebuild index

# Options
claudette search --format json "prompt"   # JSON output
claudette search --limit 10 "prompt"      # More results
claudette search --threshold 3 "prompt"   # Stricter matching
```

## Updating

```bash
go install github.com/dotcommander/claudette/cmd/claudette@latest
```

The hook picks up the new binary immediately — no settings.json changes needed.
