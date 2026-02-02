# RAG - Retrieval-Augmented Generation Module

A generic, reusable Go module providing core RAG (Retrieval-Augmented Generation) primitives for building retrieval-augmented applications.

## Packages

### pkg/retriever
Core retrieval interfaces and types including `Document`, `Options`, `Retriever` interface, and `MultiRetriever` for combining multiple retrievers.

### pkg/chunker
Document chunking strategies:
- `FixedSizeChunker` - configurable chunk size with overlap
- `RecursiveChunker` - hierarchical splitting by separators
- `SentenceChunker` - sentence-boundary aware splitting

### pkg/reranker
Result reranking implementations:
- `ScoreReranker` - reranks by existing score
- `MMRReranker` - Maximal Marginal Relevance for diversity

### pkg/pipeline
RAG pipeline composition with a fluent builder API for chaining Retriever, Reranker, and Formatter stages.

### pkg/hybrid
Hybrid retrieval combining semantic and keyword search:
- `KeywordRetriever` using BM25-style scoring
- `SemanticRetriever` using vector similarity
- Fusion strategies: RRF (Reciprocal Rank Fusion), Linear combination

## Installation

```bash
go get digital.vasic.rag
```

## Testing

```bash
go test ./... -count=1 -race
```
