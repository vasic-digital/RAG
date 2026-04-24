# CLAUDE.md


## Definition of Done

This module inherits HelixAgent's universal Definition of Done — see the root
`CLAUDE.md` and `docs/development/definition-of-done.md`. In one line: **no
task is done without pasted output from a real run of the real system in the
same session as the change.** Coverage and green suites are not evidence.

### Acceptance demo for this module

<!-- TODO: replace this block with the exact command(s) that exercise this
     module end-to-end against real dependencies, and the expected output.
     The commands must run the real artifact (built binary, deployed
     container, real service) — no in-process fakes, no mocks, no
     `httptest.NewServer`, no Robolectric, no JSDOM as proof of done. -->

```bash
# TODO
```

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
