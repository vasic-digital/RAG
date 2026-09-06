#!/usr/bin/env bash
# stress_sustained_load_challenge.sh — anti-bluff Stress Challenge
# for RAG per CONST-035 + CONST-050(B). Cascade per CONST-051(A).
#
# THREE-VALUED, and a 2 is never a pass:
#   0 = the target sustained the load within the pass and degradation budgets
#   1 = a real finding — throughput or latency budget blown, or target died
#   2 = COULD NOT DETERMINE — no target configured, target unreachable, or
#       the latency baseline could not be sampled. Nothing is asserted.
#
# Two defects fixed here (measured 2026-09-06):
#   1. The unreachable branches printed "PASSED (SKIP-OK)" and exited 0 — a
#      stopped service read, by exit code, as having sustained the load.
#   2. The baseline and post-load latency samplers each ended in `|| true`,
#      and the median was taken as `sort -n | awk NR==5`. Fewer than 5
#      successful samples yields an EMPTY median, and step [5/6] then fed
#      those empties to awk, which coerces both to 0, computes a degradation
#      of -100%, and PASSES. A total collapse of latency measurement — the
#      exact thing step [5/6] exists to measure — scored as a pass.

set -uo pipefail
HEALTH_URL="${RAG_HEALTH_URL:-}"
DURATION="${STRESS_DURATION_SEC:-15}"
RPS="${STRESS_REQUESTS_PER_SEC:-50}"
CONCURRENCY="${STRESS_CONCURRENCY:-20}"
TIMEOUT_SEC="${STRESS_TIMEOUT_SEC:-5}"
MIN_PASS_PCT="${STRESS_MIN_PASS_PCT:-95}"
MAX_DEG_PCT="${STRESS_MAX_LATENCY_DEGRADATION_PCT:-300}"

echo "=== RAG Stress Sustained-Load Challenge ==="
echo "  url=$HEALTH_URL dur=${DURATION}s rps=${RPS}"

if [[ -z "$HEALTH_URL" ]]; then
    echo "[1/6] NO TARGET: RAG_HEALTH_URL unset — no load applied"
    echo "=== RAG Stress Challenge: UNDETERMINED (no target configured) ==="
    echo "  a 2 is never a pass; set RAG_HEALTH_URL to obtain a verdict"
    exit 2
fi
pre=$(curl -sS --max-time "$TIMEOUT_SEC" -o /dev/null -w "%{http_code}" "$HEALTH_URL" 2>/dev/null) || pre="000"
if [[ "$pre" != "200" ]]; then
    echo "[1/6] UNREACHABLE: pre-flight HTTP $pre — no load applied"
    echo "=== RAG Stress Challenge: UNDETERMINED (target unreachable) ==="
    echo "  a 2 is never a pass; this says nothing about sustained-load behaviour"
    exit 2
fi
echo "[1/6] Pre-stress: PASS"

base=$(mktemp); trap "rm -f $base" EXIT
for _ in $(seq 1 10); do
    curl -sS -o /dev/null --max-time "$TIMEOUT_SEC" -w "%{time_total}\n" "$HEALTH_URL" 2>/dev/null >> "$base" || true
done
base_n=$(grep -c . "$base")
base_med=$(sort -n "$base" | awk 'NR==5{print; exit}')
echo "[2/6] Baseline median: ${base_med:-<none>}s (${base_n}/10 samples succeeded)"
if [[ "$base_n" -lt 5 ]] || [[ -z "$base_med" ]]; then
    echo "  UNDETERMINED: fewer than 5 baseline latency samples — no median to compare against"
    echo "=== RAG Stress Challenge: UNDETERMINED (baseline unmeasurable) ==="
    echo "  a 2 is never a pass; latency degradation cannot be computed from an absent baseline"
    exit 2
fi

body=$(curl -sS --max-time "$TIMEOUT_SEC" "$HEALTH_URL" 2>/dev/null || true)
printf '%s' "$body" | grep -qE '"status"\s*:\s*"(ok|healthy|UP)"' || { echo "[3/6] FAIL"; exit 1; }
echo "[3/6] Schema sanity: PASS"

RES=$(mktemp); trap "rm -f $base $RES" EXIT
start=$(date +%s.%N)
total_target=$((DURATION * RPS))
seq 1 "$total_target" | xargs -n1 -P "$CONCURRENCY" -I{} \
    curl -sS -o /dev/null --max-time "$TIMEOUT_SEC" \
        -w "%{http_code} %{time_total}\n" "$HEALTH_URL" 2>/dev/null >> "$RES" || true
finish=$(date +%s.%N)
wall=$(awk -v a="$start" -v b="$finish" 'BEGIN{printf "%.3f", b-a}')
total=$(wc -l < "$RES" | tr -d ' '); [[ "$total" -eq 0 ]] && total=1
ok=$(awk '$1=="200"{c++} END{print c+0}' "$RES")
pct=$((ok * 100 / total))
echo "[4/6] Sustained: $ok/$total ${pct}% wall=${wall}s"
[[ "$pct" -lt "$MIN_PASS_PCT" ]] && { echo "  FAIL"; exit 1; }

post_base=$(mktemp); trap "rm -f $base $RES $post_base" EXIT
for _ in $(seq 1 10); do
    curl -sS -o /dev/null --max-time "$TIMEOUT_SEC" -w "%{time_total}\n" "$HEALTH_URL" 2>/dev/null >> "$post_base" || true
done
post_n=$(grep -c . "$post_base")
post_med=$(sort -n "$post_base" | awk 'NR==5{print; exit}')
if [[ "$post_n" -lt 5 ]] || [[ -z "$post_med" ]]; then
    # Do NOT let an empty median coerce to 0 and score as a -100% improvement.
    echo "[5/6] Latency: ${base_med}s → <unmeasurable> (${post_n}/10 post-load samples succeeded)"
    echo "  FAIL: the target answered fewer than 5 of 10 post-load probes"
    echo "=== RAG Stress Challenge: FAILED (post-load latency unmeasurable) ==="
    exit 1
fi
deg=$(awk -v a="$base_med" -v b="$post_med" 'BEGIN{if(a<=0)a=0.0001; printf "%.0f", (b-a)*100/a}')
echo "[5/6] Latency: ${base_med}s → ${post_med}s (Δ=${deg}%, ${post_n}/10 samples)"
[[ "$deg" -gt "$MAX_DEG_PCT" ]] && { echo "  FAIL"; exit 1; }

post=$(curl -sS --max-time "$TIMEOUT_SEC" -o /dev/null -w "%{http_code}" "$HEALTH_URL" 2>/dev/null) || post="000"
[[ "$post" != "200" ]] && { echo "[6/6] FAIL"; exit 1; }
echo "[6/6] Post-stress liveness: PASS"

echo
echo "=== RAG Stress Challenge: PASSED ==="
echo "  evidence: dur=${wall}s reqs=${total} pct=${pct}% baseline=${base_med}s deg=${deg}%"
