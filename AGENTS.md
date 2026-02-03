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
