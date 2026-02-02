# CLAUDE.md

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
