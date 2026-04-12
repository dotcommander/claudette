# Writing Knowledge Base Entries

A well-written KB entry surfaces automatically when you need it. A poorly written one sits in `~/.claude/kb/` and never appears.

## Quick Start

Create a `.md` file in `~/.claude/kb/<category>/`:

```markdown
---
name: go-sqlite-driver-name
description: SQLite driver name is "sqlite" not "sqlite3" — wrong name causes silent failure
tags: [go, sqlite, database, driver, modernc]
---

# SQLite Driver Name is "sqlite" Not "sqlite3"

Use `"sqlite"` when opening a database with `modernc.org/sqlite`. Using
`"sqlite3"` silently fails or panics at runtime.

\```go
// Correct
db, err := sql.Open("sqlite", "./app.db")

// Wrong — panics with: unknown driver "sqlite3"
db, err := sql.Open("sqlite3", "./app.db")
\```
```

Run `claudette scan` to index it immediately. Then test:

```bash
claudette kb "sqlite driver panic"
claudette kb "sqlite3 unknown driver"
```

## Frontmatter Schema

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `name` | string | Recommended | Unique identifier. Appears in search results and usage logs. |
| `description` | string | Recommended | One-liner summary. Shown in compact output mode. |
| `tags` | string[] | Recommended | Topic labels. Same weight as the title — use them. |

```yaml
---
name: pgx-pool-max-conns
description: Default pgx pool maxConns is 4 — too low for production, causes connection exhaustion
tags: [go, postgresql, pgx, pgxpool, connection-pool, production, pool]
---
```

> **Note:** Frontmatter is optional. Claudette indexes any `.md` file and uses the filename as the entry name if frontmatter is absent. But entries without `name` and `description` surface less reliably and show blank descriptions in compact mode.

## Keyword Weight Reference

Keywords are extracted from four sources, each with a different weight. Higher weight means those terms score more strongly in a search:

| Source | Weight | What to put here |
|--------|--------|-----------------|
| `name` (frontmatter) | 3 | The most specific technical term for this problem |
| H1 title | 2 | Natural-language description you'd use when asking Claude |
| `tags` (frontmatter list) | 2 | All related package names, error message words, concept names |
| `description` (frontmatter) | 1 | A sentence that places the entry in context |
| Body (first 200 chars) | 1 | Opening sentence — put key terms early |

### Name: be specific

The `name` field gets weight 3 — the highest. Put the most precise technical term here.

```yaml
# Good — "mattn", "sqlite3", "cgo" are high-signal, low-frequency terms
name: mattn-sqlite3-cgo-breaks-ci

# Less effective — "sqlite", "note", "issue" are too common
name: sqlite-issue
```

### Tags: cover all the ways you'd phrase the problem

Tags get weight 2. Think of tags as the vocabulary someone uses when they're debugging:

```yaml
# Covers: "pgx pool", "maxconns", "connection exhaustion", "pool 4", "pgxpool prod"
tags: [go, postgresql, pgx, pgxpool, maxconns, pool, connection-pool, exhaustion, production]
```

### Body: front-load key terms

Only the first 200 characters of the body contribute to keyword extraction. The example below puts `map`, `concurrent`, `race`, `sync.RWMutex` in the first sentence — all before the 200-char cutoff:

```markdown
# Concurrent Map Read/Write Race Condition

Go's built-in `map` is not safe for concurrent reads and writes. The race
detector reports: `concurrent map read and map write`. Use `sync.RWMutex`
or `sync.Map` for shared maps across goroutines.
```

Compare this to an entry that opens with narrative:

```markdown
# Concurrent Map Read/Write Race Condition

I ran into this while building the cache layer in March 2025. We had been
seeing intermittent panics in production that only appeared under load...
```

The first 200 chars spend their weight budget on `building`, `cache`, `march`, `seeing` — none of which match a debugging query.

## Category Organization

The subdirectory under `~/.claude/kb/` becomes the entry's category. Category names map to search aliases — queries containing an alias term get a +2 bonus on all entries in that category:

| Category directory | Aliases that give +2 |
|-------------------|---------------------|
| `kb/go/` | golang, goroutine |
| `kb/postgresql/` | postgres, pg |
| `kb/typescript/` | ts |
| `kb/kubernetes/` | k8s |

Put entries in the right category and the alias bonus applies automatically. An entry in `kb/go/` surfaces when you type "golang" even if that exact word doesn't appear in the entry.

## Entry Types Beyond KB

The same frontmatter schema applies to skills, agents, and commands in their respective directories. Claudette indexes all of them.

```markdown
---
name: dc:lang-go-dev
description: Go patterns, architecture, linting, production safety
tags: [go, golang, concurrency, goroutine, cli, chi, cobra, pgx, testing]
---

# Go Development Skill
...
```

A well-tagged skill surfaces in the hook when you ask a Go question, giving Claude a pointer to invoke it.

## Testing an Entry

After writing an entry, confirm it surfaces before relying on it in a session.

```bash
# Rebuild the index
claudette scan

# Test the exact phrasing you'd use while debugging
claudette kb "sqlite3 unknown driver"

# Test alternate phrasings
claudette kb "sqlite fails silently"
claudette kb "sql.Open panic driver"

# Inspect the score in JSON to understand what's matching
claudette search --format json "sqlite driver" \
  | jq '.matches[] | {name, score, matched}'
```

If the entry doesn't appear, lower the threshold to see if it's scoring below the cutoff:

```bash
claudette search --threshold 1 "sqlite driver" | grep "go-sqlite-driver-name"
```

If it still doesn't appear at threshold 1, no tokens from your query matched any indexed keyword. The query terms and the entry's name/tags/description share no words (after stop-word filtering). Add the missing terms as tags.

## Common Mistakes

| Mistake | Effect | Fix |
|---------|--------|-----|
| Vague `name` like `sqlite-note` | Only surfaces on exact phrase "sqlite note" | Use the canonical error term: `sqlite-driver-name-sqlite-not-sqlite3` |
| No `tags` | Misses alias bonuses and alternate phrasings | Add every related term you'd type while debugging |
| Narrative opening | First 200 body chars carry no searchable signal | Lead with the technical fact and key terms |
| Wrong category directory | No alias bonus | `kb/go/` not `kb/misc/` for Go topics |
| Description that repeats the title | Compact mode shows duplicate info | Description should add context, not restate the name |
