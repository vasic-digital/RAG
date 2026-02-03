# API Reference

Complete reference for all exported types, functions, and methods in the `digital.vasic.rag` module.

---

## Package `retriever`

```
import "digital.vasic.rag/pkg/retriever"
```

Core retrieval interfaces and types for RAG systems.

### Types

#### Document

```go
type Document struct {
    ID       string         `json:"id"`
    Content  string         `json:"content"`
    Metadata map[string]any `json:"metadata,omitempty"`
    Score    float64        `json:"score,omitempty"`
    Source   string         `json:"source,omitempty"`
}
```

Represents a retrieved document. `ID` is used for deduplication in `MultiRetriever`. `Score` is set by retrievers and rerankers to indicate relevance. `Metadata` carries arbitrary key-value pairs. `Source` identifies where the document was retrieved from.

#### Options

```go
type Options struct {
    TopK     int            `json:"top_k"`
    MinScore float64        `json:"min_score"`
    Filter   map[string]any `json:"filter,omitempty"`
}
```

Configures retrieval behavior. `TopK` limits the number of returned documents. `MinScore` filters out documents below the threshold. `Filter` provides optional metadata-based filtering (interpretation is retriever-specific).

#### Retriever

```go
type Retriever interface {
    Retrieve(ctx context.Context, query string, opts Options) ([]Document, error)
}
```

The core retrieval interface. Implementations search for documents matching the query and return them ordered by relevance.

#### MultiRetriever

```go
type MultiRetriever struct {
    // contains unexported fields
}
```

Combines multiple retrievers, querying them in parallel and merging results with deduplication. Thread-safe.

### Functions

#### DefaultOptions

```go
func DefaultOptions() Options
```

Returns default retrieval options: TopK=10, MinScore=0.0, Filter=nil.

#### NewMultiRetriever

```go
func NewMultiRetriever(retrievers ...Retriever) *MultiRetriever
```

Creates a `MultiRetriever` from the given retrievers. Accepts zero or more retrievers. If zero, `Retrieve` will return an error until retrievers are added via `AddRetriever`.

### Methods

#### (*MultiRetriever) AddRetriever

```go
func (m *MultiRetriever) AddRetriever(r Retriever)
```

Adds a retriever to the multi-retriever. Thread-safe; may be called concurrently with `Retrieve`.

#### (*MultiRetriever) Retrieve

```go
func (m *MultiRetriever) Retrieve(
    ctx context.Context,
    query string,
    opts Options,
) ([]Document, error)
```

Queries all underlying retrievers in parallel. Deduplicates results by document ID, keeping the document with the highest score. Returns results sorted by score descending, limited to `opts.TopK` and filtered by `opts.MinScore`. Returns an error if no retrievers are configured or if all retrievers fail.

---

## Package `chunker`

```
import "digital.vasic.rag/pkg/chunker"
```

Document chunking strategies for splitting text into smaller pieces.

### Types

#### Chunk

```go
type Chunk struct {
    Content  string         `json:"content"`
    Start    int            `json:"start"`
    End      int            `json:"end"`
    Metadata map[string]any `json:"metadata,omitempty"`
}
```

A piece of text produced by a `Chunker`. `Start` and `End` are byte offsets into the original text. `Metadata` is available for caller use (not populated by built-in chunkers).

#### Chunker

```go
type Chunker interface {
    Chunk(text string) []Chunk
}
```

The interface for splitting text into chunks. Returns nil for empty text.

#### Config

```go
type Config struct {
    ChunkSize  int      `json:"chunk_size"`
    Overlap    int      `json:"overlap"`
    Separators []string `json:"separators,omitempty"`
}
```

Configuration for chunkers. `ChunkSize` is the maximum chunk size in bytes. `Overlap` is the number of overlapping bytes between consecutive chunks. `Separators` is the ordered list of separator strings used by `RecursiveChunker`.

#### FixedSizeChunker

```go
type FixedSizeChunker struct {
    // contains unexported fields
}
```

Splits text into fixed-size chunks with configurable overlap using a sliding window.

#### RecursiveChunker

```go
type RecursiveChunker struct {
    // contains unexported fields
}
```

Splits text by trying separators in order, falling back to the next separator when chunks are too large. Falls back to fixed-size splitting when all separators are exhausted.

#### SentenceChunker

```go
type SentenceChunker struct {
    // contains unexported fields
}
```

Splits text by sentence boundaries (`.`, `!`, `?` followed by whitespace), grouping sentences into chunks within the configured size limit.

### Functions

#### DefaultConfig

```go
func DefaultConfig() Config
```

Returns default chunker configuration: ChunkSize=1000, Overlap=200, Separators=["\n\n", "\n", ". ", " "].

#### NewFixedSizeChunker

```go
func NewFixedSizeChunker(config Config) *FixedSizeChunker
```

Creates a `FixedSizeChunker`. Clamps invalid values: ChunkSize defaults to 1000 if <= 0, Overlap is clamped to 0 if negative, and to ChunkSize/4 if >= ChunkSize.

#### NewRecursiveChunker

```go
func NewRecursiveChunker(config Config) *RecursiveChunker
```

Creates a `RecursiveChunker`. Applies the same defaults as `NewFixedSizeChunker`, plus defaults Separators to ["\n\n", "\n", ". ", " "] if empty.

#### NewSentenceChunker

```go
func NewSentenceChunker(config Config) *SentenceChunker
```

Creates a `SentenceChunker`. ChunkSize defaults to 1000 if <= 0. Overlap is clamped to 0 if negative.

### Methods

#### (*FixedSizeChunker) Chunk

```go
func (c *FixedSizeChunker) Chunk(text string) []Chunk
```

Splits text into fixed-size chunks. Step size is ChunkSize - Overlap. Returns nil for empty text. If text fits in a single chunk, returns a single-element slice.

#### (*RecursiveChunker) Chunk

```go
func (c *RecursiveChunker) Chunk(text string) []Chunk
```

Splits text recursively using separators. Empty chunks are discarded. Results include byte-accurate Start/End offsets relative to the original text.

#### (*SentenceChunker) Chunk

```go
func (c *SentenceChunker) Chunk(text string) []Chunk
```

Splits text into chunks aligned to sentence boundaries. Supports overlap by retaining trailing text from the previous chunk. Returns nil for empty text.

---

## Package `reranker`

```
import "digital.vasic.rag/pkg/reranker"
```

Result reranking strategies for improving retrieval quality.

### Types

#### Reranker

```go
type Reranker interface {
    Rerank(
        ctx context.Context,
        query string,
        docs []retriever.Document,
    ) ([]retriever.Document, error)
}
```

The interface for reranking retrieved documents. Implementations reorder documents based on relevance to the query.

#### Config

```go
type Config struct {
    Lambda float64 `json:"lambda"`
    TopK   int     `json:"top_k"`
}
```

Configuration for rerankers. `Lambda` controls the relevance/diversity trade-off in MMR (0.0=max diversity, 1.0=max relevance). `TopK` limits the number of results.

#### ScoreReranker

```go
type ScoreReranker struct {
    // contains unexported fields
}
```

Reranks documents by their existing `Score` field (passthrough). Sorts descending and limits to TopK.

#### MMRReranker

```go
type MMRReranker struct {
    // contains unexported fields
}
```

Implements Maximal Marginal Relevance for balancing relevance and diversity. Uses Jaccard word-overlap similarity.

### Functions

#### DefaultConfig

```go
func DefaultConfig() Config
```

Returns default reranker configuration: Lambda=0.5, TopK=10.

#### NewScoreReranker

```go
func NewScoreReranker(config Config) *ScoreReranker
```

Creates a `ScoreReranker`. TopK defaults to 10 if <= 0.

#### NewMMRReranker

```go
func NewMMRReranker(config Config) *MMRReranker
```

Creates an `MMRReranker`. TopK defaults to 10 if <= 0. Lambda is clamped to [0.0, 1.0].

### Methods

#### (*ScoreReranker) Rerank

```go
func (r *ScoreReranker) Rerank(
    ctx context.Context,
    query string,
    docs []retriever.Document,
) ([]retriever.Document, error)
```

Sorts documents by `Score` descending and limits to TopK. Returns a copy; does not modify the input slice. Returns the input unchanged if empty.

#### (*MMRReranker) Rerank

```go
func (r *MMRReranker) Rerank(
    ctx context.Context,
    query string,
    docs []retriever.Document,
) ([]retriever.Document, error)
```

Selects documents using the MMR algorithm: `MMR(d) = Lambda * Sim(d, query) - (1 - Lambda) * max(Sim(d, d_selected))`. Documents are selected greedily. Scores in the returned documents are set to the query-document similarity. Returns the input unchanged if empty.

---

## Package `pipeline`

```
import "digital.vasic.rag/pkg/pipeline"
```

RAG pipeline composition with a fluent builder API.

### Types

#### Stage

```go
type Stage interface {
    Process(ctx context.Context, input any) (any, error)
}
```

A processing stage in the pipeline. Receives the output of the previous stage (or `[]retriever.Document` from retrieval) and returns processed output.

#### StageFunc

```go
type StageFunc func(ctx context.Context, input any) (any, error)
```

Adapts a plain function to the `Stage` interface.

#### Config

```go
type Config struct {
    RetrievalOpts retriever.Options `json:"retrieval_opts"`
    MaxStages     int               `json:"max_stages"`
}
```

Pipeline configuration. `RetrievalOpts` is passed to the retriever. `MaxStages` limits the number of stages (0 = unlimited).

#### Pipeline

```go
type Pipeline struct {
    // contains unexported fields
}
```

Chains a retriever with multiple processing stages. Immutable after construction via `Builder`.

#### Result

```go
type Result struct {
    Documents []retriever.Document `json:"documents"`
    Output    any                  `json:"output,omitempty"`
}
```

Output of a pipeline execution. `Documents` contains the initially retrieved documents. `Output` contains the final output from the last stage (or `[]retriever.Document` if no stages are configured).

#### Builder

```go
type Builder struct {
    // contains unexported fields
}
```

Fluent API for constructing a `Pipeline`.

#### RerankerStage

```go
type RerankerStage interface {
    Rerank(
        ctx context.Context,
        query string,
        docs []retriever.Document,
    ) ([]retriever.Document, error)
}
```

The reranking contract for pipeline integration. Compatible with `reranker.Reranker`.

#### FormatterStage

```go
type FormatterStage interface {
    Format(ctx context.Context, docs []retriever.Document) (any, error)
}
```

The formatting contract for pipeline integration. Receives documents and produces formatted output.

### Functions

#### NewPipeline

```go
func NewPipeline() *Builder
```

Starts building a new Pipeline. Returns a `Builder` with default configuration.

#### DefaultConfig

```go
func DefaultConfig() Config
```

Returns default pipeline configuration: RetrievalOpts=DefaultOptions(), MaxStages=0.

#### WithQuery

```go
func WithQuery(ctx context.Context, query string) context.Context
```

Returns a context carrying the query string for pipeline stages.

#### QueryFromContext

```go
func QueryFromContext(ctx context.Context) (string, bool)
```

Extracts the query string from context. Returns the query and true if present, or empty string and false otherwise.

### Methods

#### (StageFunc) Process

```go
func (f StageFunc) Process(ctx context.Context, input any) (any, error)
```

Calls the underlying function.

#### (*Builder) WithConfig

```go
func (b *Builder) WithConfig(config Config) *Builder
```

Sets the pipeline configuration. Returns the builder for chaining.

#### (*Builder) Retrieve

```go
func (b *Builder) Retrieve(r retriever.Retriever) *Builder
```

Sets the retriever for the pipeline. Required. Returns an error on `Build()` if nil.

#### (*Builder) Rerank

```go
func (b *Builder) Rerank(reranker RerankerStage) *Builder
```

Adds a reranking stage. The stage expects `[]retriever.Document` as input. Returns an error on `Build()` if nil.

#### (*Builder) Format

```go
func (b *Builder) Format(formatter FormatterStage) *Builder
```

Adds a formatting stage. The stage expects `[]retriever.Document` as input and returns formatted output. Returns an error on `Build()` if nil.

#### (*Builder) AddStage

```go
func (b *Builder) AddStage(stage Stage) *Builder
```

Adds a custom processing stage. Returns an error on `Build()` if nil.

#### (*Builder) Build

```go
func (b *Builder) Build() (*Pipeline, error)
```

Constructs the `Pipeline`. Returns an error if: no retriever is set, any stage is nil, or MaxStages is exceeded.

#### (*Pipeline) Execute

```go
func (p *Pipeline) Execute(
    ctx context.Context,
    query string,
) (*Result, error)
```

Runs the pipeline: retrieves documents using the configured retriever and options, then passes them through each stage in order. Returns a `Result` containing the retrieved documents and the final stage output. Returns an error if retrieval or any stage fails.

---

## Package `hybrid`

```
import "digital.vasic.rag/pkg/hybrid"
```

Hybrid retrieval combining semantic and keyword search with fusion strategies.

### Types

#### FusionStrategy

```go
type FusionStrategy interface {
    Fuse(resultSets ...[]retriever.Document) []retriever.Document
}
```

Defines how to combine results from multiple retrievers. Returns merged documents sorted by fused score descending.

#### RRFStrategy

```go
type RRFStrategy struct {
    K float64
}
```

Reciprocal Rank Fusion. Computes `score(d) = sum(1 / (K + rank(d)))` across all result sets. `K` is typically 60.

#### LinearStrategy

```go
type LinearStrategy struct {
    Weights []float64
}
```

Weighted linear combination. Normalizes scores within each result set (by max score), then combines using weights. If fewer weights than result sets, remaining sets get equal weight (`1 / len(resultSets)`).

#### KeywordRetriever

```go
type KeywordRetriever struct {
    // contains unexported fields
}
```

BM25-style keyword retrieval with in-memory inverted index. Thread-safe. BM25 parameters: k1=1.2, b=0.75.

#### SemanticRetriever

```go
type SemanticRetriever struct {
    // contains unexported fields
}
```

Wraps any `retriever.Retriever` to provide a semantic search interface. Delegates all calls to the inner retriever.

#### HybridRetriever

```go
type HybridRetriever struct {
    // contains unexported fields
}
```

Combines semantic and keyword retrievers, querying both in parallel and fusing results.

#### HybridConfig

```go
type HybridConfig struct {
    PreRetrieveMultiplier int `json:"pre_retrieve_multiplier"`
}
```

Configuration for hybrid retrieval. `PreRetrieveMultiplier` causes each sub-retriever to fetch `TopK * Multiplier` results before fusion (default: 3).

### Functions

#### NewRRFStrategy

```go
func NewRRFStrategy(k float64) *RRFStrategy
```

Creates an RRF fusion strategy. `k` defaults to 60 if <= 0.

#### NewLinearStrategy

```go
func NewLinearStrategy(weights ...float64) *LinearStrategy
```

Creates a linear fusion strategy with given weights.

#### NewKeywordRetriever

```go
func NewKeywordRetriever() *KeywordRetriever
```

Creates a new BM25-based keyword retriever with an empty index.

#### NewSemanticRetriever

```go
func NewSemanticRetriever(inner retriever.Retriever) *SemanticRetriever
```

Wraps an existing `Retriever` as a semantic retriever.

#### NewHybridRetriever

```go
func NewHybridRetriever(
    semantic *SemanticRetriever,
    keyword *KeywordRetriever,
    fusion FusionStrategy,
    config HybridConfig,
) *HybridRetriever
```

Creates a hybrid retriever. If `fusion` is nil, defaults to `RRFStrategy` with k=60. If `PreRetrieveMultiplier` is <= 0, defaults to 3. Either `semantic` or `keyword` (or both) may be nil; nil retrievers are skipped during retrieval.

#### DefaultHybridConfig

```go
func DefaultHybridConfig() HybridConfig
```

Returns default hybrid configuration: PreRetrieveMultiplier=3.

### Methods

#### (*RRFStrategy) Fuse

```go
func (r *RRFStrategy) Fuse(
    resultSets ...[]retriever.Document,
) []retriever.Document
```

Merges result sets using Reciprocal Rank Fusion. Documents appearing in multiple sets accumulate scores. Returns results sorted by fused score descending.

#### (*LinearStrategy) Fuse

```go
func (l *LinearStrategy) Fuse(
    resultSets ...[]retriever.Document,
) []retriever.Document
```

Merges result sets using weighted linear combination. Normalizes scores within each set by dividing by the max score. Returns results sorted by combined score descending.

#### (*KeywordRetriever) Index

```go
func (r *KeywordRetriever) Index(docs []retriever.Document)
```

Adds documents to the BM25 index. Tokenizes content, builds term frequency and document frequency maps, and updates average document length. Thread-safe.

#### (*KeywordRetriever) Remove

```go
func (r *KeywordRetriever) Remove(id string)
```

Removes a document from the index by ID. Updates all frequency maps. No-op if the ID does not exist. Thread-safe.

#### (*KeywordRetriever) Retrieve

```go
func (r *KeywordRetriever) Retrieve(
    ctx context.Context,
    query string,
    opts retriever.Options,
) ([]retriever.Document, error)
```

Performs BM25 keyword search. Tokenizes the query, computes BM25 scores for all matching documents, filters by `MinScore`, sorts by score descending, and limits to `TopK`. Thread-safe.

#### (*SemanticRetriever) Retrieve

```go
func (s *SemanticRetriever) Retrieve(
    ctx context.Context,
    query string,
    opts retriever.Options,
) ([]retriever.Document, error)
```

Delegates to the inner retriever.

#### (*HybridRetriever) Retrieve

```go
func (h *HybridRetriever) Retrieve(
    ctx context.Context,
    query string,
    opts retriever.Options,
) ([]retriever.Document, error)
```

Performs hybrid search. Queries semantic and keyword retrievers in parallel with `TopK * PreRetrieveMultiplier`, fuses results using the configured strategy, applies `MinScore` filter, and limits to `TopK`. Returns an error only if both retrievers fail.
