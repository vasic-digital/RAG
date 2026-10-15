#!/usr/bin/env bash
# rag_unit_challenge.sh - Validates RAG module unit tests
#
# Usage:
#   bash challenges/scripts/rag_unit_challenge.sh                   # validate this module
#   bash challenges/scripts/rag_unit_challenge.sh --prove-failure   # §1.1 paired proof
#
# THREE-VALUED, and a 2 is never a pass:
#   0 = every declared test function EXECUTED and passed, and the executed
#       count reached the recorded floor (UNIT_FLOOR) in both runs
#   1 = a real finding — a test failed, a structural assertion failed, or a
#       run REPORTED fewer test functions than the floor (0 included)
#   2 = COULD NOT DETERMINE — the run did not cover every declared test
#       function (self-skipped tests, an explicit -short run, or
#       executed != declared), or the executed count could not be read
#       (no go toolchain, no scratch file, a runner that reported success
#       but printed no package result), or an unknown argument.
#       Precedence: 1 outranks 2 outranks 0.
#
# EXECUTED-COUNT FLOOR (VSC-371): UNIT_FLOOR is the number of top-level test
# functions this module declares and runs today (measured 2026-10-15: 151
# declared / 151 executed / 151 passed in full mode, and 151 under -race).
# A run that executes fewer — a deleted test file, a build constraint that
# excludes a package's tests, a runner that ran nothing — is rc 1, even when
# executed == declared, because declared shrinks with the source. Adding tests
# needs no edit here; REMOVING tests on purpose means lowering UNIT_FLOOR in
# the same commit, which is a reviewable act rather than a silent one. There
# is deliberately no environment override that lowers it. The floor is a
# MINIMUM, not an exact count: the exact population of every run is already
# pinned by executed == declared (both read fresh each run), so an exact floor
# would add nothing but an edit to this file for every new test.
#
# Counting rules: TestMain is not a test function and is not declared;
# runnable Example functions are counted on neither side; a package result
# replayed from the test cache `(cached)` is UNDETERMINED, never a pass (the
# runs use -count=1, so this only fires if that flag is lost).
#
# --prove-failure (§1.1 paired proof, VSC-371): copies this module (no .git)
# into throw-away trees under $TMPDIR, plants a defect in each copy and runs
# THIS file there — a clean copy, and copies with a TestMain or a runnable
# Example added to pkg/chunker, must exit 0; a planted failing test, a copy
# that keeps only one package's tests and a copy whose tests are all excluded
# by a build constraint must exit 1; a runner that prints no result, an
# unusable toolchain, an unusable TMPDIR, an explicit -short run and an
# unknown argument must exit 2 — and so must a copy of this file with
# -count=1 deleted, re-run so the test cache replays. Exits 0 only when every case holds, 1 when
# any does not, 2 when the proof itself cannot run.
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

# Executed-count floor — see the header. Per run (plain and -race).
UNIT_FLOOR=151

# ── paired proof ─────────────────────────────────────────────────────────────
prove_failure() {
    # T is deliberately NOT local: the EXIT trap runs after this function has
    # returned, and a local T would be empty by then and leak the scratch tree.
    local self_rel real_go c0 d
    self_rel="challenges/scripts/$(basename "${BASH_SOURCE[0]}")"
    real_go=$(command -v go) || { echo "prove-failure: cannot determine — no go toolchain on PATH"; return 2; }
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
    cannot() { echo "prove-failure: cannot determine — $1"; return 2; }

    mkdir -p "$T/shim-silent" "$T/shim-broken" || { cannot "cannot create shim dirs"; return 2; }
    # a runner that reports success for `go test` but prints nothing
    printf '#!/usr/bin/env bash\ncase "${1:-}" in test) exit 0 ;; esac\nexec %q "$@"\n' "$real_go" > "$T/shim-silent/go"
    # a toolchain that cannot run at all
    printf '#!/usr/bin/env bash\necho "go: toolchain unusable (prove-failure shim)" >&2\nexit 1\n' > "$T/shim-broken/go"
    chmod +x "$T/shim-silent/go" "$T/shim-broken/go"

    echo "prove-failure: ${MODULE_NAME} unit challenge ${MODULE_DIR}/${self_rel} (floor ${UNIT_FLOOR})"
    c0=$(mkcopy c0) || { cannot "could not copy the module"; return 2; }
    expect "C0 clean copy of this module" 0 "$c0" "VERDICT: PASS"

    d=$(mkcopy m1) || { cannot "could not copy the module"; return 2; }
    printf 'package chunker\n\nimport "testing"\n\nfunc TestPlantedFailure(t *testing.T) { t.Fatal("planted by --prove-failure") }\n' \
        > "$d/pkg/chunker/zz_prove_failure_test.go"
    expect "M1 a planted failing unit test" 1 "$d" "TestPlantedFailure"

    d=$(mkcopy m2) || { cannot "could not copy the module"; return 2; }
    find "$d/tests" -name '*_test.go' -delete
    expect "M2 far fewer tests (every tests/ suite deleted; declared == executed, pkg/ unchanged)" 1 "$d" "below the floor"

    d=$(mkcopy m3) || { cannot "could not copy the module"; return 2; }
    local f
    while IFS= read -r -d '' f; do
        if head -1 "$f" | grep -q '^//go:build'; then
            sed -i '1s#.*#//go:build prove_failure_never#' "$f"
        else
            { printf '//go:build prove_failure_never\n\n'; cat "$f"; } > "$f.prove" && mv "$f.prove" "$f"
        fi
    done < <(find "$d" -name '*_test.go' -print0)
    expect "M3 zero executed (every test file excluded by a build constraint)" 1 "$d" "below the floor"

    expect "U1 runner reports success but prints no package result" 2 "$c0" "UNDETERMINED" "PATH=$T/shim-silent:$PATH"
    expect "U2 go toolchain unusable" 2 "$c0" "UNDETERMINED" "PATH=$T/shim-broken:$PATH"
    expect "U3 scratch directory unusable (TMPDIR)" 2 "$c0" "UNDETERMINED" "TMPDIR=$T/no-such-dir"
    expect "U4 explicit -short run" 2 "$c0" "UNDETERMINED" "RAG_UNIT_ALLOW_SHORT=1"
    expect "U5 unknown argument" 2 "$c0" "unknown argument" -- --no-such-flag


    # C1/C2: TestMain is not a test function (never reported) and a runnable
    # Example IS reported — neither may make a clean module UNDETERMINED.
    d=$(mkcopy c1) || { cannot "could not copy the module"; return 2; }
    printf 'package chunker\n\nimport (\n\t"os"\n\t"testing"\n)\n\nfunc TestMain(m *testing.M) { os.Exit(m.Run()) }\n' \
        > "$d/pkg/chunker/zz_prove_main_test.go"
    expect "C1 a TestMain in pkg/chunker is not counted as a test function" 0 "$d" "VERDICT: PASS"

    d=$(mkcopy c2) || { cannot "could not copy the module"; return 2; }
    printf 'package chunker\n\nimport "fmt"\n\nfunc Example_proveFailure() {\n\tfmt.Println("ok")\n\t// Output: ok\n}\n' \
        > "$d/pkg/chunker/zz_prove_example_test.go"
    expect "C2 a runnable Example in pkg/chunker is not counted as a test function" 0 "$d" "VERDICT: PASS"

    # M4: -count=1 deleted from this file: a re-run replays the test cache,
    # which is not a fresh execution.
    d=$(mkcopy m4) || { cannot "could not copy the module"; return 2; }
    if grep -q '^ *go test -count=1 -p' "$d/$self_rel" && sed -i 's/^\( *go test\) -count=1 -p/\1 -p/' "$d/$self_rel" \
       && ! grep -q '^ *go test -count=1 -p' "$d/$self_rel"; then
        ( cd "$d" && bash "$d/$self_rel" >/dev/null 2>&1 )   # warm the test cache
        expect "M4 -count=1 deleted from this file, re-run replays cached results" 2 "$d" "cached"
    else
        P_UNDET=$((P_UNDET + 1)); echo "  UNDET M4: could not delete -count=1 from the copy of this file"
    fi
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

pass()  { PASS=$((PASS+1));  TOTAL=$((TOTAL+1)); echo "  PASS: $1"; }
fail()  { FAIL=$((FAIL+1));  TOTAL=$((TOTAL+1)); echo "  FAIL: $1"; }
undet() { UNDET=$((UNDET+1)); TOTAL=$((TOTAL+1)); echo "  UNDETERMINED: $1"; }

finish() {
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
    echo "  evidence: ${declared} declared / ${executed} executed / ${plain_pass} passed / 0 skipped (floor ${UNIT_FLOOR});"
    echo "            race: ${race_pass} passed / ${race_skip} skipped; GOMAXPROCS=${GOMAXPROCS_VAL} -p ${GOTEST_P}"
    exit 0
}

echo "=== ${MODULE_NAME} Unit Test Challenge ==="
echo ""

# Test 0: a toolchain that can answer at all. Without one nothing below can
# be measured, and "could not run" must not read as "tests failed".
echo "Test: go toolchain is usable"
if go_version=$(go version 2>&1); then
    pass "${go_version}"
else
    undet "go toolchain unusable — the executed count cannot be measured (${go_version:-no output})"
    finish
fi

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
# Fuzz targets are counted too: `go test` runs their seed corpus and reports
# them as top-level `--- PASS: FuzzX` lines. TestMain is NOT a test function
# (the runner never reports it) and is excluded; Example functions are not
# counted on either side (see run_suite), so a runnable Example cannot make a
# clean module permanently UNDETERMINED.
declared_raw=$(grep -rhE '^func (Test|Fuzz)[A-Z_]' --include='*_test.go' "${MODULE_DIR}"); declared_rc=$?
if [ "$declared_rc" -ge 2 ]; then
    declared=0
    undet "the declared test population could not be read from source (grep rc ${declared_rc})"
else
    declared=$(printf '%s\n' "$declared_raw" | grep -vE '^func TestMain\(' | grep -c '^func ')
fi

echo ""
echo "  budget: GOMAXPROCS=${GOMAXPROCS_VAL} -p ${GOTEST_P} nice ${GOTEST_NICE}"
echo "  mode:   $([ "${ALLOW_SHORT}" = "1" ] && echo '-short (verdict capped at UNDETERMINED)' || echo 'full')"
echo "  declared test functions (from source): ${declared}"
echo "  executed-count floor (per run): ${UNIT_FLOOR}"
echo ""

# run_suite <logfile> [extra go-test flags...]
# Emits nothing; sets R_PASS/R_SKIP/R_FAIL/R_PKGS/R_RC and writes the log.
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
    # 143 PASS / 0 SKIP / 0 FAIL in full mode (2026-09-06; 151 / 0 / 0 on
    # 2026-10-15).
    # Only Test/Fuzz functions are counted, matching the declared side: a
    # runnable Example prints `--- PASS: ExampleX` too, but is not declared
    # (a failing Example still fails the run through go test's rc).
    R_PASS=$(grep -cE '^--- PASS: (Test|Fuzz)' "$log")
    R_SKIP=$(grep -cE '^--- SKIP: (Test|Fuzz)' "$log")
    R_FAIL=$(grep -cE '^--- FAIL: (Test|Fuzz)' "$log")
    # `(cached)` package results are REPLAYED output, not an execution. With
    # -count=1 they cannot occur; this guards the flag's removal.
    R_CACHED=$(grep -cE '^ok[[:space:]].*\(cached\)' "$log")
    # Per-package result lines (`ok`, `FAIL`, `?  ... [no test files]`). A
    # run with none of them has told us nothing about what it executed: the
    # count is UNREADABLE, which is not the same thing as a count of zero.
    R_PKGS=$(grep -cE '^(ok|FAIL|\?)[[:space:]]+[^[:space:]]+' "$log")
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

if ! LOG_PLAIN="$(mktemp 2>/dev/null)" || ! LOG_RACE="$(mktemp 2>/dev/null)"; then
    rm -f "${LOG_PLAIN:-}"
    undet "cannot create a scratch log under TMPDIR=${TMPDIR:-/tmp} — the executed count cannot be measured"
    finish
fi
trap 'rm -f "$LOG_PLAIN" "$LOG_RACE"' EXIT

# Test 3: Unit tests pass
echo "Test: Unit tests pass"
run_suite "$LOG_PLAIN"
plain_rc=$R_RC; plain_pass=$R_PASS; plain_skip=$R_SKIP; plain_fail=$R_FAIL; plain_pkgs=$R_PKGS; plain_cached=$R_CACHED
cat "$LOG_PLAIN"
executed=$((plain_pass + plain_skip + plain_fail))
plain_readable=1
if [ "$plain_rc" -ne 0 ] || [ "$plain_fail" -gt 0 ]; then
    fail "Unit tests failed (${plain_fail} failing test function(s), go test rc=${plain_rc})"
elif [ "$plain_pkgs" -eq 0 ]; then
    plain_readable=0
    undet "go test exited 0 but printed no package result line — what it executed is unreadable"
elif [ "$plain_cached" -gt 0 ]; then
    undet "${plain_cached} package result(s) were replayed from the test cache (cached) — not a fresh execution"
else
    pass "Unit tests pass (${plain_pass} test function(s) executed and passed)"
fi

# Test 4: No race conditions
echo "Test: Race detector clean"
run_suite "$LOG_RACE" -race
race_rc=$R_RC; race_pass=$R_PASS; race_skip=$R_SKIP; race_fail=$R_FAIL; race_pkgs=$R_PKGS; race_cached=$R_CACHED
race_readable=1
if [ "$race_rc" -ne 0 ] || [ "$race_fail" -gt 0 ]; then
    fail "Race conditions detected (${race_fail} failing test function(s), go test rc=${race_rc})"
elif [ "$race_pkgs" -eq 0 ]; then
    race_readable=0
    undet "go test -race exited 0 but printed no package result line — what it executed is unreadable"
elif [ "$race_cached" -gt 0 ]; then
    undet "${race_cached} -race package result(s) were replayed from the test cache (cached) — not a fresh execution"
else
    pass "No race conditions detected (${race_pass} test function(s) executed under -race)"
fi

# Test 5: EXECUTED-COUNT FLOOR (VSC-371). executed == declared cannot catch a
# population that shrank at the source: delete every test file but one
# package's and both sides fall together. The floor is the population this
# module is known to have; a run below it is a finding, not a pass. Applied
# only to a READABLE count — an unreadable one is already UNDETERMINED and is
# not silently turned into "zero". A self-SKIPPED function counts toward the
# floor because the runner REPORTED it (the population is known); Test 6 then
# makes the run UNDETERMINED for it. Calling a deliberate -short run a "real
# finding" would be a FAIL-bluff; a function that is never reported at all is
# what the floor exists to catch.
echo "Test: Executed count reaches the floor (${UNIT_FLOOR} per run)"
plain_ran=$((plain_pass + plain_skip + plain_fail))
race_ran=$((race_pass + race_skip + race_fail))
floor_ok=1
if [ "$plain_readable" -eq 1 ] && [ "$plain_ran" -lt "$UNIT_FLOOR" ]; then
    floor_ok=0
    fail "plain run executed ${plain_ran} test function(s), below the floor ${UNIT_FLOOR} — a partial run is not a pass"
fi
if [ "$race_readable" -eq 1 ] && [ "$race_ran" -lt "$UNIT_FLOOR" ]; then
    floor_ok=0
    fail "-race run executed ${race_ran} test function(s), below the floor ${UNIT_FLOOR} — a partial run is not a pass"
fi
if [ "$floor_ok" -eq 1 ]; then
    if [ "$plain_readable" -eq 1 ] && [ "$race_readable" -eq 1 ]; then
        pass "executed ${plain_ran} (plain) and ${race_ran} (-race), floor ${UNIT_FLOOR}"
    else
        undet "the floor cannot be checked against an unreadable count"
    fi
fi

# Test 6: COVERAGE — the gate must not report a bare PASS for a population it
# never ran. This is the assertion whose absence let a -short run read as a
# pass over five untouched packages.
echo "Test: Every declared test function actually executed"
echo "  declared=${declared} executed=${executed} passed=${plain_pass} skipped=${plain_skip} failed=${plain_fail}"
if [ "${plain_skip}" -gt 0 ]; then
    echo "    packages that self-skipped:"
    skipped_packages "$LOG_PLAIN"
fi
if [ "$plain_readable" -eq 0 ]; then
    undet "executed count unreadable — coverage of this module is NOT established"
elif [ "$executed" -ne "$declared" ]; then
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

finish
