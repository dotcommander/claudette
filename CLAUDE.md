# CLAUDE.md

Claudette is your partner — a UserPromptSubmit hook that runs on every prompt, surfaces relevant knowledge base entries, skills, agents, and commands before you respond. When you see `UserPromptSubmit hook additional context:` in system reminders, that's claudette working. Read the surfaced entries before proceeding.

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Build & Test

```bash
go build -o claudette ./cmd/claudette/
go install ./cmd/claudette/
go test ./...
go test ./internal/search/ -run TestScorer   # single package/test
```

## Architecture

Claudette indexes Claude Code components (`~/.claude/kb/`, skills, agents, commands, plus plugin dirs) into a cached JSON index, then scores entries against user prompts via keyword overlap.

### Two-Tier Entry Point

`cmd/claudette/main.go` checks `os.Args[1] == "hook"` and calls `hook.Run()` **before** cobra initializes. This bypasses cobra's ~15ms startup to hit the <50ms hook latency target. All other subcommands go through cobra normally.

### Packages

- `internal/index/` — Entry types, frontmatter parsing, filesystem scanning, dir-based type classification; index cache (load/save/staleness/IDF)
- `internal/config/` — Persistent config (source dirs + context header) at `~/.config/claudette/config.json`
- `internal/settings/` — Hook-entry read/write against `~/.claude/settings.json`
- `internal/usage/` — Append-only usage tracking log
- `internal/fileutil/` — Atomic write helpers (temp-file + rename) shared by config/settings/usage
- `internal/search/` — Tokenizer (stop words, hyphen-preserving split), category alias map, keyword-overlap scorer
- `internal/actions/` — Business logic (search, scan, install); `FilterTypes` lives on `SearchOpts`
- `internal/hook/` — UserPromptSubmit + PostToolUseFailure hook modes (stdin JSON → stdout context)
- `internal/output/` — Text and JSON formatters

### Index & Staleness

Index cached at `~/.config/claudette/index.json`. Staleness detected by comparing **file count + max mtime** of all `.md` files across source dirs against cached values. A file rename without mtime change won't trigger rebuild (acceptable trade-off). Atomic writes via temp-file-then-rename prevent corruption.

Source dirs discovered at runtime: `~/.claude/kb`, `~/.claude/{skills,agents,commands}`, plus plugin install dirs from `~/.claude/plugins/installed_plugins.json`. Missing dirs silently skipped.

### Scoring Algorithm

Each entry gets pre-tokenized keywords at index time (from name, title, category, first 200 chars of description).

Per token in the user prompt:
- **+2** if token matches a category alias (e.g., "golang" -> category "go") — defined in `internal/search/aliases.go`
- **+1** for direct keyword match
- **+1** for plural normalization match (bidirectional suffix-s: "test" matches "tests" and vice versa)

Results filtered by threshold (default 2), capped by limit (default 5), sorted by score desc then name asc.

### Hook Protocol

**UserPromptSubmit**
- Input (stdin): `{"prompt": "user text"}`
- Output (stdout): `{"hookSpecificOutput": {"hookEventName": "UserPromptSubmit", "additionalContext": "..."}}`

**PostToolUseFailure** (fires only on tool failures — no regex sniffing, no success-path noise)
- Input (stdin): `{"tool_name": "...", "tool_input": {...}, "tool_response": "..."}`
- Output (stdout): `{"hookSpecificOutput": {"hookEventName": "PostToolUseFailure", "additionalContext": "..."}}`

All errors exit silently (no stdout/stderr) — hook must never block the user's conversation. Prompts starting with `/` are skipped (slash commands, not searches). Hook hardcodes threshold=2, limit=5.

## No Remote Calls (BLOCKING)

Claudette runs on every prompt in the hook hot path. Any remote call — LLM API, embedding service, HTTP fetch — adds unacceptable latency and a network dependency to a synchronous hook.

- **Zero network I/O in the hook path.** No `http.Client`, no API calls, no DNS lookups. The hook must work fully offline in <50ms.
- **No LLM calls anywhere in claudette.** No embedding generation, no semantic similarity via API, no "enrich with AI" features that call external models. All intelligence is local: keyword overlap, aliases, plural normalization, IDF weighting.
- **No remote data fetching.** No fetching skill/agent/command definitions from registries, GitHub, or plugin APIs. All data comes from local filesystem paths discovered at runtime.
- **Offline enrichment only.** If future features need AI-generated summaries or semantic clustering, they run as a separate background CLI command (e.g., `claudette enrich`), not in the hook. The hook always reads pre-computed local data.
- Adding an LLM or HTTP dependency to `go.mod` requires an explicit exception with a latency budget justification.

## Conventions

- Minimal config. `claudette install` writes `~/.config/claudette/config.json` for source dirs; everything else is convention. (`init` is kept as a back-compat alias.)
- Two external deps only: cobra, yaml.v3.
- Entry type classification is directory-based (file in `skills/` dir = skill type), not content-based.
- Keyword extraction caps description at 200 chars to prevent keyword bloat.
- Tokenizer preserves internal hyphens ("multi-page" stays as one token) — important for component names like "dc:commit-pr".
