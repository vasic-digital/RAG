package security

import (
	"context"
	"strings"
	"testing"

	"digital.vasic.rag/pkg/chunker"
	"digital.vasic.rag/pkg/hybrid"
	"digital.vasic.rag/pkg/pipeline"
	"digital.vasic.rag/pkg/reranker"
	"digital.vasic.rag/pkg/retriever"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmptyInputHandling(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping security test in short mode")
	}

	fixedChunker := chunker.NewFixedSizeChunker(chunker.DefaultConfig())
	assert.Nil(t, fixedChunker.Chunk(""))

	recursiveChunker := chunker.NewRecursiveChunker(chunker.DefaultConfig())
	assert.Nil(t, recursiveChunker.Chunk(""))

	sentenceChunker := chunker.NewSentenceChunker(chunker.DefaultConfig())
	assert.Nil(t, sentenceChunker.Chunk(""))
}

func TestEmptyDocumentReranking(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping security test in short mode")
	}

	ctx := context.Background()

	scoreRR := reranker.NewScoreReranker(reranker.DefaultConfig())
	result, err := scoreRR.Rerank(ctx, "query", nil)
	require.NoError(t, err)
	assert.Empty(t, result)

	result, err = scoreRR.Rerank(ctx, "query", []retriever.Document{})
	require.NoError(t, err)
	assert.Empty(t, result)

	mmrRR := reranker.NewMMRReranker(reranker.DefaultConfig())
	result, err = mmrRR.Rerank(ctx, "query", nil)
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestNoRetrieversPipeline(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping security test in short mode")
	}

	_, err := pipeline.NewPipeline().Build()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "requires a retriever")
}

func TestNilRetrieverInPipeline(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping security test in short mode")
	}

	_, err := pipeline.NewPipeline().
		Retrieve(nil).
		Build()
	assert.Error(t, err)
}

func TestNilRerankerInPipeline(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping security test in short mode")
	}

	kr := hybrid.NewKeywordRetriever()
	kr.Index([]retriever.Document{
		{ID: "1", Content: "test content"},
	})

	_, err := pipeline.NewPipeline().
		Retrieve(kr).
		Rerank(nil).
		Build()
	assert.Error(t, err)
}

func TestLargeInputChunking(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping security test in short mode")
	}

	largeText := strings.Repeat("word ", 100000)
	c := chunker.NewFixedSizeChunker(chunker.Config{
		ChunkSize: 1000,
		Overlap:   100,
	})
	chunks := c.Chunk(largeText)
	assert.NotEmpty(t, chunks)

	for _, ch := range chunks {
		assert.LessOrEqual(t, len(ch.Content), 1000)
		assert.NotEmpty(t, ch.Content)
	}
}

func TestMultiRetrieverAllFail(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping security test in short mode")
	}

	// Empty retrievers with no data should return an error
	multi := retriever.NewMultiRetriever()
	ctx := context.Background()
	_, err := multi.Retrieve(ctx, "query", retriever.DefaultOptions())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no retrievers configured")
}

func TestChunkerEdgeCaseConfigs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping security test in short mode")
	}

	// Zero chunk size should be corrected to default
	c := chunker.NewFixedSizeChunker(chunker.Config{
		ChunkSize: 0,
		Overlap:   0,
	})
	chunks := c.Chunk("hello world")
	assert.NotNil(t, chunks)

	// Overlap >= ChunkSize should be corrected
	c = chunker.NewFixedSizeChunker(chunker.Config{
		ChunkSize: 10,
		Overlap:   20,
	})
	chunks = c.Chunk("hello world this is a test of chunking")
	assert.NotNil(t, chunks)

	// Negative overlap should be corrected to 0
	c = chunker.NewFixedSizeChunker(chunker.Config{
		ChunkSize: 10,
		Overlap:   -5,
	})
	chunks = c.Chunk("hello world")
	assert.NotNil(t, chunks)
}

func TestMMRLambdaBoundaries(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping security test in short mode")
	}

	ctx := context.Background()
	docs := []retriever.Document{
		{ID: "1", Content: "alpha beta", Score: 0.9},
		{ID: "2", Content: "gamma delta", Score: 0.7},
	}

	// Lambda = 0 (max diversity)
	mmr0 := reranker.NewMMRReranker(reranker.Config{Lambda: 0, TopK: 2})
	result, err := mmr0.Rerank(ctx, "alpha", docs)
	require.NoError(t, err)
	assert.NotEmpty(t, result)

	// Lambda = 1 (max relevance)
	mmr1 := reranker.NewMMRReranker(reranker.Config{Lambda: 1, TopK: 2})
	result, err = mmr1.Rerank(ctx, "alpha", docs)
	require.NoError(t, err)
	assert.NotEmpty(t, result)
}
