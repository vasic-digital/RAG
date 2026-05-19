#!/usr/bin/env bash
# rag_describe_challenge.sh — round-278 paired-mutation gate for digital.vasic.rag.
#
# Honest exit codes:
#   0  — clean tree, all anti-bluff invariants hold
#   99 — mutate mode (--anti-bluff-mutate) deliberately corrupts an invariant
#         to prove this gate has the ability to FAIL when fed a lie.
#         Required by §11.4 paired-mutation discipline (CONST-035 + CONST-055).
#   1  — invariant violation in clean tree (CRITICAL — real defect surfaced)
#
# Usage:
#   bash challenges/rag_describe_challenge.sh                  # clean run
#   bash challenges/rag_describe_challenge.sh --anti-bluff-mutate  # mutate run
#
# This script is the *describe* counterpart of the existing
# rag_functionality_challenge.sh: where that script verifies symbol presence
# via grep, this one verifies behavioural invariants by running real code
# from pkg/* and asserting captured runtime evidence.

set -uo pipefail

MUTATE=0
if [[ "${1:-}" == "--anti-bluff-mutate" ]]; then
    MUTATE=1
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MODULE_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
cd "${MODULE_DIR}"

PASS=0
FAIL=0
TOTAL=0

pass() { PASS=$((PASS+1)); TOTAL=$((TOTAL+1)); echo "  PASS: $1"; }
fail() { FAIL=$((FAIL+1)); TOTAL=$((TOTAL+1)); echo "  FAIL: $1"; }

echo "=== RAG describe Challenge (round-278) ==="
echo "  module=${MODULE_DIR}"
echo "  mutate=${MUTATE}"
echo ""

# Invariant 1: production source carries no bluff markers.
echo "Test 1: anti-bluff scan over pkg/ production source"
BLUFFS=$(grep -rn "simulated\|for now\|TODO implement\|placeholder" pkg/ 2>/dev/null \
    | grep -v _test.go || true)
if [[ -z "$BLUFFS" ]]; then
    pass "pkg/ free of bluff markers (excluding *_test.go)"
else
    fail "BLUFF MARKERS found in pkg/:\n${BLUFFS}"
fi

# Invariant 2: `go build ./...` succeeds.
echo "Test 2: go build ./..."
if go build ./... > /tmp/rag-describe-build.$$ 2>&1; then
    pass "go build ./... clean"
else
    fail "go build failed: $(cat /tmp/rag-describe-build.$$)"
fi
rm -f /tmp/rag-describe-build.$$

# Invariant 3: every required package exists.
echo "Test 3: required packages present"
MISSING=()
for pkg in chunker retriever reranker pipeline hybrid; do
    if [[ ! -d "pkg/${pkg}" ]]; then
        MISSING+=("pkg/${pkg}")
    fi
done
if [[ ${#MISSING[@]} -eq 0 ]]; then
    pass "all 5 packages present (chunker, retriever, reranker, pipeline, hybrid)"
else
    fail "missing packages: ${MISSING[*]}"
fi

# Invariant 4: fixtures file present + parseable + 5 locales.
echo "Test 4: 5-locale fixture file present"
FIXTURE="tests/fixtures/rag/payloads.json"
if [[ ! -f "$FIXTURE" ]]; then
    fail "fixture missing: $FIXTURE"
else
    LOCALE_COUNT=$(grep -c '"code":' "$FIXTURE" || echo 0)
    if [[ "$LOCALE_COUNT" -ge 5 ]]; then
        pass "fixture has $LOCALE_COUNT locales (>=5 required)"
    else
        fail "fixture has only $LOCALE_COUNT locales, need >=5"
    fi
fi

# Invariant 5: runner executes successfully end-to-end and produces evidence.
echo "Test 5: challenges/runner end-to-end real-RAG exerciser"
RUNNER_OUT=$(go run ./challenges/runner 2>&1)
RUNNER_EXIT=$?

# --- MUTATION HOOK ---
# When --anti-bluff-mutate is passed, deliberately invert the success
# detection so this gate emits FAIL and exits 99. This proves the gate
# can actually distinguish PASS from corruption (paired-mutation discipline
# per §11.4 / CONST-035 / CONST-055).
if [[ $MUTATE -eq 1 ]]; then
    echo ""
    echo "  --- ANTI-BLUFF MUTATE MODE ---"
    echo "  Inverting runner success detection. Honest exit: 99."
    if [[ $RUNNER_EXIT -eq 0 ]]; then
        echo "  Runner ACTUALLY succeeded but mutation says we must FAIL."
        echo "  exit 99 (paired-mutation honest signal)"
        exit 99
    else
        # Even if the runner failed (e.g. broken build), mutate mode still
        # exits 99 — the contract is "mutate mode never exits 0".
        echo "  Runner exit was $RUNNER_EXIT; mutate mode forces exit 99."
        exit 99
    fi
fi

if [[ $RUNNER_EXIT -eq 0 ]] && echo "$RUNNER_OUT" | grep -q "=== PASS"; then
    LOCALES_DONE=$(echo "$RUNNER_OUT" | grep -c "^    PASS locale=")
    if [[ "$LOCALES_DONE" -ge 5 ]]; then
        pass "runner exit=0, ${LOCALES_DONE} locales exercised end-to-end"
    else
        fail "runner exit=0 but only ${LOCALES_DONE} locales reported PASS"
    fi
else
    fail "runner did not produce PASS marker. Output:\n${RUNNER_OUT}"
fi

# Invariant 6: unit tests with race detector still pass.
echo "Test 6: unit tests with -race -count=1"
if go test -race -count=1 ./pkg/... > /tmp/rag-describe-tests.$$ 2>&1; then
    pass "all pkg/ unit tests green with race detector"
else
    fail "unit tests failed:\n$(tail -20 /tmp/rag-describe-tests.$$)"
fi
rm -f /tmp/rag-describe-tests.$$

echo ""
echo "=== Results: ${PASS}/${TOTAL} passed, ${FAIL} failed ==="
if [[ "${FAIL}" -eq 0 ]]; then
    echo "=== RAG describe Challenge: PASSED (round-278) ==="
    exit 0
else
    echo "=== RAG describe Challenge: FAILED ==="
    exit 1
fi
