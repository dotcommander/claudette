# Installation

Claudette wires itself into Claude Code as a hook. Once installed, every prompt you type automatically surfaces relevant knowledge from `~/.claude/` — no manual lookup required.

## Quick Start

```bash
go install github.com/dotcommander/claudette/cmd/claudette@latest
claudette install
```

Open a new Claude Code session. Claudette is active.

## Requirements

- Go 1.21+
- Claude Code (`claude` CLI) with `~/.claude/settings.json`

## Installing

`claudette install` modifies three things and tells you exactly what it touched:

```
Installing claudette...
  settings: /Users/you/.claude/settings.json
  hooks:    + UserPromptSubmit -> /Users/you/go/bin/claudette hook
  hooks:    + PostToolUse     -> /Users/you/go/bin/claudette post-tool-use
  config:   wrote /Users/you/.config/claudette/config.json
  index:    881 entries cached

Installed. Hooks active on next Claude Code session.
Reverse with: claudette uninstall
```

Re-running `claudette install` is safe — it skips already-wired hooks and just rebuilds the index:

```
Installing claudette...
  settings: /Users/you/.claude/settings.json
  hooks:    already wired (idempotent no-op)
  config:   /Users/you/.config/claudette/config.json (existing)
  index:    881 entries cached
```

> **Note:** Hooks activate on the *next* Claude Code session you open, not the current one. Claude Code reads `settings.json` at session start.

## Verifying

Confirm the hook protocol works end-to-end:

```bash
# Should print JSON to stdout and a timing line to stderr
echo '{"prompt":"fix goroutine race condition"}' | claudette hook
```

Expected stderr: `claudette: [goroutine race condition] -> entry1(5), entry2(4) (12ms)`

```bash
# Slash commands are skipped — no stdout, just a skip log on stderr
echo '{"prompt":"/commit"}' | claudette hook
# stderr: claudette: skip: slash command (0ms)

# Benchmark — hook should return in under 50ms
time echo '{"prompt":"go cobra openai hook refactoring"}' | claudette hook
```

Search the index directly to confirm entries are loaded:

```bash
claudette search "goroutine race"
claudette kb "sqlite connection"
claudette skill "refactor"
```

## CLAUDE.md Directive

For Claude to act on surfaced entries, add this to `~/.claude/CLAUDE.md`:

```markdown
# Knowledge Base

`~/.claude/kb/` contains verified technical knowledge from prior sessions.
When entries appear in system reminders, read them before proceeding —
they are higher-tier knowledge than first principles.
```

Without this directive, claudette surfaces entries in the context block, but Claude has no instruction to prioritize them.

## Uninstalling

```bash
claudette uninstall
```

This removes every hook entry claudette owns from `~/.claude/settings.json`, deletes `~/.config/claudette/`, and prints the exact command to remove the binary. Hooks from other tools are left untouched.

```
Uninstalling claudette...
  settings: /Users/you/.claude/settings.json
  hooks:    removed 2 entry/entries
  config:   removed /Users/you/.config/claudette

Uninstalled.
Binary still installed at /Users/you/go/bin/claudette
Remove with: rm /Users/you/go/bin/claudette
```

## Updating

```bash
go install github.com/dotcommander/claudette/cmd/claudette@latest
```

No hook changes needed — `settings.json` already points to the binary path, and the new binary is picked up immediately.

Check your installed version:

```bash
claudette --version   # or: claudette -v
```

## Building from Source

```bash
git clone git@github.com:dotcommander/claudette.git
cd claudette
go build -o claudette ./cmd/claudette/
go install ./cmd/claudette/
claudette install
```

After building from a local checkout, `claudette --version` appends a commit hash (or `-dirty` if the tree has uncommitted changes):

```
9bc4cd0-dirty
```

A clean `go install ...@latest` reports the module version:

```
v0.7.0
```
