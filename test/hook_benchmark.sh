#!/usr/bin/env bash
# hook_benchmark.sh — Runs prompts from prompts.tsv through claudette hook mode
# and produces a search-quality analysis report.
#
# Usage: ./hook_benchmark.sh [--verbose]
# Output: summary report to stdout, TSV data file at ./hook_results.tsv
#
# Dependencies: bash 4+, jq, awk, sort, uniq

set -euo pipefail

BINARY="${CLAUDETTE_BIN:-$HOME/go/bin/claudette}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROMPTS_FILE="$SCRIPT_DIR/prompts.tsv"
TSV_OUT="$SCRIPT_DIR/hook_results.tsv"
VERBOSE="${1:-}"

if [[ ! -x "$BINARY" ]]; then
  echo "ERROR: claudette binary not found or not executable: $BINARY" >&2
  exit 1
fi
if [[ ! -f "$PROMPTS_FILE" ]]; then
  echo "ERROR: prompts file not found: $PROMPTS_FILE" >&2
  exit 1
fi

# ---------------------------------------------------------------------------
# Load prompts from TSV
# ---------------------------------------------------------------------------

declare -a CATEGORIES=()
declare -a PROMPTS=()
declare -a EXPECTED_MATCH=()
declare -a SHOULD_NOT_MATCH=()
declare -a EXPECTED_RANK=()
declare -a DENYLIST=()

while IFS= read -r _line; do
  # Replace tabs with \x01 (non-whitespace) so bash read preserves empty fields
  _line="${_line//$'\t'/$'\x01'}"
  IFS=$'\x01' read -r category prompt expected snm expected_rank denylist <<< "$_line"
  [[ "$category" == "category" ]] && continue  # skip header
  [[ "$category" == \#* ]] && continue          # skip comments
  [[ -z "$category" ]] && continue              # skip blank lines
  CATEGORIES+=("$category")
  PROMPTS+=("$prompt")
  EXPECTED_MATCH+=("$expected")
  SHOULD_NOT_MATCH+=("${snm:-0}")
  EXPECTED_RANK+=("${expected_rank:-}")
  DENYLIST+=("${denylist:-}")
done < "$PROMPTS_FILE"

total="${#PROMPTS[@]}"
[[ -n "$VERBOSE" ]] && echo "Loaded $total prompts from $PROMPTS_FILE" >&2

# ---------------------------------------------------------------------------
# Run each prompt through claudette hook
# ---------------------------------------------------------------------------

declare -a R_TOKENS=()
declare -a R_TOP_ENTRY=()
declare -a R_TOP_SCORE=()
declare -a R_NUM_RESULTS=()
declare -a R_LATENCY=()
declare -a R_STATUS=()
declare -a R_ALL_ENTRIES=()
declare -a R_ALL_SCORES=()
declare -a R_ACTUAL_RANK=()
declare -a R_EXPECTED_RANK=()

printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
  "prompt" "category" "tokens" "top_entry" "top_score" "num_results" "latency_ms" "expected_match" "expected_rank" "actual_rank" "verdict" \
  > "$TSV_OUT"

for i in "${!PROMPTS[@]}"; do
  prompt="${PROMPTS[$i]}"
  expected="${EXPECTED_MATCH[$i]}"
  snm="${SHOULD_NOT_MATCH[$i]}"

  json_input=$(jq -n --arg p "$prompt" '{"prompt": $p}')

  stderr_line=$(echo "$json_input" | "$BINARY" hook 2>&1 >/dev/null) || true

  latency_ms="0"
  tokens=""
  top_entry=""
  top_score="0"
  num_results=0
  run_status="error"
  all_entries=()
  all_scores=()

  if [[ "$stderr_line" =~ \(([0-9]+)ms\)$ ]]; then
    latency_ms="${BASH_REMATCH[1]}"
  fi

  if [[ "$stderr_line" == *"skip:"* ]]; then
    run_status="skip"
  elif [[ "$stderr_line" == *"-> no matches"* || "$stderr_line" == *"-> suppressed"* ]]; then
    run_status="zero"
    [[ "$stderr_line" =~ \[([^]]*)\] ]] && tokens="${BASH_REMATCH[1]}"
  elif [[ "$stderr_line" == *" -> "* ]]; then
    [[ "$stderr_line" =~ claudette:\ \[([^]]*)\]\ -\> ]] && tokens="${BASH_REMATCH[1]}"
    results_part="${stderr_line#* -> }"
    results_part="${results_part% \(*ms\)}"
    IFS=',' read -ra result_items <<< "$results_part"
    num_results="${#result_items[@]}"
    for item in "${result_items[@]}"; do
      item="${item# }"
      if [[ "$item" =~ ^(.+)\(([0-9]+)\)$ ]]; then
        all_entries+=("${BASH_REMATCH[1]}")
        all_scores+=("${BASH_REMATCH[2]}")
      fi
    done
    if [[ "${#all_entries[@]}" -gt 0 ]]; then
      top_entry="${all_entries[0]}"
      top_score="${all_scores[0]}"
    fi
    run_status="matched"
  fi

  # Determine verdict
  verdict="ok"
  actual_rank=0

  # Find rank of expected entry in all_entries
  if [[ -n "$expected" && "$run_status" == "matched" ]]; then
    for j in "${!all_entries[@]}"; do
      if [[ "${all_entries[$j]}" == "$expected" ]]; then
        actual_rank=$((j + 1))
        break
      fi
    done
  fi

  target_rank="${EXPECTED_RANK[$i]:-5}"
  [[ -z "$target_rank" ]] && target_rank=5

  if [[ "$snm" == "1" && "$run_status" == "matched" ]]; then
    verdict="FALSE_POSITIVE"
  elif [[ -n "$expected" ]]; then
    if [[ "$run_status" == "zero" || "$run_status" == "skip" ]]; then
      verdict="FALSE_NEGATIVE"
    elif [[ "$actual_rank" -eq 0 ]]; then
      verdict="EXPECTED_MISSING"
    elif [[ "$actual_rank" -gt "$target_rank" ]]; then
      verdict="RANK_TOO_LOW"
    fi
  fi

  # Denylist check (runs even when verdict=ok)
  if [[ "$verdict" == "ok" && -n "${DENYLIST[$i]}" && "$run_status" == "matched" ]]; then
    IFS=',' read -ra deny_items <<< "${DENYLIST[$i]}"
    for deny in "${deny_items[@]}"; do
      deny="${deny# }"; deny="${deny% }"
      [[ -z "$deny" ]] && continue
      for entry in "${all_entries[@]}"; do
        if [[ "$entry" == "$deny" ]]; then
          verdict="DENYLIST_HIT"
          break 2
        fi
      done
    done
  fi

  R_TOKENS[$i]="$tokens"
  R_TOP_ENTRY[$i]="$top_entry"
  R_TOP_SCORE[$i]="$top_score"
  R_NUM_RESULTS[$i]="$num_results"
  R_LATENCY[$i]="$latency_ms"
  R_STATUS[$i]="$run_status"
  _joined_entries="$(IFS='|'; echo "${all_entries[*]:-}")"
  _joined_scores="$(IFS='|'; echo "${all_scores[*]:-}")"
  R_ALL_ENTRIES[$i]="$_joined_entries"
  R_ALL_SCORES[$i]="$_joined_scores"
  R_ACTUAL_RANK[$i]="$actual_rank"
  R_EXPECTED_RANK[$i]="$target_rank"

  safe_prompt="${prompt//$'\t'/ }"
  safe_prompt="${safe_prompt//$'\n'/ }"
  [[ ${#safe_prompt} -gt 100 ]] && safe_prompt="${safe_prompt:0:97}..."

  printf '%s\t%s\t%s\t%s\t%s\t%d\t%s\t%s\t%s\t%s\t%s\n' \
    "$safe_prompt" "${CATEGORIES[$i]}" "$tokens" "$top_entry" "$top_score" \
    "$num_results" "$latency_ms" "$expected" "$target_rank" "$actual_rank" "$verdict" \
    >> "$TSV_OUT"

  if [[ -n "$VERBOSE" ]]; then
    printf "  [%3d/%d] %-12s %-8s %s\n" \
      "$((i+1))" "$total" "${CATEGORIES[$i]}" "$run_status" \
      "${top_entry:-${run_status}}" >&2
  fi
done

# ---------------------------------------------------------------------------
# Compute statistics
# ---------------------------------------------------------------------------

count_matched=0
count_suppressed=0
count_zero=0
count_error=0
count_false_pos=0
count_false_neg=0
count_expected_missing=0
count_rank_too_low=0
count_denylist_hit=0
latency_sum=0

declare -a latencies=()

for i in "${!PROMPTS[@]}"; do
  case "${R_STATUS[$i]}" in
    matched) count_matched=$(( count_matched + 1 )) ;;
    skip)    count_suppressed=$(( count_suppressed + 1 )) ;;
    zero)    count_zero=$(( count_zero + 1 )) ;;
    error)   count_error=$(( count_error + 1 )) ;;
  esac
  latency_sum=$(( latency_sum + R_LATENCY[$i] ))
  latencies+=("${R_LATENCY[$i]}")
done

while IFS=$'\t' read -r _ _ _ _ _ _ _ _ _ _ verdict; do
  case "$verdict" in
    FALSE_POSITIVE)   count_false_pos=$(( count_false_pos + 1 )) ;;
    FALSE_NEGATIVE)   count_false_neg=$(( count_false_neg + 1 )) ;;
    EXPECTED_MISSING) count_expected_missing=$(( count_expected_missing + 1 )) ;;
    RANK_TOO_LOW)     count_rank_too_low=$(( count_rank_too_low + 1 )) ;;
    DENYLIST_HIT)     count_denylist_hit=$(( count_denylist_hit + 1 )) ;;
  esac
done < <(tail -n +2 "$TSV_OUT")

n_lat="${#latencies[@]}"
IFS=$'\n' sorted_latencies=($(printf '%s\n' "${latencies[@]}" | sort -n))
unset IFS

lat_avg=$(( latency_sum / ( n_lat > 0 ? n_lat : 1 ) ))
lat_p50="${sorted_latencies[$(( n_lat / 2 ))]:-0}"
lat_p95="${sorted_latencies[$(( n_lat * 95 / 100 ))]:-0}"
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
printf "  Rank too low    (matched but below target rank):       %d\n" "$count_rank_too_low"
printf "  Denylist hits   (forbidden entry appeared in results): %d\n" "$count_denylist_hit"
echo ""

echo "TOP 10 MOST-SURFACED ENTRIES"
echo "----------------------------"
tail -n +2 "$TSV_OUT" \
  | awk -F'\t' '$4 != "" {print $4}' \
  | sort | uniq -c | sort -rn | head -10 \
  | while read -r cnt name; do
      printf "  %3d  %s\n" "$cnt" "$name"
    done
echo ""

echo "TOP 10 PROMPTS BY SCORE"
echo "-----------------------"
tail -n +2 "$TSV_OUT" \
  | awk -F'\t' '$5+0 > 0 {printf "%05d\t%s\t%s\n", $5+0, $1, $4}' \
  | sort -rn | head -10 \
  | while IFS=$'\t' read -r score prompt entry; do
      printf "  score=%-4d  %-45s  -> %s\n" "$((10#$score))" "${prompt:0:45}" "$entry"
    done
echo ""

echo "BOTTOM 10 NON-ZERO PROMPTS (weakest matches)"
echo "----------------------------------------------"
tail -n +2 "$TSV_OUT" \
  | awk -F'\t' '$5+0 > 0 {printf "%05d\t%s\t%s\n", $5+0, $1, $4}' \
  | sort -n | head -10 \
  | while IFS=$'\t' read -r score prompt entry; do
      printf "  score=%-4d  %-45s  -> %s\n" "$((10#$score))" "${prompt:0:45}" "$entry"
    done
echo ""

echo "FALSE NEGATIVES"
echo "---------------"
if [[ "$count_false_neg" -eq 0 ]]; then
  echo "  (none)"
else
  tail -n +2 "$TSV_OUT" | awk -F'\t' '$11 == "FALSE_NEGATIVE" {printf "  expected=%-40s  prompt: %s\n", $8, substr($1, 1, 60)}'
fi
echo ""

echo "EXPECTED ENTRY MISSING FROM RESULTS"
echo "------------------------------------"
if [[ "$count_expected_missing" -eq 0 ]]; then
  echo "  (none)"
else
  tail -n +2 "$TSV_OUT" | awk -F'\t' '$11 == "EXPECTED_MISSING" {printf "  expected=%-30s  got=%-30s  prompt: %s\n", $8, $4, substr($1, 1, 40)}'
fi
echo ""

echo "RANK TOO LOW (expected entry in results but ranked below target)"
echo "-----------------------------------------------------------------"
if [[ "$count_rank_too_low" -eq 0 ]]; then
  echo "  (none)"
else
  tail -n +2 "$TSV_OUT" | awk -F'\t' '$11 == "RANK_TOO_LOW" {printf "  expected=%s got_rank=%s target=%s  prompt: %s\n", $8, $10, $9, substr($1, 1, 60)}'
fi
echo ""

echo "DENYLIST HITS (forbidden entry appeared)"
echo "----------------------------------------"
if [[ "$count_denylist_hit" -eq 0 ]]; then
  echo "  (none)"
else
  tail -n +2 "$TSV_OUT" | awk -F'\t' '$11 == "DENYLIST_HIT" {printf "  top=%s  prompt: %s\n", $4, substr($1, 1, 60)}'
fi
echo ""

echo "FALSE POSITIVES"
echo "---------------"
if [[ "$count_false_pos" -eq 0 ]]; then
  echo "  (none)"
else
  tail -n +2 "$TSV_OUT" | awk -F'\t' '$11 == "FALSE_POSITIVE" {printf "  matched=%-30s  prompt: %s\n", $4, substr($1, 1, 60)}'
fi
echo ""

echo "MATCH DISTRIBUTION BY CATEGORY"
echo "-------------------------------"
declare -A cat_total=()
declare -A cat_matched=()
for i in "${!PROMPTS[@]}"; do
  cat="${CATEGORIES[$i]}"
  cat_total["$cat"]=$(( ${cat_total["$cat"]:-0} + 1 ))
  if [[ "${R_STATUS[$i]}" == "matched" ]]; then
    cat_matched["$cat"]=$(( ${cat_matched["$cat"]:-0} + 1 ))
  fi
done
for cat in $(printf '%s\n' "${!cat_total[@]}" | sort); do
  printf "  %-15s  %2d/%2d matched\n" "$cat" "${cat_matched[$cat]:-0}" "${cat_total[$cat]}"
done
echo ""

echo "DATA FILE: $TSV_OUT"
echo "Done."
