#!/usr/bin/env bash
# ui_terminal_interaction_challenge.sh — anti-bluff UI Challenge for
# RAG per CONST-035 + CONST-050(B). Cascade per CONST-051(A).
#
# THREE-VALUED, and a 2 is never a pass:
#   0 = the binary was exercised and never leaked a user-hostile crash
#   1 = a real finding — a panic/stack trace/crash reached the terminal
#   2 = COULD NOT DETERMINE — RAG_BIN unset or not executable.
#
# Two defects fixed here (measured 2026-09-06):
#   1. The missing-binary branch printed "PASSED (SKIP-OK)" and exited 0.
#   2. assert_no_panic could not report a clean binary. Its loop body ended in
#      `grep -qE "$pat" && { ...; return 1; }`; when the pattern did NOT match,
#      that AND-list evaluates to grep's status of 1, which became the loop's
#      status and therefore the function's return value. Every caller is
#      `assert_no_panic ... || exit 1`, so a binary with NO panic in its output
#      exited 1 — silently, because the FAIL message only prints on a match.
#      Measured against a well-behaved stub binary: rc=1 with no diagnostic.
#      The defect was invisible because RAG_BIN is never set, so the script
#      always took the SKIP-OK path above. Fixing it does not weaken the
#      assertion — the assertion could not previously pass at all.

set -uo pipefail
BIN_PATH="${RAG_BIN:-}"
TIMEOUT_SEC="${UI_TIMEOUT_SEC:-30}"
USER_HOSTILE=('panic:' 'goroutine [0-9]+ \[running\]:' 'runtime error:' 'segmentation fault' 'fatal error:')

echo "=== RAG UI Terminal-Interaction Challenge ==="
echo "  bin=$BIN_PATH timeout=${TIMEOUT_SEC}s"

if [[ -z "$BIN_PATH" ]]; then
    echo "[1/4] NO TARGET: RAG_BIN unset — no binary exercised"
    echo "=== RAG UI Challenge: UNDETERMINED (no binary configured) ==="
    echo "  a 2 is never a pass; set RAG_BIN to obtain a verdict"
    exit 2
fi
if [[ ! -x "$BIN_PATH" ]]; then
    echo "[1/4] UNUSABLE TARGET: RAG_BIN='$BIN_PATH' is not an executable file"
    echo "=== RAG UI Challenge: UNDETERMINED (binary not executable) ==="
    echo "  a 2 is never a pass; nothing about the terminal surface was observed"
    exit 2
fi
echo "[1/4] Binary present: PASS"

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
[[ -z "$help_out" ]] && { echo "[2/4] FAIL: empty help"; exit 1; }
echo "[2/4] Help: PASS"

ver_out=$(timeout "$TIMEOUT_SEC" "$BIN_PATH" --version 2>&1 || timeout "$TIMEOUT_SEC" "$BIN_PATH" -v 2>&1 || true)
assert_no_panic "--version" "$ver_out" || exit 1
echo "[3/4] Version: PASS"

# This script never sets errexit (see `set -uo pipefail` above), so the old
# `set -e` here did not RESTORE a saved state — it silently TURNED ON errexit
# for everything after it, which is not the shell semantics the rest of the
# script was written against.
bogus=$(timeout "$TIMEOUT_SEC" "$BIN_PATH" --this-flag-does-not-exist 2>&1)
bogus_exit=$?
[[ "$bogus_exit" -ge 124 ]] && { echo "[4/4] FAIL: crashed"; exit 1; }
assert_no_panic "bogus" "$bogus" || exit 1
echo "[4/4] Invalid-flag: PASS (exit $bogus_exit)"

echo
echo "=== RAG UI Challenge: PASSED ==="
echo "  evidence: bin=$BIN_PATH bogus_exit=$bogus_exit"
