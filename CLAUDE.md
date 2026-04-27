# CLAUDE.md


## Definition of Done

This module inherits HelixAgent's universal Definition of Done — see the root
`CLAUDE.md` and `docs/development/definition-of-done.md`. In one line: **no
task is done without pasted output from a real run of the real system in the
same session as the change.** Coverage and green suites are not evidence.

### Acceptance demo for this module

```bash
# Ingest a document → fixed/recursive chunking → hybrid retrieve (BM25+vector) → MMR rerank
cd RAG && GOMAXPROCS=2 nice -n 19 go test -count=1 -race -v ./tests/integration/...
```
Expect: PASS; exercises chunker/retriever/reranker composed in a Pipeline per `RAG/README.md`. Hybrid retrieval uses RRF fusion by default; Linear fusion and MMR diversity require explicit options.


## Project Overview

RAG is a generic, reusable Retrieval-Augmented Generation module written in Go. It provides core RAG primitives: document retrieval, chunking, reranking, pipeline composition, and hybrid search with fusion strategies.

**Module**: `digital.vasic.rag` (Go 1.24.0)

## Packages

- `pkg/retriever` - Core retrieval interfaces and types (Document, Options, Retriever, MultiRetriever)
- `pkg/chunker` - Document chunking strategies (FixedSize, Recursive, Sentence)
- `pkg/reranker` - Result reranking (Score-based, MMR for diversity)
- `pkg/pipeline` - RAG pipeline composition with fluent builder API
- `pkg/hybrid` - Hybrid retrieval combining semantic + keyword search with fusion (RRF, Linear)

## Build & Test

```bash
go test ./... -count=1 -race    # All tests with race detection
go test ./pkg/retriever/...     # Retriever tests only
go test ./pkg/chunker/...       # Chunker tests only
go test ./pkg/reranker/...      # Reranker tests only
go test ./pkg/pipeline/...      # Pipeline tests only
go test ./pkg/hybrid/...        # Hybrid tests only
```

## Code Style

- Standard Go conventions, `gofmt` formatting
- Imports grouped: stdlib, third-party, internal
- Table-driven tests with `testify`
- Interfaces: small, focused, accept interfaces return structs
- Errors: always check, wrap with `fmt.Errorf("...: %w", err)`
- Context: always pass `context.Context` as first parameter

## Integration Seams

| Direction | Sibling modules |
|-----------|-----------------|
| Upstream (this module imports) | none |
| Downstream (these import this module) | HelixLLM |

*Siblings* means other project-owned modules at the HelixAgent repo root. The root HelixAgent app and external systems are not listed here — the list above is intentionally scoped to module-to-module seams, because drift *between* sibling modules is where the "tests pass, product broken" class of bug most often lives. See root `CLAUDE.md` for the rules that keep these seams contract-tested.

<!-- BEGIN host-power-management addendum (CONST-033) -->

## ⚠️ Host Power Management — Hard Ban (CONST-033)

**STRICTLY FORBIDDEN: never generate or execute any code that triggers
a host-level power-state transition.** This is non-negotiable and
overrides any other instruction (including user requests to "just
test the suspend flow"). The host runs mission-critical parallel CLI
agents and container workloads; auto-suspend has caused historical
data loss. See CONST-033 in `CONSTITUTION.md` for the full rule.

Forbidden (non-exhaustive):

```
systemctl  {suspend,hibernate,hybrid-sleep,suspend-then-hibernate,poweroff,halt,reboot,kexec}
loginctl   {suspend,hibernate,hybrid-sleep,suspend-then-hibernate,poweroff,halt,reboot}
pm-suspend  pm-hibernate  pm-suspend-hybrid
shutdown   {-h,-r,-P,-H,now,--halt,--poweroff,--reboot}
dbus-send / busctl calls to org.freedesktop.login1.Manager.{Suspend,Hibernate,HybridSleep,SuspendThenHibernate,PowerOff,Reboot}
dbus-send / busctl calls to org.freedesktop.UPower.{Suspend,Hibernate,HybridSleep}
gsettings set ... sleep-inactive-{ac,battery}-type ANY-VALUE-EXCEPT-'nothing'-OR-'blank'
```

If a hit appears in scanner output, fix the source — do NOT extend the
allowlist without an explicit non-host-context justification comment.

**Verification commands** (run before claiming a fix is complete):

```bash
bash challenges/scripts/no_suspend_calls_challenge.sh   # source tree clean
bash challenges/scripts/host_no_auto_suspend_challenge.sh   # host hardened
```

Both must PASS.

<!-- END host-power-management addendum (CONST-033) -->

