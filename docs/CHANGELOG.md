# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.1.0] - 2026-02-03

### Added

- **pkg/retriever**: Core retrieval interfaces and types.
  - `Document` struct with ID, Content, Metadata, Score, and Source fields.
  - `Options` struct with TopK, MinScore, and Filter configuration.
  - `Retriever` interface defining the universal retrieval contract.
  - `MultiRetriever` for combining multiple retrievers with parallel execution, deduplication by document ID (keeping highest score), and score-sorted results.
  - `DefaultOptions()` function returning sensible defaults.
  - Full test coverage with table-driven tests.

- **pkg/chunker**: Document chunking strategies.
  - `Chunk` struct with Content, Start/End byte offsets, and Metadata.
  - `Chunker` interface for text splitting.
  - `Config` struct with ChunkSize, Overlap, and Separators.
  - `FixedSizeChunker` for sliding-window chunking with configurable overlap.
  - `RecursiveChunker` for hierarchical splitting by separator list with fallback.
  - `SentenceChunker` for sentence-boundary-aware chunking.
  - `DefaultConfig()` function with sensible defaults.
  - Input validation in all constructors (clamping invalid values).
  - Full test coverage.

- **pkg/reranker**: Result reranking strategies.
  - `Reranker` interface for document reranking.
  - `Config` struct with Lambda and TopK.
  - `ScoreReranker` for passthrough score-based sorting with TopK limiting.
  - `MMRReranker` implementing Maximal Marginal Relevance with Jaccard word-overlap similarity for balancing relevance and diversity.
  - `DefaultConfig()` function (Lambda=0.5, TopK=10).
  - Full test coverage.

- **pkg/pipeline**: RAG pipeline composition.
  - `Stage` interface for generic processing stages.
  - `StageFunc` adapter for function-to-Stage conversion.
  - `Builder` with fluent API: `Retrieve()`, `Rerank()`, `Format()`, `AddStage()`, `WithConfig()`, `Build()`.
  - `Pipeline` struct with `Execute()` for running the complete pipeline.
  - `Result` struct with Documents and Output fields.
  - `RerankerStage` and `FormatterStage` interfaces for typed pipeline integration.
  - `WithQuery()` and `QueryFromContext()` for passing query through context.
  - Builder validation (nil checks, MaxStages limit).
  - Full test coverage.

- **pkg/hybrid**: Hybrid retrieval with fusion strategies.
  - `FusionStrategy` interface for combining result sets.
  - `RRFStrategy` implementing Reciprocal Rank Fusion with configurable k constant.
  - `LinearStrategy` implementing weighted linear combination with score normalization.
  - `KeywordRetriever` with full BM25 implementation (k1=1.2, b=0.75), in-memory inverted index, incremental indexing and removal.
  - `SemanticRetriever` wrapping any `Retriever` for semantic search.
  - `HybridRetriever` combining semantic and keyword retrieval with parallel execution, configurable fusion, and PreRetrieveMultiplier for over-fetching.
  - `HybridConfig` and `DefaultHybridConfig()`.
  - Partial failure tolerance (returns results if at least one retriever succeeds).
  - Full test coverage.

- **Documentation**: CLAUDE.md, README.md, AGENTS.md, and docs/ directory.
