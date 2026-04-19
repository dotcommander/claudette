# Session Analysis

```bash
claudette turns ~/.claude/projects/-Users-you-go-src-myapp/a3f1c2d4-ee12-4b87-91aa-fbc12309cafe.jsonl
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

Claudette parses Claude Code session transcripts locally. No network calls, no API, no effect on hook latency.

## Finding Sessions

### List Your Projects

```bash
claudette projects
```

```
  my-api        /Users/you/go/src/my-api           12 sessions
  claudette     /Users/you/go/src/claudette          8 sessions
  piglet        /Users/you/go/src/piglet             3 sessions
```

Each row shows the short project name, the original filesystem path decoded from `~/.claude/projects/`, and the number of sessions recorded for that project.

```bash
claudette projects --json
```

```json
[
  {
    "name": "claudette",
    "path": "/Users/you/go/src/claudette",
    "encoded": "-Users-you-go-src-claudette",
    "session_count": 8
  }
]
```

### List Sessions for the Current Project

```bash
claudette sessions
```

```
  a3f1c2d4-...  2026-04-18 14:22  47 messages   ~/.claude/projects/…/a3f1c2d4-…
  b9e82f11-...  2026-04-17 09:05  31 messages   ~/.claude/projects/…/b9e82f11-…
```

Claudette detects the current project from `$PWD` and lists its sessions newest first, up to 10 by default.

```bash
claudette sessions --all --limit 5   # newest 5 sessions across all projects
claudette sessions --json            # structured JSON
```

| Flag | Default | Description |
|------|---------|-------------|
| `--all` | false | Show sessions from all projects, not just the current one |
| `--limit N` | `10` | Maximum sessions to return |
| `--json` | false | Emit JSON array instead of text |

### Session Directory Layout

Claude Code stores sessions at:

```
~/.claude/projects/<encoded-cwd>/<session-uuid>.jsonl
```

The `<encoded-cwd>` is your working directory with `/` and `.` replaced by hyphens. For example, `/Users/you/go/src/myapp` becomes `-Users-you-go-src-myapp`. `claudette projects` decodes this for you.

Some sessions also have a companion directory (same UUID as the transcript, sitting beside it as a sibling):

```
~/.claude/projects/<encoded-cwd>/<session-uuid>/subagents/agent-<id>.jsonl
~/.claude/projects/<encoded-cwd>/<session-uuid>/tool-results/...
```

The companion directory is optional — sessions without subagents or large tool outputs have no sibling dir. The `turns` command works on both main transcripts and subagent transcripts — pass the path to any `.jsonl` file.

## Parsing Turns

### Basic Turn Extraction

```bash
claudette turns ~/.claude/projects/-Users-you-go-src-myapp/<uuid>.jsonl
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
  ⚠ Frustrated
```

By default, `turns` shows the first 5 turns, with user and assistant text truncated to 200 characters.

```bash
claudette turns <path> --json
```

```json
[
  {
    "index": 1,
    "timestamp": "2026-04-18T11:42:00Z",
    "user_content": "fix the goroutine race in internal/index/cache.go",
    "assistant_summary": "I'll read the file first and trace the lock usage…",
    "files_read": ["internal/index/cache.go", "internal/index/entry.go"],
    "files_edited": ["internal/index/cache.go"],
    "tools_used": ["Bash"],
    "frustrated": false
  }
]
```

### What's in a Turn

| Field | What it contains |
|-------|-----------------|
| `Timestamp` | When the user sent this message |
| `UserContent` | The user's prompt (200 chars by default; `--full` for complete text) |
| `AssistantSummary` | First 200 chars of the assistant's first text response |
| `FilesRead` | Files the assistant read (`Read`, `Glob`, `Grep` tool calls) |
| `FilesEdited` | Files the assistant wrote (`Edit`, `Write`, `MultiEdit` tool calls) |
| `ToolsUsed` | All other tools invoked (`Bash`, `Skill`, `Task`, etc.) |
| `Frustrated` | `true` if the user's message contained frustration signals |

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--limit N` | `5` | Maximum number of turns to show |
| `--full` | false | Show complete user and assistant text without truncation |
| `--json` | false | Emit JSON array instead of text |

## Subagent Transcripts

Each spawned agent writes its own transcript in the optional companion directory (same UUID as the main transcript, sitting beside it):

```
~/.claude/projects/<encoded-cwd>/<session-uuid>/subagents/agent-<id>.jsonl
```

Pass the path directly to `turns`:

```bash
claudette turns ~/.claude/projects/…/<uuid>/subagents/agent-abc123.jsonl
```

The output format is identical to a main transcript. Use this to inspect what a delegated agent read, edited, and did during its run.

## Performance Notes

- Parsing runs entirely on the local filesystem — no network I/O.
- A large session transcript (thousands of messages) parses in under a second.
- Parsed results are cached at `~/.config/claudette/sessions.json`. The cache invalidates automatically when the transcript file's mtime or size changes.

> **Note:** Session parsing never runs in the hook path. It has no effect on the `UserPromptSubmit` or `PostToolUseFailure` hook latency.
