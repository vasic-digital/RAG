#!/usr/bin/env bash
# rag_unit_challenge.sh - Validates RAG module unit tests
#
# THREE-VALUED, and a 2 is never a pass:
#   0 = every declared test function EXECUTED and passed
#   1 = a real finding — a test failed, or a structural assertion failed
#   2 = COULD NOT DETERMINE — the run did not cover every declared test
#       function (self-skipped tests, an explicit -short run, or
#       executed != declared). Precedence: 1 outranks 2 outranks 0.
#
# WHY THIS GATE CHANGED (measured 2026-09-06 at 42a1428):
#   It used to run `go test -short`, and reported a bare PASS. Under -short,
#   all 27 test functions in tests/e2e, tests/integration, tests/security and
#   tests/stress call t.Skip() at their first line, so 116 of 143 declared
#   test functions executed and 27 did not — while the gate printed
#   "Results: 4/4 passed" and exited 0. Integration, e2e, stress and security
#   were NOT covered, and nothing in the output said so. Measured: the same
#   27 tests PASS in full mode and need no external infrastructure, so the
#   right fix is to run them, not to describe the hole.
#
#   `-short` is still reachable for a deliberately fast local loop, but it can
#   no longer produce a PASS: it forces the verdict to UNDETERMINED (2), which
#   is the honest verdict for a run that covered 116 of 143.
#     RAG_UNIT_ALLOW_SHORT=1 bash challenges/scripts/rag_unit_challenge.sh
#
#   GOMAXPROCS is no longer `export`ed. It used to leak from this gate into
#   every process it started and every child of those; it is now passed to the
#   two `go test` invocations only, and the resolved value is PRINTED as
#   evidence so a verdict can be read against the budget that produced it.

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MODULE_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
MODULE_NAME="RAG"

PASS=0
FAIL=0
UNDET=0
TOTAL=0

pass()  { PASS=$((PASS+1));  TOTAL=$((TOTAL+1)); echo "  PASS: $1"; }
fail()  { FAIL=$((FAIL+1));  TOTAL=$((TOTAL+1)); echo "  FAIL: $1"; }
undet() { UNDET=$((UNDET+1)); TOTAL=$((TOTAL+1)); echo "  UNDETERMINED: $1"; }

echo "=== ${MODULE_NAME} Unit Test Challenge ==="
echo ""

# Test 1: Test files exist
echo "Test: Test files exist"
test_count=$(find "${MODULE_DIR}" -name "*_test.go" | wc -l)
if [ "${test_count}" -gt 0 ]; then
    pass "Found ${test_count} test files"
else
    fail "No test files found"
fi

# Test 2: Tests exist in each package
echo "Test: Test coverage across packages"
pkgs_with_tests=0
for pkg_dir in "${MODULE_DIR}"/pkg/*/; do
    pkg_tests=$(find "$pkg_dir" -name "*_test.go" | wc -l)
    if [ "$pkg_tests" -gt 0 ]; then
        pkgs_with_tests=$((pkgs_with_tests + 1))
    fi
done
if [ "$pkgs_with_tests" -ge 3 ]; then
    pass "At least 3 packages have tests (found ${pkgs_with_tests})"
else
    fail "Only ${pkgs_with_tests} packages have tests (expected at least 3)"
fi

# Resource budget. These were the literals GOMAXPROCS=2 and -p 1, which gave a
# 2-core box and a 64-core box the same budget and could not be tuned to
# either. They are DEFAULTS now, not constants: the cap exists on purpose —
# `go test ./...` on this module will happily saturate a host and the parent
# workload it shares with — but the right number is a property of the host, not
# of this file. Override per run:
#   GOMAXPROCS=8 GOTEST_P=4 bash challenges/scripts/rag_unit_challenge.sh
# NOT exported: passed to the `go test` children only, so this gate cannot
# change the environment of anything else that runs after it.
GOMAXPROCS_VAL="${GOMAXPROCS:-2}"
GOTEST_P="${GOTEST_P:-1}"
GOTEST_NICE="${GOTEST_NICE:-19}"

# Coverage mode. -short is opt-in and can never yield a PASS.
ALLOW_SHORT="${RAG_UNIT_ALLOW_SHORT:-0}"
SHORT_FLAG=()
if [ "${ALLOW_SHORT}" = "1" ]; then
    SHORT_FLAG=(-short)
fi

# Declared population, read from source rather than from the runner, so
# "executed" can be compared against something the runner did not produce.
declared=$(grep -rhcE '^func Test[A-Z_]' --include='*_test.go' "${MODULE_DIR}" \
    | awk '{s+=$1} END{print s+0}')
declared="${declared:-0}"

echo ""
echo "  budget: GOMAXPROCS=${GOMAXPROCS_VAL} -p ${GOTEST_P} nice ${GOTEST_NICE}"
echo "  mode:   $([ "${ALLOW_SHORT}" = "1" ] && echo '-short (verdict capped at UNDETERMINED)' || echo 'full')"
echo "  declared test functions (from source): ${declared}"
echo ""

# run_suite <label> <logfile> [extra go-test flags...]
# Emits nothing; sets R_PASS/R_SKIP/R_FAIL/R_RC and writes the log.
run_suite() {
    local log="$1"; shift
    ( cd "${MODULE_DIR}" && env GOMAXPROCS="${GOMAXPROCS_VAL}" \
        nice -n "${GOTEST_NICE}" \
        go test -count=1 -p "${GOTEST_P}" -v "${SHORT_FLAG[@]}" "$@" ./... ) \
        > "$log" 2>&1
    R_RC=$?
    # Top-level results only: `go test -v` indents subtest result lines, so a
    # column-0 match counts each test FUNCTION exactly once. Verified against
    # `go test -json` on this tree: 116 PASS / 27 SKIP / 0 FAIL in -short mode,
    # 143 PASS / 0 SKIP / 0 FAIL in full mode.
    R_PASS=$(grep -c '^--- PASS:' "$log")
    R_SKIP=$(grep -c '^--- SKIP:' "$log")
    R_FAIL=$(grep -c '^--- FAIL:' "$log")
}

# Names the packages that self-skipped, so an UNDETERMINED verdict says WHICH
# populations were not covered instead of only that some were not.
skipped_packages() {
    awk '
        /^--- SKIP:/ { n++; next }
        /^(ok|FAIL|---|\?)[ \t]+[^ \t]+/ {
            if (n > 0) { printf "      %s (%d skipped)\n", $2, n }
            n = 0
        }
    ' "$1"
}

LOG_PLAIN="$(mktemp)"
LOG_RACE="$(mktemp)"
trap 'rm -f "$LOG_PLAIN" "$LOG_RACE"' EXIT

# Test 3: Unit tests pass
echo "Test: Unit tests pass"
run_suite "$LOG_PLAIN"
plain_rc=$R_RC; plain_pass=$R_PASS; plain_skip=$R_SKIP; plain_fail=$R_FAIL
cat "$LOG_PLAIN"
executed=$((plain_pass + plain_skip + plain_fail))
if [ "$plain_rc" -ne 0 ] || [ "$plain_fail" -gt 0 ]; then
    fail "Unit tests failed (${plain_fail} failing test function(s), go test rc=${plain_rc})"
else
    pass "Unit tests pass (${plain_pass} test function(s) executed and passed)"
fi

# Test 4: No race conditions
echo "Test: Race detector clean"
run_suite "$LOG_RACE" -race
race_rc=$R_RC; race_pass=$R_PASS; race_skip=$R_SKIP; race_fail=$R_FAIL
if [ "$race_rc" -ne 0 ] || [ "$race_fail" -gt 0 ]; then
    fail "Race conditions detected (${race_fail} failing test function(s), go test rc=${race_rc})"
else
    pass "No race conditions detected (${race_pass} test function(s) executed under -race)"
fi

# Test 5: COVERAGE — the gate must not report a bare PASS for a population it
# never ran. This is the assertion whose absence let a -short run read as a
# pass over five untouched packages.
echo "Test: Every declared test function actually executed"
echo "  declared=${declared} executed=${executed} passed=${plain_pass} skipped=${plain_skip} failed=${plain_fail}"
if [ "${plain_skip}" -gt 0 ]; then
    echo "    packages that self-skipped:"
    skipped_packages "$LOG_PLAIN"
fi
if [ "$executed" -ne "$declared" ]; then
    undet "executed ${executed} != declared ${declared} — some test functions never ran at all; coverage of this module is NOT established"
elif [ "${plain_skip}" -gt 0 ]; then
    undet "${plain_skip} of ${declared} declared test function(s) self-skipped — the populations named above were NOT covered by this run"
elif [ "${race_skip}" -gt 0 ]; then
    undet "${race_skip} test function(s) self-skipped under -race"
else
    pass "all ${declared} declared test function(s) executed (0 skipped)"
fi

if [ "${ALLOW_SHORT}" = "1" ]; then
    echo "Test: Coverage mode is not capped"
    undet "RAG_UNIT_ALLOW_SHORT=1 — this run used -short, which cannot cover tests/{e2e,integration,security,stress}; a -short run is never a PASS"
fi

echo ""
echo "=== Results: ${PASS}/${TOTAL} passed, ${FAIL} failed, ${UNDET} undetermined ==="
if [ "${FAIL}" -gt 0 ]; then
    echo "=== VERDICT: FAIL (real finding) ==="
    exit 1
elif [ "${UNDET}" -gt 0 ]; then
    echo "=== VERDICT: UNDETERMINED — NOT a pass ==="
    exit 2
fi
echo "=== VERDICT: PASS ==="
echo "  evidence: ${declared} declared / ${executed} executed / ${plain_pass} passed / 0 skipped;"
echo "            race: ${race_pass} passed / ${race_skip} skipped; GOMAXPROCS=${GOMAXPROCS_VAL} -p ${GOTEST_P}"
exit 0
