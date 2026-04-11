# Setup & Installation

## Prerequisites

- Go 1.22+ (`go version`)
- Claude Code with hooks support (`~/.claude/settings.json`)

## Install

```bash
cd ~/go/src/claudette
go install ./cmd/claudette/
```

This places the binary at `~/go/bin/claudette`. Verify:

```bash
claudette version
```

## Build the Index

```bash
claudette scan
```

This walks `~/.claude/kb/`, skills, agents, commands, and plugin directories, then caches the result at `~/.config/claudette/index.json`. The index auto-rebuilds when source files change, so you only need to run this manually on first install.

## Wire Up the Hook

Add claudette as a `UserPromptSubmit` hook in `~/.claude/settings.json`. Find (or create) the `hooks.UserPromptSubmit` array and add:

```json
{
  "matcher": "",
  "hooks": [
    {
      "type": "command",
      "command": "/Users/YOUR_USERNAME/go/bin/claudette hook",
      "timeout": 3000
    }
  ]
}
```

Replace `YOUR_USERNAME` with your actual username, or use the full path from `which claudette`.

If you already have a `UserPromptSubmit` hook entry (e.g., a dc plugin hook), add claudette as a second entry in the array — both will fire.

## Add the CLAUDE.md Directive

Add this near the top of `~/.claude/CLAUDE.md` so Claude reads surfaced entries:

```markdown
# Knowledge Base

`~/.claude/kb/` contains verified technical knowledge extracted from prior sessions — gotchas, race conditions, API quirks, and patterns that cost real debugging time to discover. The `claudette` hook automatically surfaces relevant entries on each prompt. **When entries are surfaced, read them before proceeding** — they are higher-tier knowledge than what you'll derive from first principles. If the hook surfaces nothing but the task clearly involves a KB category (go, openai, claude-code, piglet, bash, llm, refactoring, zai), manually scan that directory.
```

## Verify

Test the hook end-to-end:

```bash
# Should return JSON with matching KB entries
echo '{"prompt":"fix goroutine race condition"}' | claudette hook

# Should exit silently (no output)
echo '{"prompt":"update the README"}' | claudette hook

# Should exit silently (slash commands skipped)
echo '{"prompt":"/commit"}' | claudette hook
```

Benchmark:

```bash
time echo '{"prompt":"go cobra openai hook refactoring"}' | claudette hook
# Target: <50ms
```

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

After pulling changes:

```bash
cd ~/go/src/claudette
go install ./cmd/claudette/
```

The hook picks up the new binary immediately — no settings.json changes needed.

## Removing the Old Hook

If you previously used `kb-lookup.sh`, remove or replace its entry in `settings.json`:

```diff
- "command": "/Users/YOUR_USERNAME/.claude/hooks/kb-lookup.sh",
+ "command": "/Users/YOUR_USERNAME/go/bin/claudette hook",
```

The bash script can be deleted — claudette is a strict superset (covers KB + skills + agents + commands, 40x faster).
