# Searching

Search your indexed knowledge base directly from the terminal without opening a Claude Code session.

## Quick Start

```bash
claudette search "goroutine race condition"
```

```
  [5] Concurrent Map Read/Write Race Condition
      kb/go/concurrent-map-race.md  (kb/go)
      matched: goroutine, race, condition

  [4] os.Chdir in Parallel Tests Races on Getwd
      kb/go/chdir-parallel-test-race-getwd.md  (kb/go)
      matched: goroutine, race
```

## Commands

### search

```bash
claudette search "sqlite connection pool"
```

Searches all entry types — KB articles, skills, agents, and commands. Results are sorted by score, highest first. Use this when you don't know which category contains the answer.

### kb

```bash
claudette kb "channel deadlock"
```

Restricts results to `~/.claude/kb/` entries. Faster when you know the answer is in your personal notes.

### skill

```bash
claudette skill "go concurrency"
claudette skill "database migration"
```

Restricts results to skill entries. Use this to find which skill to invoke for a task before calling it.

### scan

```bash
claudette scan
```

Forces a full index rebuild and reports the result:

```
Index rebuilt: 881 entries in 12 source directories (143ms)
```

Claudette auto-rebuilds when it detects file changes, but `scan` is useful after bulk edits or when you want immediate confirmation that new entries are indexed.

## Flags

All search commands accept these flags. Flags must come before the query.

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--format` | string | `text` | Output format: `text` or `json` |
| `--threshold` | int | `2` | Minimum score to include a result |
| `--limit` | int | `5` | Maximum number of results to return |

### Adjust threshold for broader or narrower recall

Lower the threshold when a query returns nothing but you know relevant entries exist:

```bash
claudette search --threshold 1 "pgxpool"
```

Raise it to filter out weaker matches:

```bash
claudette search --threshold 5 "go race condition"
```

### Return more results

```bash
claudette search --limit 20 "authentication"
```

### JSON output

```bash
claudette search --format json "sqlite"
```

```json
{
  "matches": [
    {
      "type": "kb",
      "name": "sqlite-driver-name",
      "title": "SQLite Driver Name is sqlite Not sqlite3",
      "category": "go",
      "path": "/Users/you/.claude/kb/go/sqlite-driver-name.md",
      "score": 6,
      "matched": ["sqlite"]
    }
  ],
  "total": 1
}
```

JSON output is useful for scripting and for inspecting exact scores during entry authoring.

## Practical Examples

Find what to read after a build error:

```bash
claudette kb "undefined: chi.NewRouter"
```

Find the right skill for a task:

```bash
claudette skill "refactoring solid dry"
```

Check a specific entry's score against a query:

```bash
claudette search --format json "pgx pool" \
  | jq '.matches[] | select(.name == "pgx-pool-max-conns") | {score, matched}'
```

Get the file path of the top result to open it:

```bash
claudette search --format json "concurrent map race" | jq -r '.matches[0].path'
```

Audit which entries surface for a prompt you type frequently:

```bash
claudette search --limit 10 --threshold 1 "fix goroutine"
```

## Session Commands

Browse Claude Code projects and session transcripts from the terminal.

```bash
claudette projects           # list all Claude Code projects
claudette sessions           # sessions for the current project
claudette turns <path>       # parse a session transcript into turns
```

### projects

```bash
claudette projects
```

```
  my-api        /Users/you/go/src/my-api           12 sessions
  claudette     /Users/you/go/src/claudette          8 sessions
  piglet        /Users/you/go/src/piglet             3 sessions
```

Lists every Claude Code project claudette knows about, decoded from the `~/.claude/projects/` directory. Each row shows the short name, the original filesystem path, and how many sessions exist.

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

| Flag | Default | Description |
|------|---------|-------------|
| `--json` | false | Emit JSON array instead of text |

### sessions

```bash
claudette sessions
```

```
  a3f1c2d4-...  2026-04-18 14:22  47 messages   ~/.claude/projects/…/a3f1c2d4-…
  b9e82f11-...  2026-04-17 09:05  31 messages   ~/.claude/projects/…/b9e82f11-…
```

Lists sessions for the current project (detected from `$PWD`). Results are sorted newest first, limited to 10 by default.

```bash
claudette sessions --all --limit 5   # sessions across all projects, newest 5
claudette sessions --json            # JSON output
```

| Flag | Default | Description |
|------|---------|-------------|
| `--all` | false | Show sessions from all projects, not just the current one |
| `--limit N` | `10` | Maximum number of sessions to return |
| `--json` | false | Emit JSON array instead of text |

### turns

```bash
claudette turns ~/.claude/projects/<encoded-cwd>/<uuid>.jsonl
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

Parses a session JSONL transcript and shows extracted turns. Each turn includes the user prompt, the assistant's opening response (first 200 chars), files read, files edited, and other tools used.

```bash
claudette turns <path> --full    # show complete user and assistant text, not truncated
claudette turns <path> --limit 3 # show only the first 3 turns
claudette turns <path> --json    # structured JSON output
```

The `--json` flag emits one object per turn:

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

| Flag | Default | Description |
|------|---------|-------------|
| `--limit N` | `5` | Maximum number of turns to show |
| `--full` | false | Show complete user and assistant text (default: 200 chars) |
| `--json` | false | Emit JSON array instead of text |

> **Note:** Turn parsing runs entirely on local files — no network I/O, no API calls. It never runs in the hook path and has no effect on hook latency.

## Version

```bash
claudette --version
claudette -v
claudette version
```

All three print the same version string.
