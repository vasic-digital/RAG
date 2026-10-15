#!/usr/bin/env bash
# rag_functionality_challenge.sh - Validates RAG module core functionality and structure
#
# Usage:
#   bash challenges/scripts/rag_functionality_challenge.sh                   # validate this module
#   bash challenges/scripts/rag_functionality_challenge.sh --prove-failure   # §1.1 paired proof
#
# THREE-VALUED, and a 2 is never a pass:
#   0 = every assertion held, and exactly FUNC_FLOOR assertions ran
#   1 = a real finding — an assertion failed, or the number of assertions
#       that ran differs from FUNC_FLOOR (fewer: an assertion lost to an edit
#       or an early exit — the count is taken by an EXIT trap, so `exit 0`
#       mid-file cannot skip it; more: an assertion added without raising
#       FUNC_FLOOR)
#   2 = COULD NOT DETERMINE — part of the source could not be read (grep
#       could not open a file or directory), or an unknown argument.
#       Precedence: 1 outranks 2 outranks 0.
#
# EXECUTED-COUNT FLOOR (VSC-371): FUNC_FLOOR is the number of assertions this
# file makes (10, T01..T10). Every assertion ends in exactly one `verdict`
# call, which is what is counted; a run that reaches the end having made
# fewer — an assertion deleted or commented out, a block skipped — is rc 1,
# never a quiet pass over a smaller check. Adding an assertion means raising
# FUNC_FLOOR in the same commit (the count is exact, see finish()); there is
# no environment override. This file checks that the core types are DECLARED
# (structure); behaviour is exercised by challenges/rag_describe_challenge.sh.
#
# --prove-failure (§1.1 paired proof, VSC-371): copies this module (no .git)
# into throw-away trees under $TMPDIR, plants a defect in each copy and runs
# THIS file there — a clean copy must exit 0; a renamed Chunker interface, a
# changed Retrieve signature, a copy of this file with one assertion deleted,
# one with an `exit 0` planted mid-file, one with an extra assertion and a
# pkg/pipeline reduced to a comment saying "stage" must exit 1; an unreadable
# package directory and an unknown argument must exit 2. Exits 0 only when every case holds, 1 when any does
# not, 2 when the proof itself cannot run (e.g. as root, where chmod 000 does
# not deny a read).

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MODULE_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
MODULE_NAME="RAG"

# Executed-count floor — see the header. Assertions (verdict calls).
FUNC_FLOOR=10

# ── paired proof ─────────────────────────────────────────────────────────────
prove_failure() {
    # T is deliberately NOT local: the EXIT trap runs after this function has
    # returned, and a local T would be empty by then and leak the scratch tree.
    local self_rel c0 d
    self_rel="challenges/scripts/$(basename "${BASH_SOURCE[0]}")"
    T=$(mktemp -d) || { echo "prove-failure: cannot determine — mktemp -d failed"; return 2; }
    trap 'chmod -R u+rwx "$T" 2>/dev/null; rm -rf "$T"' EXIT
    P_PASS=0; P_FAIL=0; P_UNDET=0

    # mkcopy <name> -> path of a fresh copy of this module's working tree (no .git)
    mkcopy() {
        local dst="$T/$1"
        mkdir -p "$dst" || return 1
        ( cd "$MODULE_DIR" && tar --exclude=.git -cf - . ) | ( cd "$dst" && tar -xf - ) || return 1
        [ -f "$dst/$self_rel" ] || return 1
        printf '%s\n' "$dst"
    }
    # expect <name> <want-rc> <copy> <needle> [VAR=value ...] [-- script args...]
    # Runs the copy's own instance of THIS file; the rc must equal <want-rc>
    # and the output must contain <needle>, so an rc reached for an unrelated
    # reason does not count as the planted defect being caught.
    expect() {
        local name=$1 want=$2 dir=$3 needle=$4 out rc
        shift 4
        local envs=() args=()
        while [ $# -gt 0 ]; do
            if [ "$1" = "--" ]; then shift; args=("$@"); break; fi
            envs+=("$1"); shift
        done
        out=$(cd "$dir" && env "${envs[@]}" bash "$dir/$self_rel" "${args[@]}" 2>&1); rc=$?
        if [ "$rc" -ne "$want" ]; then
            P_FAIL=$((P_FAIL + 1)); echo "  FAIL $name: want rc $want, got rc $rc"
            printf '%s\n' "$out" | tail -8 | sed 's/^/       | /'
        elif ! grep -qF -- "$needle" <<<"$out"; then
            P_FAIL=$((P_FAIL + 1)); echo "  FAIL $name: rc $rc as wanted, but the output does not name '$needle'"
            printf '%s\n' "$out" | tail -8 | sed 's/^/       | /'
        else
            P_PASS=$((P_PASS + 1)); echo "  PASS $name (rc $rc)"
        fi
    }
    cannot() { echo "prove-failure: cannot determine — $1"; }

    echo "prove-failure: ${MODULE_NAME} functionality challenge ${MODULE_DIR}/${self_rel} (floor ${FUNC_FLOOR} assertions)"
    c0=$(mkcopy c0) || { cannot "could not copy the module"; return 2; }
    expect "C0 clean copy of this module" 0 "$c0" "VERDICT: PASS"

    d=$(mkcopy m1) || { cannot "could not copy the module"; return 2; }
    grep -rlZ 'type Chunker interface' "$d/pkg/chunker" | xargs -0 -r sed -i 's/type Chunker interface/type ChunkerRenamed interface/'
    expect "M1 the Chunker interface is renamed away" 1 "$d" "Chunker interface not found"

    d=$(mkcopy m2) || { cannot "could not copy the module"; return 2; }
    grep -rlZ 'Retrieve(ctx context.Context, query string' "$d/pkg/retriever" | xargs -0 -r sed -i 's/Retrieve(ctx context.Context, query string/Retrieve(ctx context.Context, q string/'
    expect "M2 the Retrieve(ctx, query, ...) signature changes" 1 "$d" "declares no Retrieve"

    d=$(mkcopy m3) || { cannot "could not copy the module"; return 2; }
    if grep -q '^verdict T10 ' "$d/$self_rel" && sed -i '/^verdict T10 /d' "$d/$self_rel" \
       && ! grep -q '^verdict T10 ' "$d/$self_rel"; then
        expect "M3 one assertion deleted from this file, floor left as is" 1 "$d" "below the floor"
    else
        P_UNDET=$((P_UNDET + 1)); echo "  UNDET M3: could not delete assertion T10 from the copy of this file"
    fi

    if [ "$(id -u)" -eq 0 ]; then
        P_UNDET=$((P_UNDET + 1)); echo "  UNDET U1: running as root — chmod 000 does not deny reads here"
    else
        d=$(mkcopy u1) || { cannot "could not copy the module"; return 2; }
        chmod 000 "$d/pkg/retriever"
        expect "U1 an unreadable package directory is not a pass" 2 "$d" "UNDETERMINED"
        chmod 755 "$d/pkg/retriever"
    fi
    expect "U2 unknown argument" 2 "$c0" "unknown argument" -- --no-such-flag


    # M4: a planted `exit 0` after assertion T05 — an early exit is not a pass.
    d=$(mkcopy m4) || { cannot "could not copy the module"; return 2; }
    if grep -q '^verdict T05 ' "$d/$self_rel" && sed -i '/^verdict T05 /a exit 0' "$d/$self_rel"; then
        expect "M4 a planted exit 0 after T05 (early exit)" 1 "$d" "below the floor"
    else
        P_UNDET=$((P_UNDET + 1)); echo "  UNDET M4: could not plant exit 0 in the copy of this file"
    fi

    # M5: one assertion more than FUNC_FLOOR records — the floor is an exact count.
    d=$(mkcopy m5) || { cannot "could not copy the module"; return 2; }
    if grep -q '^verdict T10 ' "$d/$self_rel" && sed -i '/^verdict T10 /p' "$d/$self_rel"; then
        expect "M5 an extra assertion without raising FUNC_FLOOR" 1 "$d" "raise FUNC_FLOOR"
    else
        P_UNDET=$((P_UNDET + 1)); echo "  UNDET M5: could not duplicate assertion T10 in the copy of this file"
    fi

    # M6: pkg/pipeline reduced to a package clause and a comment saying "stage".
    d=$(mkcopy m6) || { cannot "could not copy the module"; return 2; }
    find "$d/pkg/pipeline" -name '*.go' ! -name '*_test.go' -delete
    printf 'package pipeline\n\n// stage\n' > "$d/pkg/pipeline/doc.go"
    expect "M6 pkg/pipeline holds only a comment saying 'stage'" 1 "$d" "pkg/pipeline lacks"
    echo "prove-failure: ${P_PASS} passed, ${P_FAIL} failed, ${P_UNDET} undetermined"
    [ "$P_FAIL" -gt 0 ] && return 1
    [ "$P_UNDET" -gt 0 ] && return 2
    return 0
}

case "${1:-}" in
    "") ;;
    --prove-failure)
        [ $# -eq 1 ] || { echo "ERROR: --prove-failure takes no further argument" >&2; exit 2; }
        prove_failure; exit $? ;;
    -h|--help) sed -n '2,/^$/p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0 ;;
    *) echo "ERROR: unknown argument: $1 — this challenge validates ${MODULE_DIR} and takes no target" >&2
       echo "=== VERDICT: UNDETERMINED — NOT a pass ===" >&2
       exit 2 ;;
esac

PASS=0
FAIL=0
UNDET=0
TOTAL=0
EXECUTED=0

pass()  { PASS=$((PASS+1));  TOTAL=$((TOTAL+1)); echo "  PASS: $1"; }
fail()  { FAIL=$((FAIL+1));  TOTAL=$((TOTAL+1)); echo "  FAIL: $1"; }
undet() { UNDET=$((UNDET+1)); TOTAL=$((TOTAL+1)); echo "  UNDETERMINED: $1"; }

# SINGLE EXIT PATH (VSC-371 review): the floor and the verdict are evaluated
# from this EXIT trap, not at the end of the file, so an `exit 0` anywhere
# below, an unbound variable or any other early exit still reaches the count
# — and an early exit is, by construction, a run that made fewer assertions
# than FUNC_FLOOR (rc 1). The floor is an EXACT count: an assertion added
# without raising FUNC_FLOOR is rc 1 too, so the recorded number cannot drift
# from the file.
finish() {
    trap - EXIT
    echo ""
    echo "Test: Every assertion ran (exactly ${FUNC_FLOOR})"
    if [ "$EXECUTED" -lt "$FUNC_FLOOR" ]; then
        fail "executed ${EXECUTED} assertion(s), below the floor ${FUNC_FLOOR} — a partial run (an early exit, a deleted assertion) is not a pass"
    elif [ "$EXECUTED" -gt "$FUNC_FLOOR" ]; then
        fail "executed ${EXECUTED} assertion(s), more than FUNC_FLOOR=${FUNC_FLOOR} records — raise FUNC_FLOOR in the commit that adds an assertion"
    else
        pass "executed ${EXECUTED} assertion(s) (exactly FUNC_FLOOR=${FUNC_FLOOR})"
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
    exit 0
}
trap finish EXIT

# probe <path> <grep flags and -e pattern...>
#   0 match, 1 no match (a missing path is "no match": a real finding),
#   2 could not read — grep hit an unreadable file or directory and found no
#   match, so "absent" cannot be told apart from "not looked at".
probe() {
    local path=$1; shift
    [ -e "$path" ] || return 1
    grep "$@" "$path" 2>/dev/null
    local rc=$?
    [ "$rc" -le 1 ] && return "$rc"
    return 2
}

# verdict <id> <rc> <pass-message> <fail-message> — ONE assertion, counted once
verdict() {
    EXECUTED=$((EXECUTED+1))
    case "$2" in
        0) pass "$3" ;;
        1) fail "$4" ;;
        *) undet "$1: part of the source could not be read — '$4' cannot be ruled out" ;;
    esac
}

# worst <rc...>: 1 if any is 1, else 2 if any is 2, else 0
worst() { local r w=0; for r in "$@"; do [ "$r" -eq 1 ] && return 1; [ "$r" -ge 2 ] && w=2; done; return "$w"; }

echo "=== ${MODULE_NAME} Functionality Challenge ==="
echo ""

# Test 1: Required packages exist
echo "Test: Required packages exist"
missing=""
for pkg in chunker retriever reranker pipeline hybrid; do
    [ -d "${MODULE_DIR}/pkg/${pkg}" ] || missing="${missing} pkg/${pkg}"
done
if [ -z "$missing" ]; then rc=0; else rc=1; fi
verdict T01 "$rc" "All required packages present (chunker, retriever, reranker, pipeline, hybrid)" "Missing package(s):${missing}"

# Test 2: Chunker interface is defined
echo "Test: Chunker interface is defined"
probe "${MODULE_DIR}/pkg/chunker/" -rq -e "type Chunker interface"; rc=$?
verdict T02 "$rc" "Chunker interface is defined in pkg/chunker" "Chunker interface not found in pkg/chunker"

# Test 3: Chunk struct is defined
echo "Test: Chunk struct is defined"
probe "${MODULE_DIR}/pkg/chunker/" -rq -e "type Chunk struct"; rc=$?
verdict T03 "$rc" "Chunk struct is defined in pkg/chunker" "Chunk struct not found in pkg/chunker"

# Test 4: Retriever interface is defined
echo "Test: Retriever interface is defined"
probe "${MODULE_DIR}/pkg/retriever/" -rq -e "type Retriever interface"; rc=$?
verdict T04 "$rc" "Retriever interface is defined in pkg/retriever" "Retriever interface not found in pkg/retriever"

# Test 5: Reranker interface is defined
echo "Test: Reranker interface is defined"
probe "${MODULE_DIR}/pkg/reranker/" -rq -e "type Reranker interface"; rc=$?
verdict T05 "$rc" "Reranker interface is defined in pkg/reranker" "Reranker interface not found in pkg/reranker"

# Test 6: Document struct is defined
echo "Test: Document struct is defined"
probe "${MODULE_DIR}/pkg/" -rq -e "type Document struct"; rc=$?
verdict T06 "$rc" "Document struct found" "Document struct not found"

# Test 7: Multiple chunker implementations
echo "Test: Multiple chunker implementations exist"
impls=$(probe "${MODULE_DIR}/pkg/chunker/" -rh -e "type\s\+\w*Chunker\s\+struct"); rc=$?
impl_count=0
[ "$rc" -eq 0 ] && impl_count=$(printf '%s\n' "$impls" | grep -c .)
if [ "$impl_count" -ge 2 ]; then
    msg="Multiple chunker implementations found (${impl_count})"
else
    msg="At least one chunker implementation found"
fi
verdict T07 "$rc" "$msg" "No chunker implementations found"

# Test 8: Pipeline support exists
# The pattern here was "Pipeline\|Stage\|stage" against pkg/pipeline/: a
# one-line comment containing the word "stage" satisfied it (VSC-371 review).
# Now: the Pipeline type and the Stage interface it composes must exist.
echo "Test: Pipeline support exists"
probe "${MODULE_DIR}/pkg/pipeline/" -rqE -e "^type +Pipeline +struct"; r1=$?
probe "${MODULE_DIR}/pkg/pipeline/" -rqE -e "^type +Stage +interface"; r2=$?
worst "$r1" "$r2"; rc=$?
verdict T08 "$rc" "Pipeline support found in pkg/pipeline (Pipeline struct + Stage interface)" "pkg/pipeline lacks a Pipeline struct and/or a Stage interface"

# Test 9: Hybrid search support
# The pattern here was "Hybrid\|hybrid\|MultiRetriever" against pkg/hybrid/.
# The lowercase alternative matches the PACKAGE CLAUSE of every file in that
# directory, so a file whose entire content is `package hybrid` satisfied it.
# Measured 2026-09-06: that exact one-line file passes the old assertion. It
# could not fail while the directory existed, so it asserted nothing.
# Now: a real fusion strategy and a real hybrid retriever type must exist.
echo "Test: Hybrid search support exists"
probe "${MODULE_DIR}/pkg/hybrid/" -rqE -e "^type +HybridRetriever +struct"; r1=$?
probe "${MODULE_DIR}/pkg/hybrid/" -rqE -e "^type +FusionStrategy +interface"; r2=$?
worst "$r1" "$r2"; rc=$?
verdict T09 "$rc" "Hybrid search support found in pkg/hybrid (HybridRetriever + FusionStrategy)" "pkg/hybrid lacks a HybridRetriever struct and/or a FusionStrategy interface"

# Test 10: Search/Query capability
# The pattern here was "Search\|Query\|Retrieve" against pkg/retriever/. The
# identifier `Retriever` — whose existence Test 4 already asserts — contains
# the substring "Retrieve", so Test 10 could not fail whenever Test 4 passed.
# Measured 2026-09-06: a file containing only `package retriever` plus
# `type Retriever interface{}` satisfied the old assertion.
# Now: the interface must actually declare a Retrieve method with a context
# and a query, which is the capability the test claims to check.
echo "Test: Search/Query capability exists"
probe "${MODULE_DIR}/pkg/retriever/" -rqE -e "Retrieve\(ctx context\.Context, query string"; rc=$?
verdict T10 "$rc" "Search/Query capability found in pkg/retriever (Retrieve(ctx, query, ...))" "pkg/retriever declares no Retrieve(ctx context.Context, query string, ...) method"

# The floor and the verdict are evaluated by finish() (EXIT trap above).
