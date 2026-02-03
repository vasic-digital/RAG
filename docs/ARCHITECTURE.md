# Architecture

## Overview

The `digital.vasic.rag` module provides core RAG (Retrieval-Augmented Generation) primitives as a zero-dependency Go library (only `testify` for tests). It is designed as a foundation layer that application code builds upon, rather than a complete RAG system.

The module is organized into five packages with a strict, unidirectional dependency graph:

```
chunker (standalone)

retriever  <--  reranker
retriever  <--  pipeline
retriever  <--  hybrid
```

The `retriever` package is the foundational package. It defines the `Document` type and the `Retriever` interface that all other packages depend on. The `chunker` package is completely standalone with no internal dependencies.

## Design Decisions

### 1. Interface-First Design

Every major abstraction is defined as a Go interface:

- `retriever.Retriever` -- the universal retrieval contract
- `chunker.Chunker` -- text splitting contract
- `reranker.Reranker` -- result reranking contract
- `pipeline.Stage` -- generic pipeline processing stage
- `hybrid.FusionStrategy` -- result fusion contract

This allows consumers to provide their own implementations (vector database retrievers, LLM-based rerankers, custom fusion strategies) without modifying the module.

### 2. No External Dependencies

The module has zero runtime dependencies. Only `github.com/stretchr/testify` is used, and only in test files. This is a deliberate choice to keep the module lightweight and avoid dependency conflicts when integrated into larger systems like HelixAgent.

### 3. Value Semantics for Documents

`retriever.Document` is a struct, not an interface. Documents are passed by value (or by slice), making them safe to copy, serialize, and pass across goroutine boundaries without synchronization concerns.

### 4. Context-First Methods

All methods that perform I/O or may be long-running accept `context.Context` as the first parameter, following standard Go conventions. This enables cancellation, timeouts, and deadline propagation.

## Design Patterns

### Strategy Pattern

Used extensively for interchangeable algorithms:

- **Chunking strategies**: `FixedSizeChunker`, `RecursiveChunker`, `SentenceChunker` all implement the `Chunker` interface. Consumers select the strategy at construction time.
- **Reranking strategies**: `ScoreReranker` and `MMRReranker` implement `Reranker`. The choice depends on whether diversity is needed.
- **Fusion strategies**: `RRFStrategy` and `LinearStrategy` implement `FusionStrategy`. RRF is rank-based and score-agnostic; Linear is score-based with configurable weights.

### Facade Pattern

`MultiRetriever` acts as a facade over multiple `Retriever` implementations. It presents a single `Retrieve` method while internally managing parallel execution, deduplication, error aggregation, and result merging.

`HybridRetriever` similarly facades over a `SemanticRetriever` and `KeywordRetriever`, coordinating parallel queries and delegating fusion to the configured `FusionStrategy`.

### Template Method Pattern

The chunkers follow a template method approach:

1. Check for empty text (return nil)
2. Check if text fits in a single chunk (return single-element slice)
3. Apply the splitting algorithm (varies per implementation)
4. Return chunks with position tracking (Start/End offsets)

Steps 1 and 2 are shared across all chunkers. Step 3 is the variant behavior:
- `FixedSizeChunker`: sliding window with step = ChunkSize - Overlap
- `RecursiveChunker`: hierarchical splitting by separator list, with fallback to next separator
- `SentenceChunker`: sentence boundary detection, then grouping within chunk size limits

### Pipeline Pattern (Builder)

The `pipeline` package implements a multi-stage processing pipeline with a fluent builder API:

```
Builder -> configure -> build -> Pipeline -> execute
```

The builder validates configuration incrementally:
1. `Retrieve()` sets the retriever (required)
2. `Rerank()` adds a reranking stage (optional)
3. `Format()` adds a formatting stage (optional)
4. `AddStage()` adds custom stages (optional)
5. `Build()` validates and produces an immutable `Pipeline`

Pipeline execution follows a fixed order:
1. Retrieval (always first, produces `[]Document`)
2. Stages in order (each receives previous stage's output)

Stages are adapted via internal adapters (`rerankerAdapter`, `formatterAdapter`) that handle type assertions and error wrapping.

### Adapter Pattern

The pipeline uses adapters to unify different stage types under the common `Stage` interface:

- `rerankerAdapter` wraps `RerankerStage` into `Stage`, handling `any` to `[]Document` conversion
- `formatterAdapter` wraps `FormatterStage` into `Stage`
- `StageFunc` adapts plain functions to the `Stage` interface

## Package Deep Dives

### retriever

The foundational package. Defines:

- `Document`: the universal document representation with ID, Content, Metadata, Score, and Source
- `Options`: retrieval configuration with TopK, MinScore, and optional Filter map
- `Retriever`: single-method interface for document retrieval
- `MultiRetriever`: concurrent multi-source retrieval with deduplication

`MultiRetriever` uses `sync.RWMutex` for safe concurrent access to its retriever list, and `sync.WaitGroup` for parallel query execution. Deduplication keeps the document with the highest score when the same ID appears from multiple sources.

### chunker

Standalone package with no internal dependencies. Provides three chunking strategies:

- **FixedSizeChunker**: Simple sliding window. Step size = ChunkSize - Overlap. Produces chunks with byte-accurate Start/End offsets.
- **RecursiveChunker**: Tries separators in order (default: paragraph, line, sentence, word). If splitting by the current separator produces a chunk that exceeds ChunkSize, it recursively tries the next separator. Falls back to fixed-size splitting when all separators are exhausted. After splitting, it merges results and computes original-text offsets.
- **SentenceChunker**: Detects sentence boundaries (`.`, `!`, `?` followed by whitespace), groups sentences into chunks within size limits, and supports overlap by retaining trailing sentences from the previous chunk.

All chunkers validate configuration at construction time, clamping invalid values to sensible defaults.

### reranker

Depends on `retriever` for the `Document` type. Provides:

- **ScoreReranker**: Passthrough reranker that sorts by existing score and limits to TopK. Useful as a baseline or when retriever scores are already high quality.
- **MMRReranker**: Implements Maximal Marginal Relevance. Uses word-overlap Jaccard similarity (via the internal `textSimilarity` function) for both query-document and document-document similarity. Greedy selection: at each step, picks the unselected document maximizing `Lambda * relevance - (1 - Lambda) * max_similarity_to_selected`.

The `textSimilarity` function tokenizes text into lowercase words and computes Jaccard similarity (intersection / union). This is a lightweight approximation; production systems may substitute embedding-based similarity.

### pipeline

Depends on `retriever` for types. Implements the builder pattern:

- `Builder` accumulates configuration with method chaining
- `Build()` validates (retriever required, max stages check, no nil stages) and returns an immutable `Pipeline`
- `Pipeline.Execute()` runs retrieval, then passes output through stages sequentially

The package also provides context utilities (`WithQuery`, `QueryFromContext`) for passing the query string through the pipeline via `context.Context`, allowing stages to access the original query.

### hybrid

The most complex package. Depends on `retriever` for types. Contains:

- **KeywordRetriever**: Full BM25 implementation with in-memory inverted index. Maintains term frequencies, document frequencies, document lengths, and average document length. Supports incremental indexing (`Index`) and removal (`Remove`). Uses standard BM25 parameters (k1=1.2, b=0.75).
- **SemanticRetriever**: Thin wrapper around any `Retriever`, providing a semantic search interface. Designed to be backed by a vector database in production.
- **HybridRetriever**: Orchestrates parallel retrieval from semantic and keyword retrievers, then fuses results. Uses `PreRetrieveMultiplier` to over-fetch before fusion. Tolerates partial failures (returns results if at least one retriever succeeds).
- **RRFStrategy**: Reciprocal Rank Fusion. Rank-based, score-agnostic. Default k=60.
- **LinearStrategy**: Weighted linear combination with per-set score normalization.

## Error Handling

The module follows Go error wrapping conventions:

- Errors are wrapped with context: `fmt.Errorf("retrieval failed: %w", err)`
- Partial failures in parallel operations (MultiRetriever, HybridRetriever) return results from successful sources
- Total failures return combined error messages
- Pipeline stages report their index on failure: `"stage %d failed: %w"`

## Concurrency Model

Thread-safe types use `sync.RWMutex` for read-heavy workloads:
- `MultiRetriever`: RLock for Retrieve, Lock for AddRetriever
- `KeywordRetriever`: RLock for Retrieve, Lock for Index/Remove

Parallel operations use `sync.WaitGroup`:
- `MultiRetriever.Retrieve`: queries all retrievers concurrently
- `HybridRetriever.Retrieve`: queries semantic and keyword retrievers concurrently

No goroutines are leaked; all parallel operations complete before returning.
