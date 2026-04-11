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

while IFS=$'\t' read -r category prompt expected snm; do
  [[ "$category" == "category" ]] && continue  # skip header
  [[ "$category" == \#* ]] && continue          # skip comments
  [[ -z "$category" ]] && continue              # skip blank lines
  CATEGORIES+=("$category")
  PROMPTS+=("$prompt")
  EXPECTED_MATCH+=("$expected")
  SHOULD_NOT_MATCH+=("${snm:-0}")
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

printf '%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n' \
  "prompt" "tokens" "top_entry" "top_score" "num_results" "latency_ms" "expected_match" "verdict" \
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
    first_item="${result_items[0]# }"
    [[ "$first_item" =~ ^(.+)\(([0-9]+)\)$ ]] && {
      top_entry="${BASH_REMATCH[1]}"
      top_score="${BASH_REMATCH[2]}"
    }
    run_status="matched"
  fi

  # Determine verdict
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

  safe_prompt="${prompt//$'\t'/ }"
  safe_prompt="${safe_prompt//$'\n'/ }"
  [[ ${#safe_prompt} -gt 100 ]] && safe_prompt="${safe_prompt:0:97}..."

  printf '%s\t%s\t%s\t%s\t%d\t%s\t%s\t%s\n' \
    "$safe_prompt" "$tokens" "$top_entry" "$top_score" \
    "$num_results" "$latency_ms" "$expected" "$verdict" \
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

while IFS=$'\t' read -r _ _ _ _ _ _ _ verdict; do
  case "$verdict" in
    FALSE_POSITIVE)   count_false_pos=$(( count_false_pos + 1 )) ;;
    FALSE_NEGATIVE)   count_false_neg=$(( count_false_neg + 1 )) ;;
    EXPECTED_MISSING) count_expected_missing=$(( count_expected_missing + 1 )) ;;
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
echo ""

echo "TOP 10 MOST-SURFACED ENTRIES"
echo "----------------------------"
tail -n +2 "$TSV_OUT" \
  | awk -F'\t' '$3 != "" {print $3}' \
  | sort | uniq -c | sort -rn | head -10 \
  | while read -r cnt name; do
      printf "  %3d  %s\n" "$cnt" "$name"
    done
echo ""

echo "TOP 10 PROMPTS BY SCORE"
echo "-----------------------"
tail -n +2 "$TSV_OUT" \
  | awk -F'\t' '$4+0 > 0 {printf "%05d\t%s\t%s\n", $4+0, $1, $3}' \
  | sort -rn | head -10 \
  | while IFS=$'\t' read -r score prompt entry; do
      printf "  score=%-4d  %-45s  -> %s\n" "$((10#$score))" "${prompt:0:45}" "$entry"
    done
echo ""

echo "BOTTOM 10 NON-ZERO PROMPTS (weakest matches)"
echo "----------------------------------------------"
tail -n +2 "$TSV_OUT" \
  | awk -F'\t' '$4+0 > 0 {printf "%05d\t%s\t%s\n", $4+0, $1, $3}' \
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
  tail -n +2 "$TSV_OUT" | while IFS=$'\t' read -r prompt _ _ _ _ _ expected verdict; do
    if [[ "$verdict" == "FALSE_NEGATIVE" ]]; then
      printf "  expected=%-40s  prompt: %s\n" "$expected" "${prompt:0:60}"
    fi
  done
fi
echo ""

echo "EXPECTED ENTRY MISSING FROM RESULTS"
echo "------------------------------------"
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

echo "FALSE POSITIVES"
echo "---------------"
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
