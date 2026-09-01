#!/usr/bin/env bash
# rag_unit_challenge.sh - Validates RAG module unit tests
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MODULE_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
MODULE_NAME="RAG"

PASS=0
FAIL=0
TOTAL=0

pass() { PASS=$((PASS+1)); TOTAL=$((TOTAL+1)); echo "  PASS: $1"; }
fail() { FAIL=$((FAIL+1)); TOTAL=$((TOTAL+1)); echo "  FAIL: $1"; }

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
    pkg_name=$(basename "$pkg_dir")
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
GOMAXPROCS="${GOMAXPROCS:-2}"
GOTEST_P="${GOTEST_P:-1}"
GOTEST_NICE="${GOTEST_NICE:-19}"
export GOMAXPROCS

# Test 3: Unit tests pass
echo "Test: Unit tests pass"
if (cd "${MODULE_DIR}" && nice -n "${GOTEST_NICE}" go test -short -count=1 -p "${GOTEST_P}" ./... 2>&1); then
    pass "Unit tests pass"
else
    fail "Unit tests failed"
fi

# Test 4: No race conditions (short mode)
echo "Test: Race detector clean"
if (cd "${MODULE_DIR}" && nice -n "${GOTEST_NICE}" go test -short -race -count=1 -p "${GOTEST_P}" ./... 2>&1); then
    pass "No race conditions detected"
else
    fail "Race conditions detected"
fi

echo ""
echo "=== Results: ${PASS}/${TOTAL} passed, ${FAIL} failed ==="
[ "${FAIL}" -eq 0 ] && exit 0 || exit 1
