# User Guide

## Overview

`digital.vasic.rag` is a generic Go module providing core Retrieval-Augmented Generation primitives. It includes document chunking, retrieval interfaces, result reranking, hybrid search with fusion strategies, and pipeline composition via a fluent builder API.

## Installation

```bash
go get digital.vasic.rag
```

Requires Go 1.24 or later.

## Quick Start

```go
package main

import (
    "context"
    "fmt"

    "digital.vasic.rag/pkg/chunker"
    "digital.vasic.rag/pkg/hybrid"
    "digital.vasic.rag/pkg/pipeline"
    "digital.vasic.rag/pkg/reranker"
    "digital.vasic.rag/pkg/retriever"
)
```

## Document Chunking

The `chunker` package provides three strategies for splitting text into smaller pieces suitable for embedding and retrieval.

### Fixed-Size Chunking

Splits text into fixed-size chunks with configurable overlap. Overlap ensures that content near chunk boundaries is not lost.

```go
cfg := chunker.Config{
    ChunkSize: 500,
    Overlap:   100,
}
c := chunker.NewFixedSizeChunker(cfg)

text := "Your long document text goes here..."
chunks := c.Chunk(text)

for _, ch := range chunks {
    fmt.Printf("Chunk [%d:%d]: %s\n", ch.Start, ch.End, ch.Content)
}
```

The `ChunkSize` controls the maximum number of bytes per chunk. The `Overlap` controls how many trailing bytes from the previous chunk are repeated at the start of the next. If `Overlap` is greater than or equal to `ChunkSize`, it is automatically clamped to `ChunkSize / 4`.

### Recursive Chunking

Splits text hierarchically by trying separators in order. If a chunk produced by the first separator is still too large, it is split again using the next separator in the list. Falls back to fixed-size splitting when no separators remain.

```go
cfg := chunker.Config{
    ChunkSize:  1000,
    Overlap:    200,
    Separators: []string{"\n\n", "\n", ". ", " "},
}
c := chunker.NewRecursiveChunker(cfg)

chunks := c.Chunk(longDocument)
for _, ch := range chunks {
    fmt.Printf("[%d:%d] %s\n", ch.Start, ch.End, ch.Content[:50])
}
```

The default separators are `["\n\n", "\n", ". ", " "]`, which progressively split by paragraph, line, sentence, and word.

### Sentence-Boundary Chunking

Groups sentences into chunks that respect sentence boundaries. Sentences are detected by `.`, `!`, and `?` followed by whitespace or end-of-text.

```go
cfg := chunker.Config{
    ChunkSize: 800,
    Overlap:   150,
}
c := chunker.NewSentenceChunker(cfg)

chunks := c.Chunk(articleText)
// Each chunk contains complete sentences
```

### Default Configuration

Use `chunker.DefaultConfig()` for sensible defaults:

```go
cfg := chunker.DefaultConfig()
// ChunkSize: 1000, Overlap: 200
// Separators: ["\n\n", "\n", ". ", " "]
```

## Retrieval

The `retriever` package defines the core `Retriever` interface and the `Document` type used throughout the module.

### The Document Type

```go
type Document struct {
    ID       string         `json:"id"`
    Content  string         `json:"content"`
    Metadata map[string]any `json:"metadata,omitempty"`
    Score    float64        `json:"score,omitempty"`
    Source   string         `json:"source,omitempty"`
}
```

### Implementing a Retriever

Any type that implements the `Retriever` interface can be used in pipelines and hybrid search:

```go
type Retriever interface {
    Retrieve(ctx context.Context, query string, opts Options) ([]Document, error)
}
```

Example implementation wrapping a vector database:

```go
type VectorDBRetriever struct {
    client VectorDBClient
}

func (v *VectorDBRetriever) Retrieve(
    ctx context.Context,
    query string,
    opts retriever.Options,
) ([]retriever.Document, error) {
    results, err := v.client.Search(ctx, query, opts.TopK)
    if err != nil {
        return nil, fmt.Errorf("vector search failed: %w", err)
    }

    docs := make([]retriever.Document, len(results))
    for i, r := range results {
        docs[i] = retriever.Document{
            ID:      r.ID,
            Content: r.Text,
            Score:   r.Similarity,
            Source:  "vector-db",
        }
    }
    return docs, nil
}
```

### Retrieval Options

```go
opts := retriever.Options{
    TopK:     20,              // Maximum number of results
    MinScore: 0.5,             // Minimum relevance threshold
    Filter:   map[string]any{  // Optional metadata filters
        "category": "technical",
    },
}
```

Use `retriever.DefaultOptions()` for defaults (TopK=10, MinScore=0.0, no filters).

### Multi-Retriever

Combine multiple retrievers. Results are queried in parallel, deduplicated by document ID (keeping the highest score), and sorted by score descending.

```go
vectorRetriever := NewVectorDBRetriever(vectorClient)
graphRetriever := NewGraphDBRetriever(graphClient)

multi := retriever.NewMultiRetriever(vectorRetriever, graphRetriever)

docs, err := multi.Retrieve(ctx, "What is RAG?", retriever.DefaultOptions())
if err != nil {
    log.Fatal(err)
}

// Results are deduplicated and sorted by score
for _, doc := range docs {
    fmt.Printf("[%.2f] %s: %s\n", doc.Score, doc.ID, doc.Content[:80])
}
```

Retrievers can be added dynamically:

```go
multi := retriever.NewMultiRetriever()
multi.AddRetriever(vectorRetriever)
multi.AddRetriever(graphRetriever)
```

If some retrievers fail but at least one succeeds, results from the successful retrievers are returned. An error is returned only when all retrievers fail.

## Reranking

The `reranker` package provides strategies for improving result quality after initial retrieval.

### Score-Based Reranking

Sorts documents by their existing score and limits to TopK:

```go
cfg := reranker.Config{TopK: 5}
sr := reranker.NewScoreReranker(cfg)

reranked, err := sr.Rerank(ctx, "query", docs)
```

### MMR (Maximal Marginal Relevance)

Balances relevance against diversity. The Lambda parameter controls the trade-off:
- Lambda=1.0: Maximum relevance (ignores diversity)
- Lambda=0.0: Maximum diversity (ignores relevance)
- Lambda=0.5: Balanced (default)

```go
cfg := reranker.Config{
    Lambda: 0.7,  // Favor relevance slightly over diversity
    TopK:   10,
}
mmr := reranker.NewMMRReranker(cfg)

diverseResults, err := mmr.Rerank(ctx, "machine learning basics", docs)
```

The MMR algorithm uses word-overlap (Jaccard) similarity for computing inter-document and query-document similarity. The formula is:

```
MMR(d) = Lambda * Sim(d, query) - (1 - Lambda) * max(Sim(d, d_selected))
```

Documents are selected greedily, one at a time, maximizing the MMR score at each step.

## Hybrid Search

The `hybrid` package combines semantic (vector) and keyword (BM25) search with configurable fusion strategies.

### Keyword Retrieval (BM25)

In-memory BM25-based keyword search. Documents must be indexed before querying.

```go
kw := hybrid.NewKeywordRetriever()

// Index documents
kw.Index([]retriever.Document{
    {ID: "1", Content: "Introduction to machine learning algorithms"},
    {ID: "2", Content: "Deep learning neural network architectures"},
    {ID: "3", Content: "Statistical methods for data analysis"},
})

// Search
docs, err := kw.Retrieve(ctx, "machine learning", retriever.DefaultOptions())

// Remove a document
kw.Remove("3")
```

### Semantic Retrieval

Wraps any `retriever.Retriever` to provide a semantic search interface. In production, the inner retriever would be backed by a vector database.

```go
vectorRet := NewVectorDBRetriever(client)
semantic := hybrid.NewSemanticRetriever(vectorRet)
```

### Hybrid Retrieval

Combines semantic and keyword retrievers, querying both in parallel and fusing results:

```go
kw := hybrid.NewKeywordRetriever()
kw.Index(documents)

semantic := hybrid.NewSemanticRetriever(vectorRetriever)

hybridRet := hybrid.NewHybridRetriever(
    semantic,
    kw,
    hybrid.NewRRFStrategy(60),       // Reciprocal Rank Fusion
    hybrid.DefaultHybridConfig(),     // PreRetrieveMultiplier: 3
)

docs, err := hybridRet.Retrieve(ctx, "query", retriever.Options{TopK: 10})
```

The `PreRetrieveMultiplier` causes each sub-retriever to fetch `TopK * Multiplier` results before fusion, ensuring enough candidates for quality fusion output.

### Fusion Strategies

**Reciprocal Rank Fusion (RRF):**

```go
rrf := hybrid.NewRRFStrategy(60) // k=60 is the standard constant
```

RRF computes: `score(d) = sum(1 / (k + rank(d)))` across all result sets. It is rank-based and does not depend on score magnitudes.

**Linear Combination:**

```go
linear := hybrid.NewLinearStrategy(0.7, 0.3) // 70% semantic, 30% keyword
```

Normalizes scores within each result set (dividing by the maximum score), then combines using the specified weights. If fewer weights are provided than result sets, remaining sets receive equal weight.

## Pipeline Composition

The `pipeline` package provides a fluent builder API for chaining retrieval, reranking, and formatting stages.

### Basic Pipeline

```go
p, err := pipeline.NewPipeline().
    Retrieve(myRetriever).
    Build()
if err != nil {
    log.Fatal(err)
}

result, err := p.Execute(ctx, "What is RAG?")
if err != nil {
    log.Fatal(err)
}

for _, doc := range result.Documents {
    fmt.Printf("[%.2f] %s\n", doc.Score, doc.Content)
}
```

### Pipeline with Reranking and Formatting

```go
p, err := pipeline.NewPipeline().
    WithConfig(pipeline.Config{
        RetrievalOpts: retriever.Options{TopK: 20, MinScore: 0.3},
    }).
    Retrieve(hybridRetriever).
    Rerank(reranker.NewMMRReranker(reranker.Config{Lambda: 0.7, TopK: 10})).
    Format(myFormatter).
    Build()
if err != nil {
    log.Fatal(err)
}

result, err := p.Execute(ctx, "Explain RAG pipelines")
// result.Documents contains the retrieved documents
// result.Output contains the formatted output from the last stage
```

### Custom Stages

Add arbitrary processing stages using `StageFunc` or by implementing the `Stage` interface:

```go
// Using StageFunc
filterStage := pipeline.StageFunc(func(
    ctx context.Context,
    input any,
) (any, error) {
    docs := input.([]retriever.Document)
    var filtered []retriever.Document
    for _, doc := range docs {
        if doc.Score > 0.5 {
            filtered = append(filtered, doc)
        }
    }
    return filtered, nil
})

p, err := pipeline.NewPipeline().
    Retrieve(myRetriever).
    AddStage(filterStage).
    Rerank(mmrReranker).
    Format(myFormatter).
    Build()
```

### Passing Query to Stages

Use `pipeline.WithQuery` to make the query string available to custom stages via context:

```go
ctx := pipeline.WithQuery(context.Background(), "my query")
result, err := p.Execute(ctx, "my query")

// Inside a custom stage:
query, ok := pipeline.QueryFromContext(ctx)
```

### Implementing a Formatter

Implement the `FormatterStage` interface to produce final output from retrieved documents:

```go
type MarkdownFormatter struct{}

func (f *MarkdownFormatter) Format(
    ctx context.Context,
    docs []retriever.Document,
) (any, error) {
    var sb strings.Builder
    for i, doc := range docs {
        sb.WriteString(fmt.Sprintf("## Result %d\n\n%s\n\n", i+1, doc.Content))
    }
    return sb.String(), nil
}
```

### Pipeline Configuration

```go
cfg := pipeline.Config{
    RetrievalOpts: retriever.Options{
        TopK:     50,
        MinScore: 0.1,
    },
    MaxStages: 5, // Limit total pipeline stages (0 = unlimited)
}
```

## End-to-End Example

A complete example combining chunking, indexing, hybrid retrieval, reranking, and pipeline composition:

```go
package main

import (
    "context"
    "fmt"
    "log"

    "digital.vasic.rag/pkg/chunker"
    "digital.vasic.rag/pkg/hybrid"
    "digital.vasic.rag/pkg/pipeline"
    "digital.vasic.rag/pkg/reranker"
    "digital.vasic.rag/pkg/retriever"
)

func main() {
    ctx := context.Background()

    // 1. Chunk a large document
    c := chunker.NewRecursiveChunker(chunker.DefaultConfig())
    chunks := c.Chunk(largeDocumentText)

    // 2. Convert chunks to documents and index for keyword search
    docs := make([]retriever.Document, len(chunks))
    for i, ch := range chunks {
        docs[i] = retriever.Document{
            ID:      fmt.Sprintf("chunk-%d", i),
            Content: ch.Content,
            Metadata: map[string]any{
                "start": ch.Start,
                "end":   ch.End,
            },
        }
    }

    kw := hybrid.NewKeywordRetriever()
    kw.Index(docs)

    // 3. Set up hybrid retriever (keyword only in this example;
    //    in production, add a SemanticRetriever backed by a vector DB)
    hybridRet := hybrid.NewHybridRetriever(
        nil, // no semantic retriever in this example
        kw,
        hybrid.NewRRFStrategy(60),
        hybrid.DefaultHybridConfig(),
    )

    // 4. Build and execute the pipeline
    p, err := pipeline.NewPipeline().
        WithConfig(pipeline.Config{
            RetrievalOpts: retriever.Options{TopK: 20},
        }).
        Retrieve(hybridRet).
        Rerank(reranker.NewMMRReranker(reranker.Config{
            Lambda: 0.7,
            TopK:   5,
        })).
        Build()
    if err != nil {
        log.Fatal(err)
    }

    result, err := p.Execute(ctx, "What are the key findings?")
    if err != nil {
        log.Fatal(err)
    }

    fmt.Printf("Found %d documents\n", len(result.Documents))
    for _, doc := range result.Documents {
        fmt.Printf("[%.4f] %s\n", doc.Score, doc.Content[:80])
    }
}
```

## Testing

Run all tests with race detection:

```bash
go test ./... -count=1 -race
```

Run tests for a specific package:

```bash
go test ./pkg/chunker/... -v
go test ./pkg/hybrid/... -v -run TestKeywordRetriever
```
