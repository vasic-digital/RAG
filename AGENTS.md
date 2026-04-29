# AGENTS.md - Multi-Agent Coordination Guide

## Overview

This document provides guidance for AI agents (Claude Code, Copilot, Cursor, etc.) working with the `digital.vasic.rag` module. It describes conventions, coordination patterns, and boundaries that agents must respect.

## Module Identity

- **Module path**: `digital.vasic.rag`
- **Language**: Go 1.24+
- **Dependencies**: `github.com/stretchr/testify` (tests only)
- **Scope**: Generic, reusable RAG (Retrieval-Augmented Generation) primitives. No application-specific logic.

## Package Responsibilities

| Package | Owner Concern | Agent Must Not |
|---------|--------------|----------------|
| `pkg/retriever` | Core retrieval interfaces, Document type, MultiRetriever | Add provider-specific implementations |
| `pkg/chunker` | Text splitting strategies (fixed-size, recursive, sentence) | Introduce external NLP dependencies |
| `pkg/reranker` | Result reranking (score-based, MMR diversity) | Add network-dependent rerankers |
| `pkg/pipeline` | Pipeline composition with fluent builder API | Break the builder pattern chain |
| `pkg/hybrid` | Hybrid search combining semantic + keyword with fusion | Add vector database dependencies |

## Coordination Rules

### 1. Single-Package Changes

When modifying a single package, the agent owns that package for the duration of the task. No coordination with other agents is needed unless the change affects an exported interface.

### 2. Cross-Package Changes

If a change affects an exported type or interface (e.g., `retriever.Document`, `retriever.Retriever`), the agent must:

1. Verify all consumers of the interface within the module.
2. Update all affected packages in a single commit.
3. Run `go test ./... -race` to confirm no regressions.

### 3. Interface Contracts

These interfaces are stability boundaries. Breaking changes require explicit human approval:

- `retriever.Retriever` -- `Retrieve(ctx, query, opts) ([]Document, error)`
- `chunker.Chunker` -- `Chunk(text) []Chunk`
- `reranker.Reranker` -- `Rerank(ctx, query, docs) ([]Document, error)`
- `pipeline.Stage` -- `Process(ctx, input) (any, error)`
- `pipeline.RerankerStage` -- `Rerank(ctx, query, docs) ([]Document, error)`
- `pipeline.FormatterStage` -- `Format(ctx, docs) (any, error)`
- `hybrid.FusionStrategy` -- `Fuse(resultSets...) []Document`

### 4. Thread Safety Invariants

The following types are safe for concurrent use. Agents must preserve this guarantee:

- `retriever.MultiRetriever` -- protected by `sync.RWMutex`
- `hybrid.KeywordRetriever` -- protected by `sync.RWMutex`
- `hybrid.HybridRetriever` -- uses parallel goroutines with `sync.WaitGroup`

Agents must:

- Never remove mutex protection from shared state.
- Never introduce a public method that requires external synchronization.
- Always run `go test -race` after changes.

### 5. Dependency Direction

The dependency graph flows in one direction only:

```
retriever  <--  reranker
retriever  <--  pipeline
retriever  <--  hybrid
chunker    (standalone, no internal dependencies)
```

Agents must not introduce circular dependencies. The `retriever` package must never import other packages from this module.

### 6. Test Requirements

- All tests use `testify/assert` and `testify/require`.
- Test naming convention: `Test<Struct>_<Method>_<Scenario>`.
- Table-driven tests are preferred.
- Race detector must pass: `go test ./... -race`.
- Mock implementations are permitted only in `_test.go` files.

## Agent Workflow

### Before Making Changes

```bash
# Verify the module builds and tests pass
go build ./...
go test ./... -count=1 -race
```

### After Making Changes

```bash
# Format, vet, and test
gofmt -w .
go vet ./...
go test ./... -count=1 -race
```

### Commit Convention

```
<type>(<package>): <description>

# Examples:
feat(chunker): add semantic-aware chunking strategy
fix(reranker): correct MMR score normalization
test(hybrid): add BM25 edge case coverage
refactor(pipeline): simplify builder validation logic
docs(retriever): update Document field descriptions
```

## Boundaries

### What Agents May Do

- Fix bugs in any package.
- Add tests for uncovered code paths.
- Refactor internals without changing exported APIs.
- Add new exported methods that extend existing types.
- Add new Chunker, Reranker, or FusionStrategy implementations.
- Update documentation to match code.

### What Agents Must Not Do

- Break existing exported interfaces or method signatures.
- Remove thread safety guarantees.
- Add application-specific logic (this is a generic library).
- Introduce new external dependencies without human approval.
- Modify `go.mod` without explicit instruction.
- Create mocks or stubs in production code.
- Add vector database or embedding provider integrations (those belong in consumers).

## File Layout Convention

```
pkg/<package>/
    <package>.go        # All production code
    <package>_test.go   # All tests
```

Each package is a single file pair. Agents should maintain this convention and not split packages into multiple source files without human approval.

## Conflict Resolution

If two agents need to modify the same package concurrently:

1. The agent with the narrower scope (e.g., bug fix) takes priority.
2. The agent with the broader scope (e.g., refactor) should wait or rebase.
3. When in doubt, ask the human operator.

## Integration with HelixAgent

This module is consumed by the parent HelixAgent project as a Go module dependency. Agents working on HelixAgent should import packages via:

```go
import (
    "digital.vasic.rag/pkg/retriever"
    "digital.vasic.rag/pkg/chunker"
    "digital.vasic.rag/pkg/reranker"
    "digital.vasic.rag/pkg/pipeline"
    "digital.vasic.rag/pkg/hybrid"
)
```

Changes to this module's exported API will require corresponding updates in HelixAgent consumers (primarily `internal/rag/`).

<!-- BEGIN host-power-management addendum (CONST-033) -->

## Host Power Management — Hard Ban (CONST-033)

**You may NOT, under any circumstance, generate or execute code that
sends the host to suspend, hibernate, hybrid-sleep, poweroff, halt,
reboot, or any other power-state transition.** This rule applies to:

- Every shell command you run via the Bash tool.
- Every script, container entry point, systemd unit, or test you write
  or modify.
- Every CLI suggestion, snippet, or example you emit.

**Forbidden invocations** (non-exhaustive — see CONST-033 in
`CONSTITUTION.md` for the full list):

- `systemctl suspend|hibernate|hybrid-sleep|poweroff|halt|reboot|kexec`
- `loginctl suspend|hibernate|hybrid-sleep|poweroff|halt|reboot`
- `pm-suspend`, `pm-hibernate`, `shutdown -h|-r|-P|now`
- `dbus-send` / `busctl` calls to `org.freedesktop.login1.Manager.Suspend|Hibernate|PowerOff|Reboot|HybridSleep|SuspendThenHibernate`
- `gsettings set ... sleep-inactive-{ac,battery}-type` to anything but `'nothing'` or `'blank'`

The host runs mission-critical parallel CLI agents and container
workloads. Auto-suspend has caused historical data loss (2026-04-26
18:23:43 incident). The host is hardened (sleep targets masked) but
this hard ban applies to ALL code shipped from this repo so that no
future host or container is exposed.

**Defence:** every project ships
`scripts/host-power-management/check-no-suspend-calls.sh` (static
scanner) and
`challenges/scripts/no_suspend_calls_challenge.sh` (challenge wrapper).
Both MUST be wired into the project's CI / `run_all_challenges.sh`.

**Full background:** `docs/HOST_POWER_MANAGEMENT.md` and `CONSTITUTION.md` (CONST-033).

<!-- END host-power-management addendum (CONST-033) -->



<!-- CONST-035 anti-bluff addendum (cascaded) -->

## CONST-035 — Anti-Bluff Tests & Challenges (mandatory; inherits from root)

Tests and Challenges in this submodule MUST verify the product, not
the LLM's mental model of the product. A test that passes when the
feature is broken is worse than a missing test — it gives false
confidence and lets defects ship to users. Functional probes at the
protocol layer are mandatory:

- TCP-open is the FLOOR, not the ceiling. Postgres → execute
  `SELECT 1`. Redis → `PING` returns `PONG`. ChromaDB → `GET
  /api/v1/heartbeat` returns 200. MCP server → TCP connect + valid
  JSON-RPC handshake. HTTP gateway → real request, real response,
  non-empty body.
- Container `Up` is NOT application healthy. A `docker/podman ps`
  `Up` status only means PID 1 is running; the application may be
  crash-looping internally.
- No mocks/fakes outside unit tests (already CONST-030; CONST-035
  raises the cost of a mock-driven false pass to the same severity
  as a regression).
- Re-verify after every change. Don't assume a previously-passing
  test still verifies the same scope after a refactor.
- Verification of CONST-035 itself: deliberately break the feature
  (e.g. `kill <service>`, swap a password). The test MUST fail. If
  it still passes, the test is non-conformant and MUST be tightened.

## CONST-033 clarification — distinguishing host events from sluggishness

Heavy container builds (BuildKit pulling many GB of layers, parallel
podman/docker compose-up across many services) can make the host
**appear** unresponsive — high load average, slow SSH, watchers
timing out. **This is NOT a CONST-033 violation.** Suspend / hibernate
/ logout are categorically different events. Distinguish via:

- `uptime` — recent boot? if so, the host actually rebooted.
- `loginctl list-sessions` — session(s) still active? if yes, no logout.
- `journalctl ... | grep -i 'will suspend\|hibernate'` — zero broadcasts
  since the CONST-033 fix means no suspend ever happened.
- `dmesg | grep -i 'killed process\|out of memory'` — OOM kills are
  also NOT host-power events; they're memory-pressure-induced and
  require their own separate fix (lower per-container memory limits,
  reduce parallelism).

A sluggish host under build pressure recovers when the build finishes;
a suspended host requires explicit unsuspend (and CONST-033 should
make that impossible by hardening `IdleAction=ignore` +
`HandleSuspendKey=ignore` + masked `sleep.target`,
`suspend.target`, `hibernate.target`, `hybrid-sleep.target`).

If you observe what looks like a suspend during heavy builds, the
correct first action is **not** "edit CONST-033" but `bash
challenges/scripts/host_no_auto_suspend_challenge.sh` to confirm the
hardening is intact. If hardening is intact AND no suspend
broadcast appears in journal, the perceived event was build-pressure
sluggishness, not a power transition.

<!-- BEGIN no-session-termination addendum (CONST-036) -->

## User-Session Termination — Hard Ban (CONST-036)

**You may NOT, under any circumstance, generate or execute code that
ends the currently-logged-in user's desktop session, kills their
`user@<UID>.service` user manager, or indirectly forces them to
manually log out / power off.** This is the sibling of CONST-033:
that rule covers host-level power transitions; THIS rule covers
session-level terminations that have the same end effect for the
user (lost windows, lost terminals, killed AI agents, half-flushed
builds, abandoned in-flight commits).

**Why this rule exists.** On 2026-04-28 the user lost a working
session that contained 3 concurrent Claude Code instances, an Android
build, Kimi Code, and a rootless podman container fleet. The
`user.slice` consumed 60.6 GiB peak / 5.2 GiB swap, the GUI became
unresponsive, the user was forced to log out and then power off via
the GNOME shell. The host could not auto-suspend (CONST-033 was in
place and verified) and the kernel OOM killer never fired — but the
user had to manually end the session anyway, because nothing
prevented overlapping heavy workloads from saturating the slice.
CONST-036 closes that loophole at both the source-code layer and the
operational layer. See
`docs/issues/fixed/SESSION_LOSS_2026-04-28.md` in the HelixAgent
project.

**Forbidden direct invocations** (non-exhaustive):

- `loginctl terminate-user|terminate-session|kill-user|kill-session`
- `systemctl stop user@<UID>` / `systemctl kill user@<UID>`
- `gnome-session-quit`
- `pkill -KILL -u $USER` / `killall -u $USER`
- `dbus-send` / `busctl` calls to `org.gnome.SessionManager.Logout|Shutdown|Reboot`
- `echo X > /sys/power/state`
- `/usr/bin/poweroff`, `/usr/bin/reboot`, `/usr/bin/halt`

**Indirect-pressure clauses:**

1. Do not spawn parallel heavy workloads casually; check `free -h`
   first; keep `user.slice` under 70% of physical RAM.
2. Long-lived background subagents go in `system.slice`. Rootless
   podman containers die with the user manager.
3. Document AI-agent concurrency caps in CLAUDE.md.
4. Never script "log out and back in" recovery flows.

**Defence:** every project ships
`scripts/host-power-management/check-no-session-termination-calls.sh`
(static scanner) and
`challenges/scripts/no_session_termination_calls_challenge.sh`
(challenge wrapper). Both MUST be wired into the project's CI /
`run_all_challenges.sh`.

<!-- END no-session-termination addendum (CONST-036) -->

<!-- BEGIN const035-strengthening-2026-04-29 -->

## CONST-035 — End-User Usability Mandate (2026-04-29 strengthening)

A test or Challenge that PASSES is a CLAIM that the tested behavior
**works for the end user of the product**. The HelixAgent project
has repeatedly hit the failure mode where every test ran green AND
every Challenge reported PASS, yet most product features did not
actually work — buggy challenge wrappers masked failed assertions,
scripts checked file existence without executing the file,
"reachability" tests tolerated timeouts, contracts were honest in
advertising but broken in dispatch. **This MUST NOT recur.**

Every PASS result MUST guarantee:

a. **Quality** — the feature behaves correctly under inputs an end
   user will send, including malformed input, edge cases, and
   concurrency that real workloads produce.
b. **Completion** — the feature is wired end-to-end from public
   API surface down to backing infrastructure, with no stub /
   placeholder / "wired lazily later" gaps that silently 503.
c. **Full usability** — a CLI agent / SDK consumer / direct curl
   client following the documented model IDs, request shapes, and
   endpoints SUCCEEDS without having to know which of N internal
   aliases the dispatcher actually accepts.

A passing test that doesn't certify all three is a **bluff** and
MUST be tightened, or marked `t.Skip("...SKIP-OK: #<ticket>")`
so absence of coverage is loud rather than silent.

### Bluff taxonomy (each pattern observed in HelixAgent and now forbidden)

- **Wrapper bluff** — assertions PASS but the wrapper's exit-code
  logic is buggy, marking the run FAILED (or the inverse: assertions
  FAIL but the wrapper swallows them). Every aggregating wrapper MUST
  use a robust counter (`! grep -qs "|FAILED|" "$LOG"` style) —
  never inline arithmetic on a command that prints AND exits
  non-zero.
- **Contract bluff** — the system advertises a capability but
  rejects it in dispatch. Every advertised capability MUST be
  exercised by a test or Challenge that actually invokes it.
- **Structural bluff** — `check_file_exists "foo_test.go"` passes
  if the file is present but doesn't run the test or assert anything
  about its content. File-existence checks MUST be paired with at
  least one functional assertion.
- **Comment bluff** — a code comment promises a behavior the code
  doesn't actually have. Documentation written before / about code
  MUST be re-verified against the code on every change touching the
  documented function.
- **Skip bluff** — `t.Skip("not running yet")` without a
  `SKIP-OK: #<ticket>` marker silently passes. Every skip needs the
  marker; CI fails on bare skips.

The taxonomy is illustrative, not exhaustive. Every Challenge or
test added going forward MUST pass an honest self-review against
this taxonomy before being committed.

<!-- END const035-strengthening-2026-04-29 -->
