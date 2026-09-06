#!/usr/bin/env bash
# ux_end_to_end_flow_challenge.sh — anti-bluff UX Challenge for
# RAG per CONST-035 + CONST-050(B). Cascade per CONST-051(A).
#
# THREE-VALUED, and a 2 is never a pass:
#   0 = the binary was driven through the full journey without a hostile leak
#   1 = a real finding — a panic/stack trace/crash reached the terminal
#   2 = COULD NOT DETERMINE — RAG_BIN unset or not executable.
#
# Two defects fixed here (measured 2026-09-06), identical to the UI challenge:
#   1. The missing-binary branch printed "PASSED (SKIP-OK)" and exited 0.
#   2. assert_no_panic returned FAILURE on its clean path — the loop body's
#      final `grep -qE ... && { ...; return 1; }` evaluates to grep's status
#      of 1 when the pattern does not match, and every caller is
#      `assert_no_panic ... || exit 1`. Measured against a well-behaved stub
#      binary: rc=1, with no diagnostic printed. Fixing it does not weaken the
#      assertion — the assertion could not previously pass at all.

set -uo pipefail
BIN_PATH="${RAG_BIN:-}"
TIMEOUT_SEC="${UX_TIMEOUT_SEC:-30}"
USER_HOSTILE=('panic:' 'goroutine [0-9]+ \[running\]:' 'runtime error:' 'segmentation fault' 'fatal error:')

echo "=== RAG UX End-to-End Flow Challenge ==="
echo "  bin=$BIN_PATH timeout=${TIMEOUT_SEC}s"

if [[ -z "$BIN_PATH" ]]; then
    echo "[1/5] NO TARGET: RAG_BIN unset — no journey exercised"
    echo "=== RAG UX Challenge: UNDETERMINED (no binary configured) ==="
    echo "  a 2 is never a pass; set RAG_BIN to obtain a verdict"
    exit 2
fi
if [[ ! -x "$BIN_PATH" ]]; then
    echo "[1/5] UNUSABLE TARGET: RAG_BIN='$BIN_PATH' is not an executable file"
    echo "=== RAG UX Challenge: UNDETERMINED (binary not executable) ==="
    echo "  a 2 is never a pass; no user journey was observed"
    exit 2
fi
echo "[1/5] Binary present: PASS"

assert_no_panic() {
    local label="$1" body="$2" pat
    for pat in "${USER_HOSTILE[@]}"; do
        if printf '%s' "$body" | grep -qE "$pat"; then
            echo "  FAIL: $label leaked: $pat"
            return 1
        fi
    done
    return 0
}

help_out=$(timeout "$TIMEOUT_SEC" "$BIN_PATH" --help 2>&1 || timeout "$TIMEOUT_SEC" "$BIN_PATH" -h 2>&1 || true)
assert_no_panic "--help" "$help_out" || exit 1
[[ -z "$help_out" ]] && { echo "[2/5] FAIL: empty help"; exit 1; }
echo "[2/5] Help discovery: PASS"

ver_out=$(timeout "$TIMEOUT_SEC" "$BIN_PATH" --version 2>&1 || timeout "$TIMEOUT_SEC" "$BIN_PATH" -v 2>&1 || true)
assert_no_panic "--version" "$ver_out" || exit 1
echo "[3/5] Version surface: PASS"

# This script never sets errexit (see `set -uo pipefail` above), so the old
# `set -e` here did not RESTORE a saved state — it silently TURNED ON errexit
# for everything after it.
bogus_out=$(timeout "$TIMEOUT_SEC" "$BIN_PATH" --does-not-exist-flag 2>&1)
bogus_exit=$?
assert_no_panic "bogus" "$bogus_out" || exit 1
[[ "$bogus_exit" -ge 124 ]] && { echo "[4/5] FAIL: crashed"; exit 1; }
echo "[4/5] Graceful recovery: PASS (exit $bogus_exit)"

post=$(timeout "$TIMEOUT_SEC" "$BIN_PATH" --help 2>&1 || timeout "$TIMEOUT_SEC" "$BIN_PATH" -h 2>&1 || true)
assert_no_panic "post-error --help" "$post" || exit 1
[[ -z "$post" ]] && { echo "[5/5] FAIL"; exit 1; }
echo "[5/5] Post-error liveness: PASS"

echo
echo "=== RAG UX Challenge: PASSED ==="
echo "  evidence: journey=discover→help→version→recover→post-liveness bogus_exit=$bogus_exit"
