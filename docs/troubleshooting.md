# Troubleshooting

## Hook Not Firing

**Symptom:** Prompts produce no `<related_skills_knowledge>` blocks in Claude's context.

**Step 1:** Verify the hooks are registered.

```bash
jq '.hooks | to_entries | map(select(.value | tostring | contains("claudette"))) | map(.key)' \
  ~/.claude/settings.json
```

Expected output: `["PostToolUseFailure", "UserPromptSubmit"]`

If the output is `[]`, the hooks aren't registered. Run `claudette install`.

**Step 2:** Confirm the binary path matches what's in `settings.json`.

```bash
which claudette
jq '[.hooks.UserPromptSubmit[].hooks[].command]' ~/.claude/settings.json
```

If you reinstalled Go or changed `$GOPATH`, the binary moved but `settings.json` still points to the old path. Run `claudette install` — it detects the current binary and updates both hook entries in place.

**Step 3:** Test the hook protocol directly.

```bash
echo '{"prompt":"fix goroutine race condition"}' | claudette hook
```

If this returns JSON, claudette works. The issue is that Claude Code isn't invoking the hook. Check whether you're in a new session — hooks activate on session start, not mid-session.

**Step 4:** Open a new Claude Code session.

Hooks are read from `settings.json` when a session starts. `claudette install` during an active session takes effect only after you open a new one.

---

## No Results for a Query

**Symptom:** `claudette search "your query"` returns nothing.

Lower the threshold to see whether entries exist but score below the cutoff:

```bash
claudette search --threshold 1 "your query"
```

If results appear at threshold 1, the entries match but weakly. Either the query is too vague or the entries need better `name` and `tags`. See [Writing Entries](writing-entries.md).

Check that the index has content:

```bash
claudette scan
# Expected: Index rebuilt: N entries in M source directories
```

If N is 0, claudette found no `.md` files. Verify your source directories contain markdown files and are listed in `~/.config/claudette/config.json`.

---

## Entries Appear in CLI but Not in the Hook

The hook applies three filters the CLI doesn't:

| Filter | CLI | Hook |
|--------|-----|------|
| Minimum score | `--threshold 2` (configurable) | 2 (fixed) |
| Confidence gate | Not applied | Top result must score ≥ 4 |
| Single-token floor | Not applied | Score ≥ 8 for single-token matches |

An entry scoring 3 appears in CLI results but the hook suppresses it — the confidence gate requires the top result to score at least 4 before claudette writes anything to stdout.

To understand what the hook sees, check scores with JSON output:

```bash
claudette search --format json "your query" | jq '.matches[] | {name, score}'
```

If the top score is 3, the hook will suppress it. Improve the entry's `name` and `tags` to raise the score, or check whether the query needs more specific terms.

---

## Hooks Slow — Latency Over 50ms

The index rebuild happens synchronously when claudette detects stale files. On large `~/.claude/` trees this can spike.

Count your indexed files:

```bash
find ~/.claude/kb ~/.claude/skills ~/.claude/agents ~/.claude/commands \
  -name "*.md" | wc -l
```

If you have thousands of files, narrow `source_dirs` in `~/.config/claudette/config.json` to the directories you actually use day-to-day.

The index is cached at `~/.config/claudette/index.json` — rebuilds only happen when the file count or newest mtime changes. If the index is current, hook overhead is scoring-only and should be well under 50ms.

---

## Version Mismatch After Updating

```bash
claudette --version
```

If this reports an old version after running `go install ...@latest`, the shell is returning a cached lookup:

```bash
which claudette    # Confirm this is the expected path
hash -r            # Clear shell command cache (bash/zsh)
claudette --version
```

---

## Config File Missing

If `~/.config/claudette/config.json` is absent (e.g., after `claudette uninstall`), claudette falls back to the four default source directories. Running `claudette install` recreates the config file.

---

## Hook Removed Another Tool's Entry

Claudette's uninstall only removes hook entries whose command string contains `"claudette"`. If another tool's hook is missing after `claudette uninstall`, inspect the settings:

```bash
jq '.hooks' ~/.claude/settings.json
```

If the entry is genuinely gone, restore it from the backup claudette prints during uninstall — or re-run that tool's own install command.

File a bug at [github.com/dotcommander/claudette](https://github.com/dotcommander/claudette) with the `settings.json` diff if claudette incorrectly removed an unrelated hook.

---

## Diagnostic Reference

### stderr log messages

Every hook call logs one line to stderr. These are diagnostic — they never appear in Claude's context.

| Message | Meaning | Action |
|---------|---------|--------|
| `[tokens] -> entry1(5), entry2(4) (12ms)` | Matched entries — normal operation | None |
| `[tokens] -> no matches (8ms)` | No entries scored above threshold | Normal for vague prompts |
| `[tokens] -> suppressed (low confidence, top score 3)` | Top result below confidence gate | Improve entry tags |
| `[tokens] -> suppressed (single-token weak match, score 5)` | One token, not strong enough | Add more query terms or improve entry name |
| `skip: slash command` | Prompt starts with `/` | Expected behavior |
| `skip: no searchable tokens` | Prompt is all stop words | Expected behavior |
| `skip: empty prompt` | Empty input | Expected behavior |
| `skip: index load failed` | Can't read index file | Run `claudette scan` |
| `skip: source discovery failed` | Can't read config or home dir | Run `claudette install` |
| `skip: empty tool response` | PostToolUseFailure — tool failed with no response text | Expected behavior |

### Quick diagnostics

```bash
# Check hook registration
jq '.hooks | to_entries | map(select(.value | tostring | contains("claudette"))) | map(.key)' \
  ~/.claude/settings.json

# Test UserPromptSubmit hook
echo '{"prompt":"fix goroutine race condition"}' | claudette hook

# Test PostToolUseFailure hook with a simulated tool failure payload
echo '{"tool_name":"Bash","tool_input":{},"tool_response":"Error: undefined: chi.NewRouter"}' \
  | claudette post-tool-use-failure

# Check index health
claudette scan

# Inspect a specific entry's score
claudette search --format json "your query" | jq '.matches[] | {name, score, matched}'

# See all entries (broad search)
claudette search --threshold 0 --limit 100 --format json "go" | jq '.total'
```
