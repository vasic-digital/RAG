package integration

import (
	"context"
	"testing"

	"digital.vasic.rag/pkg/chunker"
	"digital.vasic.rag/pkg/hybrid"
	"digital.vasic.rag/pkg/pipeline"
	"digital.vasic.rag/pkg/reranker"
	"digital.vasic.rag/pkg/retriever"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChunkerRerankerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")  // SKIP-OK: #short-mode
	}

	text := "Machine learning is a subset of artificial intelligence. " +
		"Deep learning uses neural networks with many layers. " +
		"Natural language processing deals with text analysis. " +
		"Computer vision handles image recognition tasks."

	c := chunker.NewSentenceChunker(chunker.Config{
		ChunkSize: 80,
		Overlap:   0,
	})
	chunks := c.Chunk(text)
	require.NotEmpty(t, chunks)

	docs := make([]retriever.Document, len(chunks))
	for i, ch := range chunks {
		docs[i] = retriever.Document{
			ID:      "doc-" + string(rune('a'+i)),
			Content: ch.Content,
			Score:   float64(len(chunks)-i) * 0.2,
		}
	}

	rr := reranker.NewScoreReranker(reranker.Config{TopK: 3})
	ctx := context.Background()
	reranked, err := rr.Rerank(ctx, "neural networks", docs)
	require.NoError(t, err)
	require.NotEmpty(t, reranked)

	for i := 1; i < len(reranked); i++ {
		assert.GreaterOrEqual(t, reranked[i-1].Score, reranked[i].Score)
	}
}

func TestKeywordRetrieverRerankerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")  // SKIP-OK: #short-mode
	}

	kr := hybrid.NewKeywordRetriever()
	kr.Index([]retriever.Document{
		{ID: "doc1", Content: "Go programming language concurrency goroutines channels"},
		{ID: "doc2", Content: "Python programming language data science machine learning"},
		{ID: "doc3", Content: "Rust programming language memory safety ownership"},
		{ID: "doc4", Content: "JavaScript web development frontend React Vue"},
	})

	ctx := context.Background()
	results, err := kr.Retrieve(ctx, "programming language", retriever.Options{
		TopK:     4,
		MinScore: 0.0,
	})
	require.NoError(t, err)
	require.NotEmpty(t, results)

	mmr := reranker.NewMMRReranker(reranker.Config{
		Lambda: 0.5,
		TopK:   3,
	})
	reranked, err := mmr.Rerank(ctx, "programming language", results)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(reranked), 3)
}

func TestHybridRetrieverIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")  // SKIP-OK: #short-mode
	}

	kr := hybrid.NewKeywordRetriever()
	docs := []retriever.Document{
		{ID: "d1", Content: "vector database embeddings similarity search"},
		{ID: "d2", Content: "relational database SQL queries optimization"},
		{ID: "d3", Content: "graph database nodes edges traversal"},
	}
	kr.Index(docs)

	semanticKR := hybrid.NewKeywordRetriever()
	semanticKR.Index(docs)
	semantic := hybrid.NewSemanticRetriever(semanticKR)

	fusion := hybrid.NewRRFStrategy(60)
	hr := hybrid.NewHybridRetriever(
		semantic, kr, fusion, hybrid.DefaultHybridConfig(),
	)

	ctx := context.Background()
	results, err := hr.Retrieve(ctx, "database", retriever.Options{
		TopK:     3,
		MinScore: 0.0,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, results)
	assert.LessOrEqual(t, len(results), 3)
}

func TestPipelineWithRetrieverAndRerankerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")  // SKIP-OK: #short-mode
	}

	kr := hybrid.NewKeywordRetriever()
	kr.Index([]retriever.Document{
		{ID: "1", Content: "kubernetes container orchestration pods services"},
		{ID: "2", Content: "docker container images dockerfile build"},
		{ID: "3", Content: "podman rootless containers security"},
	})

	rr := reranker.NewScoreReranker(reranker.Config{TopK: 2})

	p, err := pipeline.NewPipeline().
		Retrieve(kr).
		Rerank(rr).
		Build()
	require.NoError(t, err)

	ctx := pipeline.WithQuery(context.Background(), "container orchestration")
	result, err := p.Execute(ctx, "container orchestration")
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotEmpty(t, result.Documents)
}

func TestChunkerVariantsIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")  // SKIP-OK: #short-mode
	}

	text := "First paragraph with some content.\n\n" +
		"Second paragraph with different content.\n\n" +
		"Third paragraph with more content."

	fixedChunker := chunker.NewFixedSizeChunker(chunker.Config{
		ChunkSize: 50,
		Overlap:   10,
	})
	fixedChunks := fixedChunker.Chunk(text)

	recursiveChunker := chunker.NewRecursiveChunker(chunker.Config{
		ChunkSize:  50,
		Overlap:    10,
		Separators: []string{"\n\n", "\n", ". ", " "},
	})
	recursiveChunks := recursiveChunker.Chunk(text)

	sentenceChunker := chunker.NewSentenceChunker(chunker.Config{
		ChunkSize: 50,
		Overlap:   0,
	})
	sentenceChunks := sentenceChunker.Chunk(text)

	assert.NotEmpty(t, fixedChunks)
	assert.NotEmpty(t, recursiveChunks)
	assert.NotEmpty(t, sentenceChunks)

	for _, ch := range fixedChunks {
		assert.NotEmpty(t, ch.Content)
		assert.GreaterOrEqual(t, ch.End, ch.Start)
	}
}

func TestFusionStrategiesIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")  // SKIP-OK: #short-mode
	}

	set1 := []retriever.Document{
		{ID: "a", Content: "alpha", Score: 0.9},
		{ID: "b", Content: "beta", Score: 0.7},
		{ID: "c", Content: "gamma", Score: 0.5},
	}
	set2 := []retriever.Document{
		{ID: "b", Content: "beta", Score: 0.8},
		{ID: "d", Content: "delta", Score: 0.6},
		{ID: "a", Content: "alpha", Score: 0.4},
	}

	rrf := hybrid.NewRRFStrategy(60)
	rrfResults := rrf.Fuse(set1, set2)
	assert.NotEmpty(t, rrfResults)

	linear := hybrid.NewLinearStrategy(0.6, 0.4)
	linearResults := linear.Fuse(set1, set2)
	assert.NotEmpty(t, linearResults)

	for i := 1; i < len(rrfResults); i++ {
		assert.GreaterOrEqual(t, rrfResults[i-1].Score, rrfResults[i].Score)
	}
	for i := 1; i < len(linearResults); i++ {
		assert.GreaterOrEqual(t, linearResults[i-1].Score, linearResults[i].Score)
	}
}
