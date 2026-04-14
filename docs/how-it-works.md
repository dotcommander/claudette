# How It Works

Claudette runs as two Claude Code hooks. On every prompt, it scores your text against an index of everything in `~/.claude/` and injects the top matches as context before Claude responds.

## The Two Hooks

### UserPromptSubmit

This hook fires on every message you send. Claude Code passes your prompt as JSON on stdin; claudette scores it against the index and writes matching entries back to stdout as `additionalContext`.

```
You type:  "fix goroutine race condition"
              ↓
claudette: tokenize → ["goroutine", "race", "condition"]
              ↓
         score against index (BM25 + IDF)
              ↓
         top matches: go/concurrent-map-race (5), go/sync-pool-race (4)
              ↓
         write context JSON to stdout
              ↓
Claude sees:  <related_skills_knowledge>
                kb/go/concurrent-map-race.md — Concurrent Map Read/Write Race Condition
                kb/go/sync-pool-race.md — sync.Pool Race on Concurrent Put/Get
              </related_skills_knowledge>
```

The full round-trip runs in under 50ms on a warm index.

**Prompts claudette skips automatically:**

| Condition | Example | Reason |
|-----------|---------|--------|
| Slash command | `/commit` | Tool invocation, not a knowledge query |
| Empty prompt | `""` | Nothing to score |
| All stop words | `"ok great thanks"` | No searchable tokens after filtering |

### PostToolUseFailure

This hook fires **only when a tool call fails** — Claude Code does the failure detection, so claudette doesn't regex-sniff tool output. Successful tool calls don't invoke this hook at all.

```
A Bash tool call fails:
  "go build ./...: undefined: chi.NewRouter"
              ↓
Claude Code fires PostToolUseFailure with the tool response
              ↓
claudette: tokenize the response → ["undefined", "chi", "newrouter"]
              ↓
         score → kb/go/chi-import-path (6)
              ↓
Claude sees the chi import path KB entry without you asking
```

Because the hook only fires on actual failures, there are no false positives from successful output that happens to contain the word "error" or "fail" (source code, log files, docs).

## The Index

The index lives at `~/.config/claudette/index.json`. It's built from every `.md` file in your source directories.

### What Gets Indexed

| Source | Default Location | Entry Type |
|--------|-----------------|------------|
| Knowledge base | `~/.claude/kb/` | `kb` |
| Skills | `~/.claude/skills/` | `skill` |
| Agents | `~/.claude/agents/` | `agent` |
| Commands | `~/.claude/commands/` | `command` |
| Plugin components | From `~/.claude/plugins/installed_plugins.json` | varies |

Each `.md` file becomes one entry. The category field comes from the subdirectory name — a file at `kb/go/concurrent-map.md` has category `go`.

### Staleness Detection

Before every hook call, claudette compares the current file count and newest modification time against the cached index. If anything changed, it rebuilds inline before scoring.

**No daemon or file watcher** — the rebuild is synchronous and in-process. On a typical `~/.claude/` directory with hundreds of entries it takes 10–40ms.

### Keyword Weights

Keywords are extracted from each entry's frontmatter and content, weighted by source:

| Source | Weight | Indexing strategy |
|--------|--------|------------------|
| `name` (frontmatter) | 3 | Highest signal — put canonical error terms here |
| H1 title | 2 | Natural-language description of the problem |
| `tags` (frontmatter) | 2 | All related package names, error keywords, concepts |
| `description` (frontmatter) | 1 | Contextual — broadens recall on related queries |
| Body (first 200 chars) | 1 | Supplements precision with contextual terms |

## Scoring

Claudette uses a BM25-inspired algorithm with IDF (inverse document frequency) weighting.

### Per-Token Scoring

For each token that survives stop-word filtering:

| Match type | Points | Example |
|------------|--------|---------|
| Category alias | +2 | Query "golang" matches category `go`; "postgres" matches `postgresql` |
| Direct keyword | +1 × IDF | Token matches entry's indexed keywords |
| Plural stem | +1 | "test" matches entries with "tests", and vice versa |

IDF weights rare terms more than common ones. A match on `pgxpool` scores higher than a match on `error` because `pgxpool` appears in far fewer entries.

### Worked Example

Prompt: `"fix goroutine race condition"`

Stop words removed → tokens: `["goroutine", "race", "condition"]`

| Entry | goroutine | race | condition | Score |
|-------|-----------|------|-----------|-------|
| `kb/go/concurrent-map-race.md` | +2 (alias) +1 | +1 (IDF high) | +1 | **5** |
| `kb/go/chdir-parallel-test-race-getwd.md` | +1 | +2 (alias) +1 | 0 | **4** |
| `kb/go/bounded-writer.md` | +1 | 0 | 0 | **1** → below threshold |

The third entry drops below the threshold (2) and is excluded from results.

### Suppression Filters

The hook applies three filters that the CLI does not — they prevent weak or noisy matches from cluttering Claude's context:

| Filter | Value | Purpose |
|--------|-------|---------|
| Minimum score | ≥ 2 | Drops entries with very weak match |
| Confidence gate | Top result ≥ 4 | Suppresses all output if best match is borderline |
| Single-token floor | Score ≥ 8 | Single-keyword matches need strong signal to avoid false positives |

The confidence gate is why claudette produces no output for vague prompts like `"great work on the refactor"` — only `"refactor"` survives stop-word filtering, and one weak token doesn't clear the floor of 8.

## Output Format

Claudette injects context using the `hookSpecificOutput.additionalContext` field in the hook response JSON. Claude Code appends this to the system prompt for that turn.

### Full mode (default)

```
<related_skills_knowledge>
Scan first 10 lines of each file. Only read full files that are clearly relevant.

  kb/go/concurrent-map-race.md — Concurrent Map Read/Write Race Condition [matched: goroutine, race]
  kb/go/chdir-parallel-test-race-getwd.md — os.Chdir in Parallel Tests Races on Getwd [matched: goroutine, race]
</related_skills_knowledge>
```

Shows the file path relative to `~/.claude/` and the entry's H1 title.

### Compact mode

```
<related_skills_knowledge>
Scan first 10 lines of each file. Only read full files that are clearly relevant.

  go/concurrent-map-race — Races on concurrent map read/write [matched: goroutine, race]
  go/chdir-parallel-test — os.Chdir races with Getwd in parallel tests [matched: goroutine, race]
</related_skills_knowledge>
```

Shows the entry name and frontmatter description. Set via `CLAUDETTE_OUTPUT=compact`.

## Usage Tracking

Claudette appends every surfaced entry to `~/.config/claudette/usage.log`. Each record stores a timestamp, the entry name, and its score. Over time this enables hit-count aggregation — frequently surfaced entries signal high relevance.

The log is append-only and safe to truncate if it grows large:

```bash
> ~/.config/claudette/usage.log
```
