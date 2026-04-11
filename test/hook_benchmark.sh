#!/usr/bin/env bash
# hook_benchmark.sh — Runs 100 diverse prompts through claudette hook mode
# and produces a search-quality analysis report.
#
# Usage: ./hook_benchmark.sh [--verbose]
# Output: summary report to stdout, TSV data file at ./hook_results.tsv
#
# Dependencies: bash 4+, jq, awk, sort, uniq — no external tools required.

set -euo pipefail

BINARY="${CLAUDETTE_BIN:-$HOME/go/bin/claudette}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TSV_OUT="$SCRIPT_DIR/hook_results.tsv"
VERBOSE="${1:-}"

if [[ ! -x "$BINARY" ]]; then
  echo "ERROR: claudette binary not found or not executable: $BINARY" >&2
  exit 1
fi

# ---------------------------------------------------------------------------
# Prompt definitions
# Each entry: category, prompt, expected_match_tag, should_not_match
#   expected_match_tag: entry name that SHOULD appear (empty = no expectation)
#   should_not_match:   "1" if this prompt should produce zero results
# ---------------------------------------------------------------------------

declare -a PROMPTS=()
declare -a EXPECTED_MATCH=()
declare -a SHOULD_NOT_MATCH=()
declare -a CATEGORIES=()

add_prompt() {
  # args: category prompt expected_match should_not_match
  CATEGORIES+=("$1")
  PROMPTS+=("$2")
  EXPECTED_MATCH+=("$3")
  SHOULD_NOT_MATCH+=("$4")
}

# --- Go development ---
add_prompt "go"     "how do goroutines work with channels"                          "extension-sdk-goroutine-capture-gotcha"         "0"
add_prompt "go"     "goroutine channel buffer deadlock"                             "extension-sdk-goroutine-capture-gotcha"         "0"
add_prompt "go"     "Go error handling best practices with fmt.Errorf and %w"       "02-error-handling"                              "0"
add_prompt "go"     "writing benchmarks in Go with testing.B"                       ""                                               "0"
add_prompt "go"     "how does the maps.Keys function work in Go"                    "maps-keys-returns-iterator-not-slice"            "0"
add_prompt "go"     "cobra CLI custom args validation friendly error messages"       "cobra-custom-args-validator-friendly-errors"    "0"
add_prompt "go"     "multi-module Go repo with cross dependencies"                  "multi-module-repo-cross-dependency-elimination" "0"
add_prompt "go"     "submodule tagging strategy for multi-module Go repository"     "multi-module-repo-submodule-tags"               "0"
add_prompt "go"     "how to avoid getwd race in parallel Go tests"                  "chdir-parallel-test-race-getwd"                 "0"
add_prompt "go"     "io.Writer interface must return original length"               "bounded-writer-must-return-original-len"         "0"
add_prompt "go"     "keyword scoring with IDF and bigram matching in Go"            "lightweight-keyword-search-scoring"             "0"
add_prompt "go"     "Go context cancellation with goroutines"                       ""                                               "0"
add_prompt "go"     "pgxpool connection pool configuration in Go"                   ""                                               "0"

# --- Python / FastAPI / Django ---
add_prompt "python" "FastAPI dependency injection and route handlers"               "" "0"
add_prompt "python" "Django ORM queryset optimization N+1 problem"                  "" "0"
add_prompt "python" "async await Python event loop"                                  "" "0"
add_prompt "python" "pydantic model validation in FastAPI"                           "" "0"
add_prompt "python" "Python uv package manager pyproject.toml setup"               "" "0"

# --- Frontend: React, Svelte, Tailwind ---
add_prompt "frontend" "React hooks useState useEffect rendering"                    "" "0"
add_prompt "frontend" "Svelte 5 runes and reactive state management"               "" "0"
add_prompt "frontend" "Tailwind CSS v4 migration from v3 CSS variables"            "" "0"
add_prompt "frontend" "SvelteKit SSR server side rendering routes"                  "" "0"
add_prompt "frontend" "Next.js App Router server components vs client components"   "" "0"
add_prompt "frontend" "CSS grid flexbox layout responsive design"                   "" "0"

# --- Claude Code: hooks, skills, agents, commands ---
add_prompt "claude-code" "Claude Code UserPromptSubmit hook JSON format"            "hook-reload-session-caching"          "0"
add_prompt "claude-code" "hook session caching and reload behavior"                 "hook-reload-session-caching"          "0"
add_prompt "claude-code" "how to write a Claude Code skill"                         ""                                     "0"
add_prompt "claude-code" "plugin binary bootstrap pattern for Claude Code"          "plugin-binary-bootstrap-pattern"      "0"
add_prompt "claude-code" "parallel agent A/B testing workflow"                      "parallel-agent-ab-testing-workflow"   "0"
add_prompt "claude-code" "Claude Code settings.json hook configuration"             ""                                     "0"
add_prompt "claude-code" "agent context window management and token budget"         "agent-loop-context-window-management" "0"
add_prompt "claude-code" "go formatter hook edit race condition"                    "go-formatter-hook-edit-race"          "0"
add_prompt "claude-code" "haiku model behavior and capabilities"                    "haiku-model-behavior-patterns"        "0"

# --- Refactoring / SOLID / code smells ---
add_prompt "refactoring" "SOLID principles single responsibility violation"          "detector-first-code-smell-pipeline"         "0"
add_prompt "refactoring" "detecting code smells in a pipeline pattern"               "detector-first-code-smell-pipeline"         "0"
add_prompt "refactoring" "DRY principle removing duplicated logic"                   ""                                           "0"
add_prompt "refactoring" "extract method refactoring to reduce nesting"              ""                                           "0"
add_prompt "refactoring" "technical debt assessment and prioritization"              ""                                           "0"
add_prompt "refactoring" "collapse repeated log lines before truncation"             "collapse-repeated-lines-before-truncation"  "0"

# --- DevOps / Docker / Kubernetes / CI ---
add_prompt "devops" "Docker multi-stage build optimization"                          "" "0"
add_prompt "devops" "Kubernetes pod deployment health checks liveness"              "" "0"
add_prompt "devops" "GitHub Actions workflow with matrix strategy"                   "" "0"
add_prompt "devops" "CI/CD pipeline caching dependencies"                            "" "0"
add_prompt "devops" "Dockerfile ENTRYPOINT vs CMD difference"                        "" "0"

# --- Databases: PostgreSQL, SQLite, Redis, migrations ---
add_prompt "databases" "PostgreSQL index selection and query planning"              "" "0"
add_prompt "databases" "SQLite WAL mode concurrent reads"                            "" "0"
add_prompt "databases" "Redis pub/sub pattern for real-time updates"               "" "0"
add_prompt "databases" "database migration rollback strategy"                        "" "0"
add_prompt "databases" "PostgreSQL connection pool exhaustion pgbouncer"             "" "0"

# --- LLM / AI / RAG / embeddings ---
add_prompt "llm" "LLM prompt engineering techniques for instruction following"       "prompt-ab-testing-what-works"               "0"
add_prompt "llm" "RAG retrieval augmented generation chunking strategy"             ""                                            "0"
add_prompt "llm" "embedding models for semantic search similarity"                   ""                                            "0"
add_prompt "llm" "fine-tuning large language models with LoRA"                      ""                                            "0"
add_prompt "llm" "llm as judge evaluation multi-trial calibration"                  "llm-as-judge-multi-trial-calibration"        "0"
add_prompt "llm" "token budget optimization for LLM cost reduction"                 "llm-cost-optimization-patterns"             "0"
add_prompt "llm" "GPT-4 nano reasoning token budget"                                "gpt54-nano-reasoning-token-budget"           "0"
add_prompt "llm" "LLM provider model namespace collision slash"                     "llm-provider-model-slash-namespace-collision" "0"
add_prompt "llm" "agent loop context window overflow handling"                       "agent-loop-context-window-management"       "0"
add_prompt "llm" "prompt A/B testing what works and what does not"                  "prompt-ab-testing-what-works"               "0"
add_prompt "llm" "OpenAI SDK v3 parameter wiring Go client"                         "go-sdk-v3-param-wiring"                     "0"

# --- Security ---
add_prompt "security" "XSS cross-site scripting prevention sanitization"            "" "0"
add_prompt "security" "CSRF token validation in web applications"                   "" "0"
add_prompt "security" "JWT token authentication refresh strategy"                   "" "0"
add_prompt "security" "OAuth2 PKCE flow for single page applications"              "" "0"
add_prompt "security" "SQL injection prevention parameterized queries"              "" "0"

# --- Piglet / ZAI ---
add_prompt "piglet" "piglet extension SDK goroutine variable capture"               "extension-sdk-goroutine-capture-gotcha" "0"
add_prompt "piglet" "testing piglet extensions with jsonrpc protocol"               "extension-testing-jsonrpc-protocol"     "0"
add_prompt "zai"    "zai dual endpoint standard vs coding plan"                     "dual-endpoint-standard-vs-coding-plan"  "0"
add_prompt "zai"    "zai coding assistant endpoint selection"                       "dual-endpoint-standard-vs-coding-plan"  "0"

# --- Bigram matching tests ---
add_prompt "bigram" "error handling Go"                                              "" "0"
add_prompt "bigram" "channel goroutine buffer overflow race"                        "" "0"
add_prompt "bigram" "code smell detector pipeline refactoring"                      "detector-first-code-smell-pipeline"     "0"
add_prompt "bigram" "dev tty existence openability macOS bash"                      "dev-tty-existence-vs-openability-macos" "0"
add_prompt "bigram" "jq safe text replacement bash"                                 "jq-safe-text-replacement"               "0"

# --- Stop-word-heavy prompts (tokens after filtering should be few/none) ---
add_prompt "stopwords" "the and or but if then else"        "" "1"
add_prompt "stopwords" "is it possible to do this with that" "" "1"
add_prompt "stopwords" "how do I use this in my project"     "" "1"
add_prompt "stopwords" "what is the best way to do it"       "" "1"
add_prompt "stopwords" "can you help me with this please"    "" "1"

# --- Generic/vague prompts: SHOULD produce no results ---
add_prompt "vague" "hello"       "" "1"
add_prompt "vague" "what time is it" "" "1"
add_prompt "vague" "thanks"      "" "1"
add_prompt "vague" "ok got it"   "" "1"
add_prompt "vague" "looks good"  "" "1"
add_prompt "vague" "yes please"  "" "1"
add_prompt "vague" "no"          "" "1"

# --- Slash commands: SHOULD be skipped (suppressed) ---
add_prompt "slash" "/help"   "" "1"
add_prompt "slash" "/clear"  "" "1"
add_prompt "slash" "/commit" "" "1"

# --- Edge cases ---
add_prompt "edge" ""                                                                       "" "1"
add_prompt "edge" "$(printf 'x %.0s' {1..200})"                                           "" "0"
add_prompt "edge" "fix bug"                                                                "" "0"
add_prompt "edge" "refactor this function please make it cleaner and more readable"       "" "0"
add_prompt "edge" 'special chars: <>&"'"'"' tab tab {}[]'                                 "" "0"
add_prompt "edge" "123 456 789 0"                                                          "" "1"
add_prompt "edge" "GOROUTINE CHANNEL ERROR HANDLING uppercase"                             "" "0"

# ---------------------------------------------------------------------------
# Run each prompt through claudette hook and collect results
# ---------------------------------------------------------------------------

total="${#PROMPTS[@]}"

declare -a R_TOKENS=()
declare -a R_TOP_ENTRY=()
declare -a R_TOP_SCORE=()
declare -a R_NUM_RESULTS=()
declare -a R_LATENCY=()
declare -a R_STATUS=()    # matched | skip | zero | error

[[ -n "$VERBOSE" ]] && echo "Running $total prompts through claudette hook..." >&2

# Write TSV header
printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
  "prompt" "tokens" "top_entry" "top_score" "num_results" "latency_ms" "expected_match" "verdict" \
  > "$TSV_OUT"

for i in "${!PROMPTS[@]}"; do
  prompt="${PROMPTS[$i]}"
  expected="${EXPECTED_MATCH[$i]}"
  snm="${SHOULD_NOT_MATCH[$i]}"

  # Build JSON safely with jq to handle special characters
  json_input=$(jq -n --arg p "$prompt" '{"prompt": $p}')

  # Capture stdout and stderr separately into temp files
  stdout_tmp=$(mktemp)
  stderr_tmp=$(mktemp)

  echo "$json_input" | "$BINARY" hook >"$stdout_tmp" 2>"$stderr_tmp" || true

  stderr_line=$(cat "$stderr_tmp")
  rm -f "$stdout_tmp" "$stderr_tmp"

  # Parse stderr. Formats emitted by hook.go:
  #   claudette: skip: <reason> (Nms)
  #   claudette: [tok1 tok2] -> no matches (Nms)
  #   claudette: [tok1 tok2] -> name(8), name(5) (Nms)
  #   claudette: stdin error (Nms)

  latency_ms="0"
  tokens=""
  top_entry=""
  top_score="0"
  num_results=0
  run_status="error"

  # Extract latency from trailing "(Nms)"
  if [[ "$stderr_line" =~ \(([0-9]+)ms\)$ ]]; then
    latency_ms="${BASH_REMATCH[1]}"
  fi

  if [[ "$stderr_line" == *"claudette: skip:"* ]]; then
    run_status="skip"

  elif [[ "$stderr_line" == *"-> no matches"* ]]; then
    run_status="zero"
    if [[ "$stderr_line" =~ \[([^]]*)\] ]]; then
      tokens="${BASH_REMATCH[1]}"
    fi

  elif [[ "$stderr_line" == *" -> "* ]]; then
    # Has match results — extract tokens and parse entries
    if [[ "$stderr_line" =~ claudette:\ \[([^]]*)\]\ -\> ]]; then
      tokens="${BASH_REMATCH[1]}"
    fi

    # Strip prefix through " -> " and trailing " (Nms)"
    results_part="${stderr_line#* -> }"
    results_part="${results_part% \(*ms\)}"

    # Count comma-separated entries
    IFS=',' read -ra result_items <<< "$results_part"
    num_results="${#result_items[@]}"

    # Parse top entry: "name(score)" or " name(score)"
    first_item="${result_items[0]# }"
    if [[ "$first_item" =~ ^(.+)\(([0-9]+)\)$ ]]; then
      top_entry="${BASH_REMATCH[1]}"
      top_score="${BASH_REMATCH[2]}"
    fi

    run_status="matched"
  fi

  # Determine verdict for quality analysis
  verdict="ok"
  if [[ "$snm" == "1" && "$run_status" == "matched" ]]; then
    verdict="FALSE_POSITIVE"
  elif [[ -n "$expected" ]]; then
    if [[ "$stderr_line" == *"$expected"* ]]; then
      verdict="ok"
    elif [[ "$run_status" == "zero" || "$run_status" == "skip" ]]; then
      verdict="FALSE_NEGATIVE"
    else
      verdict="EXPECTED_MISSING"
    fi
  fi

  R_TOKENS[$i]="$tokens"
  R_TOP_ENTRY[$i]="$top_entry"
  R_TOP_SCORE[$i]="$top_score"
  R_NUM_RESULTS[$i]="$num_results"
  R_LATENCY[$i]="$latency_ms"
  R_STATUS[$i]="$run_status"

  # Sanitize prompt for TSV (replace tabs, newlines; truncate long prompts)
  safe_prompt="${prompt//$'\t'/ }"
  safe_prompt="${safe_prompt//$'\n'/ }"
  if [[ ${#safe_prompt} -gt 100 ]]; then
    safe_prompt="${safe_prompt:0:97}..."
  fi

  printf '%s\t%s\t%s\t%s\t%d\t%s\t%s\t%s\n' \
    "$safe_prompt" \
    "$tokens" \
    "$top_entry" \
    "$top_score" \
    "$num_results" \
    "$latency_ms" \
    "$expected" \
    "$verdict" \
    >> "$TSV_OUT"

  if [[ -n "$VERBOSE" ]]; then
    printf "  [%3d/%d] %-12s %-8s %s\n" \
      "$((i+1))" "$total" "${CATEGORIES[$i]}" "$run_status" \
      "${top_entry:-${run_status}}" >&2
  fi
done

# ---------------------------------------------------------------------------
# Compute statistics — avoid ((n++)) on zero which returns exit 1 under set -e
# ---------------------------------------------------------------------------

count_matched=0
count_suppressed=0
count_zero=0
count_error=0
count_false_pos=0
count_false_neg=0
count_expected_missing=0
latency_sum=0

declare -a latencies=()
declare -A entry_freq=()

for i in "${!PROMPTS[@]}"; do
  st="${R_STATUS[$i]}"
  lat="${R_LATENCY[$i]}"

  case "$st" in
    matched) count_matched=$(( count_matched + 1 )) ;;
    skip)    count_suppressed=$(( count_suppressed + 1 )) ;;
    zero)    count_zero=$(( count_zero + 1 )) ;;
    error)   count_error=$(( count_error + 1 )) ;;
  esac

  latency_sum=$(( latency_sum + lat ))
  latencies+=("$lat")

  entry="${R_TOP_ENTRY[$i]}"
  if [[ -n "$entry" ]]; then
    entry_freq["$entry"]=$(( ${entry_freq["$entry"]:-0} + 1 ))
  fi
done

# Read verdicts from TSV (guaranteed accurate)
while IFS=$'\t' read -r _ _ _ _ _ _ _ verdict; do
  case "$verdict" in
    FALSE_POSITIVE)   count_false_pos=$(( count_false_pos + 1 )) ;;
    FALSE_NEGATIVE)   count_false_neg=$(( count_false_neg + 1 )) ;;
    EXPECTED_MISSING) count_expected_missing=$(( count_expected_missing + 1 )) ;;
  esac
done < <(tail -n +2 "$TSV_OUT")

# Compute latency percentiles
n_lat="${#latencies[@]}"
IFS=$'\n' sorted_latencies=($(printf '%s\n' "${latencies[@]}" | sort -n))
unset IFS

lat_avg=$(( latency_sum / ( n_lat > 0 ? n_lat : 1 ) ))
lat_p50="${sorted_latencies[$(( n_lat / 2 ))]:-0}"
lat_p95_idx=$(( n_lat * 95 / 100 ))
lat_p95="${sorted_latencies[$lat_p95_idx]:-0}"
lat_max="${sorted_latencies[-1]:-0}"

# ---------------------------------------------------------------------------
# Report
# ---------------------------------------------------------------------------

echo "============================================================"
echo "  claudette hook benchmark -- $(date '+%Y-%m-%d %H:%M:%S')"
echo "============================================================"
echo ""
echo "SUMMARY"
echo "-------"
printf "  Total prompts:      %d\n" "$total"
printf "  Matched (results):  %d\n" "$count_matched"
printf "  Zero results:       %d\n" "$count_zero"
printf "  Suppressed (skip):  %d\n" "$count_suppressed"
printf "  Errors:             %d\n" "$count_error"
echo ""
echo "LATENCY (ms)"
echo "------------"
printf "  avg=%d  p50=%d  p95=%d  max=%d\n" \
  "$lat_avg" "$lat_p50" "$lat_p95" "$lat_max"
echo ""

echo "QUALITY SIGNALS"
echo "---------------"
printf "  False positives (should_not_match but matched):     %d\n" "$count_false_pos"
printf "  False negatives (expected entry not in results):    %d\n" "$count_false_neg"
printf "  Expected missing  (matched but wrong top entry):    %d\n" "$count_expected_missing"
echo ""

# Top 10 most-surfaced entries
echo "TOP 10 MOST-SURFACED ENTRIES"
echo "----------------------------"
tail -n +2 "$TSV_OUT" \
  | awk -F'\t' '$3 != "" {print $3}' \
  | sort | uniq -c | sort -rn | head -10 \
  | while read -r cnt name; do
      printf "  %3d  %s\n" "$cnt" "$name"
    done
echo ""

# Top 10 prompts by score
echo "TOP 10 PROMPTS BY SCORE (strongest signal)"
echo "------------------------------------------"
tail -n +2 "$TSV_OUT" \
  | awk -F'\t' '$4 != "" && $4+0 > 0 {printf "%05d\t%s\t%s\n", $4+0, $1, $3}' \
  | sort -rn | head -10 \
  | while IFS=$'\t' read -r score prompt entry; do
      printf "  score=%-4d  %-45s  -> %s\n" "$((10#$score))" "${prompt:0:45}" "$entry"
    done
echo ""

# Bottom 10 non-zero prompts (weakest matches)
echo "BOTTOM 10 NON-ZERO PROMPTS (weakest matches -- threshold candidates)"
echo "--------------------------------------------------------------------"
tail -n +2 "$TSV_OUT" \
  | awk -F'\t' '$4 != "" && $4+0 > 0 {printf "%05d\t%s\t%s\n", $4+0, $1, $3}' \
  | sort -n | head -10 \
  | while IFS=$'\t' read -r score prompt entry; do
      printf "  score=%-4d  %-45s  -> %s\n" "$((10#$score))" "${prompt:0:45}" "$entry"
    done
echo ""

# False negatives — run in main shell, not subshell, so we can check count
echo "FALSE NEGATIVES (expected match, got nothing or zero)"
echo "------------------------------------------------------"
if [[ "$count_false_neg" -eq 0 ]]; then
  echo "  (none)"
else
  tail -n +2 "$TSV_OUT" | while IFS=$'\t' read -r prompt _ _ _ _ _ expected verdict; do
    if [[ "$verdict" == "FALSE_NEGATIVE" ]]; then
      printf "  expected=%-40s  prompt: %s\n" "$expected" "${prompt:0:60}"
    fi
  done
fi
echo ""

# Expected-missing (matched but expected entry was not top)
echo "EXPECTED ENTRY MISSING FROM RESULTS (matched wrong)"
echo "----------------------------------------------------"
if [[ "$count_expected_missing" -eq 0 ]]; then
  echo "  (none)"
else
  tail -n +2 "$TSV_OUT" | while IFS=$'\t' read -r prompt _ top_entry _ _ _ expected verdict; do
    if [[ "$verdict" == "EXPECTED_MISSING" ]]; then
      printf "  expected=%-30s  got=%-30s  prompt: %s\n" "$expected" "$top_entry" "${prompt:0:40}"
    fi
  done
fi
echo ""

# False positives
echo "FALSE POSITIVES (should not match, but did)"
echo "--------------------------------------------"
if [[ "$count_false_pos" -eq 0 ]]; then
  echo "  (none)"
else
  tail -n +2 "$TSV_OUT" | while IFS=$'\t' read -r prompt _ top_entry _ _ _ _ verdict; do
    if [[ "$verdict" == "FALSE_POSITIVE" ]]; then
      printf "  matched=%-30s  prompt: %s\n" "$top_entry" "${prompt:0:60}"
    fi
  done
fi
echo ""

# Category distribution
echo "MATCH DISTRIBUTION BY CATEGORY"
echo "-------------------------------"
for cat in go python frontend claude-code refactoring devops databases llm security piglet zai bigram stopwords vague slash edge; do
  cat_total=0
  cat_matched=0
  for i in "${!PROMPTS[@]}"; do
    if [[ "${CATEGORIES[$i]}" == "$cat" ]]; then
      cat_total=$(( cat_total + 1 ))
      if [[ "${R_STATUS[$i]}" == "matched" ]]; then
        cat_matched=$(( cat_matched + 1 ))
      fi
    fi
  done
  if [[ "$cat_total" -gt 0 ]]; then
    printf "  %-15s  %2d/%2d matched\n" "$cat" "$cat_matched" "$cat_total"
  fi
done
echo ""

echo "DATA FILE: $TSV_OUT"
echo "Done."
