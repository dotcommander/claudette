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

## Version

```bash
claudette --version
claudette -v
claudette version
```

All three print the same version string.
