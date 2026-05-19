# RAG - Retrieval-Augmented Generation Module

`digital.vasic.rag` -- Retrieval-Augmented Generation with document chunking, multi-strategy retrieval, result reranking, hybrid search with fusion, and composable pipeline architecture.

## Overview

RAG is a Go module that provides core primitives for building Retrieval-Augmented Generation systems. It covers the full RAG pipeline from document chunking through retrieval, reranking, and output formatting, with a fluent builder API for composing these stages into executable pipelines.

The module implements three chunking strategies (fixed-size with overlap, recursive separator-based, and sentence-boundary-aware), two retrieval approaches (BM25 keyword search and semantic vector search via a pluggable interface), two reranking algorithms (score-based passthrough and Maximal Marginal Relevance for diversity), and two fusion strategies for hybrid search (Reciprocal Rank Fusion and weighted linear combination).

All components are designed around small, focused interfaces that can be implemented independently. The `MultiRetriever` runs multiple retrievers in parallel with score-based deduplication, while the `HybridRetriever` combines semantic and keyword search with configurable fusion. The pipeline builder chains retrieval, reranking, formatting, and custom stages into a single executable unit.

## Architecture

```
+-------------------------------------------------------------------+
|                        RAG Pipeline                                 |
|                                                                     |
|  Document Ingestion        Retrieval           Post-Processing      |
|  +-----------------+   +---------------+   +-------------------+    |
|  | Chunker         |   | Retriever     |   | Reranker          |    |
|  | - FixedSize     |   | - Multi       |   | - Score           |    |
|  | - Recursive     |   | - Keyword     |   | - MMR (diversity) |    |
|  | - Sentence      |   | - Semantic    |   +-------------------+    |
|  +-----------------+   | - Hybrid      |   | Formatter         |    |
|                        |   (RRF/Linear)|   | (custom stages)   |    |
|                        +---------------+   +-------------------+    |
|                                                                     |
|  Pipeline Builder: NewPipeline().Retrieve(r).Rerank(rr).Build()    |
+-------------------------------------------------------------------+
```

## Package Structure

| Package | Purpose |
|---------|---------|
| `pkg/retriever` | Core retrieval interfaces: `Document`, `Options`, `Retriever`, `MultiRetriever` |
| `pkg/chunker` | Document chunking: `FixedSizeChunker`, `RecursiveChunker`, `SentenceChunker` |
| `pkg/reranker` | Result reranking: `ScoreReranker`, `MMRReranker` (Maximal Marginal Relevance) |
| `pkg/hybrid` | Hybrid retrieval: `KeywordRetriever` (BM25), `SemanticRetriever`, `HybridRetriever`, fusion strategies |
| `pkg/pipeline` | Pipeline composition: fluent `Builder`, `Stage` interface, query context passing |

## API Reference

### Core Types (pkg/retriever)

```go
// Document represents a retrieved document
type Document struct {
    ID       string         `json:"id"`
    Content  string         `json:"content"`
    Metadata map[string]any `json:"metadata,omitempty"`
    Score    float64        `json:"score,omitempty"`
    Source   string         `json:"source,omitempty"`
}

// Options configures retrieval behavior
type Options struct {
    TopK     int            `json:"top_k"`      // Max results (default: 10)
    MinScore float64        `json:"min_score"`   // Minimum relevance score
    Filter   map[string]any `json:"filter,omitempty"`
}

// Retriever defines the interface for document retrieval
type Retriever interface {
    Retrieve(ctx context.Context, query string, opts Options) ([]Document, error)
}

// MultiRetriever combines multiple retrievers with parallel execution and deduplication
func NewMultiRetriever(retrievers ...Retriever) *MultiRetriever
func (m *MultiRetriever) AddRetriever(r Retriever)
func (m *MultiRetriever) Retrieve(ctx, query, opts) ([]Document, error)
```

### Chunker Types (pkg/chunker)

```go
// Chunker splits text into chunks
type Chunker interface {
    Chunk(text string) []Chunk
}

type Chunk struct {
    Content  string         `json:"content"`
    Start    int            `json:"start"`    // byte offset in original text
    End      int            `json:"end"`
    Metadata map[string]any `json:"metadata,omitempty"`
}

type Config struct {
    ChunkSize  int      `json:"chunk_size"`   // Max chunk size in bytes (default: 1000)
    Overlap    int      `json:"overlap"`       // Overlap between chunks (default: 200)
    Separators []string `json:"separators"`    // For recursive splitting
}

func NewFixedSizeChunker(config Config) *FixedSizeChunker
func NewRecursiveChunker(config Config) *RecursiveChunker
func NewSentenceChunker(config Config) *SentenceChunker
```

### Reranker Types (pkg/reranker)

```go
// Reranker reorders documents by relevance
type Reranker interface {
    Rerank(ctx context.Context, query string, docs []retriever.Document) ([]retriever.Document, error)
}

type Config struct {
    Lambda float64 `json:"lambda"` // MMR diversity factor: 0=diverse, 1=relevant (default: 0.5)
    TopK   int     `json:"top_k"`  // Max results (default: 10)
}

func NewScoreReranker(config Config) *ScoreReranker
func NewMMRReranker(config Config) *MMRReranker
```

### Hybrid Search Types (pkg/hybrid)

```go
// FusionStrategy combines results from multiple retrievers
type FusionStrategy interface {
    Fuse(resultSets ...[]retriever.Document) []retriever.Document
}

// RRFStrategy implements Reciprocal Rank Fusion: score = sum(1/(k + rank))
func NewRRFStrategy(k float64) *RRFStrategy  // k default: 60

// LinearStrategy combines with weighted normalized scores
func NewLinearStrategy(weights ...float64) *LinearStrategy

// KeywordRetriever implements BM25 keyword search
func NewKeywordRetriever() *KeywordRetriever
func (r *KeywordRetriever) Index(docs []retriever.Document)
func (r *KeywordRetriever) Remove(id string)
func (r *KeywordRetriever) Retrieve(ctx, query, opts) ([]retriever.Document, error)

// SemanticRetriever wraps any Retriever as semantic search
func NewSemanticRetriever(inner retriever.Retriever) *SemanticRetriever

// HybridRetriever combines semantic + keyword with fusion
func NewHybridRetriever(semantic, keyword retriever.Retriever, fusion FusionStrategy, config HybridConfig) *HybridRetriever
```

### Pipeline Types (pkg/pipeline)

```go
// Stage defines a processing stage
type Stage interface {
    Process(ctx context.Context, input any) (any, error)
}

// StageFunc adapts a function to Stage
type StageFunc func(ctx context.Context, input any) (any, error)

// Pipeline builder (fluent API)
func NewPipeline() *Builder
func (b *Builder) WithConfig(config Config) *Builder
func (b *Builder) Retrieve(r retriever.Retriever) *Builder
func (b *Builder) Rerank(reranker RerankerStage) *Builder
func (b *Builder) Format(formatter FormatterStage) *Builder
func (b *Builder) AddStage(stage Stage) *Builder
func (b *Builder) Build() (*Pipeline, error)

// Pipeline execution
func (p *Pipeline) Execute(ctx context.Context, query string) (*Result, error)

// Context helpers
func WithQuery(ctx context.Context, query string) context.Context
func QueryFromContext(ctx context.Context) (string, bool)
```

## Usage Examples

### Document chunking

```go
config := chunker.Config{ChunkSize: 500, Overlap: 100}

// Fixed-size chunks
fc := chunker.NewFixedSizeChunker(config)
chunks := fc.Chunk(longDocument)

// Recursive splitting by separators
rc := chunker.NewRecursiveChunker(chunker.Config{
    ChunkSize:  500,
    Overlap:    50,
    Separators: []string{"\n\n", "\n", ". ", " "},
})
chunks = rc.Chunk(longDocument)

// Sentence-boundary chunking
sc := chunker.NewSentenceChunker(config)
chunks = sc.Chunk(longDocument)
```

### Hybrid retrieval with RRF fusion

```go
// Set up keyword index
kw := hybrid.NewKeywordRetriever()
kw.Index(documents)

// Wrap vector store as semantic retriever
sem := hybrid.NewSemanticRetriever(vectorStoreRetriever)

// Create hybrid retriever with RRF
hr := hybrid.NewHybridRetriever(
    sem, kw,
    hybrid.NewRRFStrategy(60),
    hybrid.DefaultHybridConfig(),
)

results, err := hr.Retrieve(ctx, "how to handle errors in Go",
    retriever.Options{TopK: 10, MinScore: 0.1})
```

### Complete RAG pipeline

```go
pipe, err := pipeline.NewPipeline().
    Retrieve(hybridRetriever).
    Rerank(reranker.NewMMRReranker(reranker.Config{Lambda: 0.7, TopK: 5})).
    Format(myFormatter).
    Build()

result, err := pipe.Execute(ctx, "explain Go error handling best practices")
// result.Documents -- retrieved and reranked docs
// result.Output    -- formatted output from the formatter stage
```

### Multi-retriever with parallel execution

```go
multi := retriever.NewMultiRetriever(
    vectorRetriever,
    keywordRetriever,
    graphRetriever,
)

// All three run in parallel; results are deduplicated by ID, keeping highest score
docs, err := multi.Retrieve(ctx, query, retriever.DefaultOptions())
```

## Configuration

### Chunker Defaults

| Parameter | Default | Description |
|-----------|---------|-------------|
| ChunkSize | 1000 | Maximum chunk size in bytes |
| Overlap | 200 | Overlap between consecutive chunks |
| Separators | `["\n\n", "\n", ". ", " "]` | Recursive chunker separator hierarchy |

### BM25 Parameters (KeywordRetriever)

| Parameter | Default | Description |
|-----------|---------|-------------|
| k1 | 1.2 | Term frequency saturation |
| b | 0.75 | Document length normalization |

### Hybrid Retrieval Defaults

| Parameter | Default | Description |
|-----------|---------|-------------|
| RRF K constant | 60 | Reciprocal Rank Fusion smoothing |
| PreRetrieveMultiplier | 3 | Retrieve N*topK before fusion |

### MMR Reranker

| Parameter | Default | Description |
|-----------|---------|-------------|
| Lambda | 0.5 | Balance between relevance (1.0) and diversity (0.0) |
| TopK | 10 | Maximum results after reranking |

## Testing

```bash
go test ./... -count=1 -race          # All tests with race detection
go test ./pkg/retriever/... -v        # Retriever tests
go test ./pkg/chunker/... -v          # Chunker tests
go test ./pkg/reranker/... -v         # Reranker tests
go test ./pkg/pipeline/... -v         # Pipeline tests
go test ./pkg/hybrid/... -v           # Hybrid search tests
```

## Integration with HelixAgent

RAG connects to HelixAgent through the adapter at `internal/adapters/rag/`:

- **Retrieval Pipeline**: HelixAgent's `/v1/rag` endpoint uses the pipeline builder to compose retrieval, reranking, and formatting stages dynamically based on request parameters.
- **Hybrid Search**: The hybrid retriever combines vector search (via the VectorDB module's Qdrant/Pinecone/Milvus adapters) with BM25 keyword search for improved recall.
- **Debate Context**: Retrieved documents are injected into debate turns as context, allowing debate participants to ground their arguments in factual sources.
- **Memory Integration**: RAG chunking and retrieval work alongside the Memory and HelixMemory modules to provide both semantic search over documents and entity-graph-based memory recall.
- **Embedding Pipeline**: The Embeddings module provides vector representations for the semantic retriever, supporting 6 embedding providers (OpenAI, Cohere, Voyage, Jina, Google, Bedrock).

---

## Anti-Bluff Guarantees (round-278 deep-doc — mirror of round-220 template)

This module ships under the **HelixCode Constitution v2.3.0+** and inherits
the parent-repo prime directive recorded in this submodule's `CLAUDE.md`:

> Verbatim 2026-05-19 operator mandate: *"all existing tests and Challenges
> do work in anti-bluff manner — they MUST confirm that all tested codebase
> really works as expected! We had been in position that all tests do execute
> with success and all Challenges as well, but in reality the most of the
> features does not work and can't be used! This MUST NOT be the case and
> execution of tests and Challenges MUST guarantee the quality, the
> completition and full usability by end users of the product!"*

### What runtime evidence this module produces

Every test type in this submodule is paired with **positive runtime evidence**
captured during execution — never a `assert.True(t, true)`, never a
metadata-only PASS, never a grep-based "the symbol exists" claim:

| Anti-bluff invariant (Article XI §11.9) | How this module satisfies it |
|---|---|
| **No simulation in production code** | `pkg/chunker`, `pkg/retriever`, `pkg/reranker`, `pkg/hybrid`, `pkg/pipeline` contain only real implementations. Anti-bluff scan: `grep -rn 'simulated\|for now\|TODO implement\|placeholder' pkg/ \| grep -v _test.go` returns empty. |
| **No mocks beyond unit tests** (CONST-050(A)) | Unit tests (`*_test.go` in `pkg/`) use `testify` mocks/stubs only. Integration/E2E/security/stress (`tests/integration`, `tests/e2e`, `tests/security`, `tests/stress`) and Challenges exercise real chunker/retriever/reranker/hybrid/pipeline code paths with real text inputs and real document corpora. |
| **End-to-end usability proof** | `challenges/runner/main.go` (added in round-278) builds a real pipeline (`NewPipeline().Retrieve(r).Rerank(rr).Build()`), feeds it 5-locale bilingual prompts loaded from `tests/fixtures/rag/payloads.json`, and asserts that every locale produces a non-empty ranked-document list with stable score ordering. |
| **Paired-mutation gate** (§11.4 / CONST-035) | `challenges/rag_describe_challenge.sh` produces a clean exit 0 on a healthy tree and exit 99 when invoked with `--anti-bluff-mutate`, proving the gate distinguishes truth from corruption rather than always passing. |
| **CONST-046 no-hardcoded-content** | Fixture-driven invocation: all user-facing query text loaded from `tests/fixtures/rag/payloads.json` rather than baked into Go source. Five locales (en, sr, ja, es, de) cover the Article XI §11.9 multilingual usability principle. |
| **CONST-053 repository hygiene** | `.gitignore` covers binaries (`*.exe`, `*.test`), Go workspace (`go.work*`), coverage outputs (`coverage.out`, `coverage.html`), OS/IDE state (`.idea/`, `.vscode/`, `.DS_Store`), and run artefacts (`*.out`). Round-278 verifies no forbidden-class file is currently tracked. |
| **CONST-060 fetch-before-edit** | The round-278 commit recorded a captured `git fetch --all --prune` + `HEAD..@{u}` log proving the submodule was synced before edits. |

### How to verify these guarantees locally

```bash
# 1. Unit tests with race detector — all five packages green
cd dependencies/vasic-digital/RAG && go test -race -count=1 ./...

# 2. Anti-bluff smoke scan over production source
grep -rn 'simulated\|for now\|TODO implement\|placeholder' pkg/ \
  | grep -v _test.go && echo "BLUFF FOUND" || echo "clean"

# 3. Round-278 Challenge: real RAG exerciser, 5-locale bilingual
go run ./challenges/runner

# 4. Paired-mutation gate (proves gate fails when fed a lie)
bash challenges/rag_describe_challenge.sh                  # → exit 0
bash challenges/rag_describe_challenge.sh --anti-bluff-mutate  # → exit 99

# 5. Symbol → test ledger (round-278 deep-doc)
cat docs/test-coverage.md
```

### What the round-278 enrichment added

- `README.md` anti-bluff guarantees section (this block).
- `docs/test-coverage.md` — CONST-050(B) symbol → test ledger covering every
  exported type/function across `pkg/chunker`, `pkg/retriever`, `pkg/reranker`,
  `pkg/hybrid`, `pkg/pipeline` with the unit / integration / Challenge that
  exercises it.
- `challenges/runner/main.go` — real RAG exerciser building an actual
  `Pipeline` and processing 5-locale fixtures end-to-end with stable
  ordering assertions.
- `challenges/rag_describe_challenge.sh` — paired-mutation Challenge with
  honest exit-99 on `--anti-bluff-mutate`.
- `tests/fixtures/rag/payloads.json` — 5-locale bilingual query corpus.

### What round-278 explicitly does NOT claim

- This module does not ship its own vector store; the `pkg/hybrid`
  `SemanticRetriever` is an interface that consumers fulfil with their
  preferred backend (Qdrant, Pinecone, Milvus, etc.).
- This module does not ship its own embedding provider; consumers wire
  one in via the parent project's `Embeddings` module.
- The Challenge runner exercises in-memory corpora — for full
  production wire-evidence consult the parent `helix_qa` autonomous
  session bank.
