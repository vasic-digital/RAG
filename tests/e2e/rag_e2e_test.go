package e2e

import (
	"context"
	"fmt"
	"testing"

	"digital.vasic.rag/pkg/chunker"
	"digital.vasic.rag/pkg/hybrid"
	"digital.vasic.rag/pkg/pipeline"
	"digital.vasic.rag/pkg/reranker"
	"digital.vasic.rag/pkg/retriever"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFullRAGPipelineE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	// Step 1: Chunk a large document
	document := "Retrieval-Augmented Generation combines information retrieval with " +
		"language model generation. The retrieval component searches a knowledge base " +
		"for relevant documents. The generation component uses these documents as context " +
		"to produce more accurate and grounded responses. RAG systems are widely used in " +
		"question answering, fact verification, and knowledge-intensive tasks. " +
		"Vector databases store document embeddings for fast similarity search. " +
		"Reranking improves result quality by applying a second-stage relevance model."

	c := chunker.NewRecursiveChunker(chunker.Config{
		ChunkSize:  120,
		Overlap:    20,
		Separators: []string{". ", " "},
	})
	chunks := c.Chunk(document)
	require.NotEmpty(t, chunks)

	// Step 2: Index chunks into keyword retriever
	docs := make([]retriever.Document, len(chunks))
	for i, ch := range chunks {
		docs[i] = retriever.Document{
			ID:       fmt.Sprintf("chunk-%d", i),
			Content:  ch.Content,
			Source:   "document",
			Metadata: map[string]any{"start": ch.Start, "end": ch.End},
		}
	}

	kr := hybrid.NewKeywordRetriever()
	kr.Index(docs)

	// Step 3: Query through pipeline
	rr := reranker.NewScoreReranker(reranker.Config{TopK: 3})

	p, err := pipeline.NewPipeline().
		Retrieve(kr).
		Rerank(rr).
		Build()
	require.NoError(t, err)

	ctx := pipeline.WithQuery(context.Background(), "retrieval knowledge base")
	result, err := p.Execute(ctx, "retrieval knowledge base")
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotEmpty(t, result.Documents)
}

func TestMultiRetrieverE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	kr1 := hybrid.NewKeywordRetriever()
	kr1.Index([]retriever.Document{
		{ID: "a", Content: "Go concurrency goroutines channels"},
		{ID: "b", Content: "Rust memory safety ownership borrowing"},
	})

	kr2 := hybrid.NewKeywordRetriever()
	kr2.Index([]retriever.Document{
		{ID: "c", Content: "Python data science pandas numpy"},
		{ID: "a", Content: "Go concurrency goroutines channels"},
	})

	multi := retriever.NewMultiRetriever(kr1, kr2)

	ctx := context.Background()
	results, err := multi.Retrieve(ctx, "concurrency goroutines", retriever.Options{
		TopK:     5,
		MinScore: 0.0,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, results)

	idSet := make(map[string]bool)
	for _, doc := range results {
		assert.False(t, idSet[doc.ID], "duplicate ID: %s", doc.ID)
		idSet[doc.ID] = true
	}
}

func TestHybridSearchWithRerankerE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	sharedDocs := []retriever.Document{
		{ID: "1", Content: "machine learning neural networks deep learning training"},
		{ID: "2", Content: "natural language processing text tokenization embeddings"},
		{ID: "3", Content: "computer vision image recognition convolutional networks"},
		{ID: "4", Content: "reinforcement learning reward policy agent environment"},
		{ID: "5", Content: "data engineering pipeline ETL transformation loading"},
	}

	kr := hybrid.NewKeywordRetriever()
	kr.Index(sharedDocs)

	semanticKR := hybrid.NewKeywordRetriever()
	semanticKR.Index(sharedDocs)
	semantic := hybrid.NewSemanticRetriever(semanticKR)

	fusion := hybrid.NewLinearStrategy(0.7, 0.3)
	hr := hybrid.NewHybridRetriever(
		semantic, kr, fusion, hybrid.HybridConfig{PreRetrieveMultiplier: 2},
	)

	ctx := context.Background()
	results, err := hr.Retrieve(ctx, "neural networks deep learning", retriever.Options{
		TopK:     3,
		MinScore: 0.0,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, results)
	assert.LessOrEqual(t, len(results), 3)

	mmr := reranker.NewMMRReranker(reranker.Config{
		Lambda: 0.7,
		TopK:   2,
	})
	reranked, err := mmr.Rerank(ctx, "neural networks", results)
	require.NoError(t, err)
	assert.NotEmpty(t, reranked)
	assert.LessOrEqual(t, len(reranked), 2)
}

func TestPipelineWithCustomStageE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	kr := hybrid.NewKeywordRetriever()
	kr.Index([]retriever.Document{
		{ID: "1", Content: "hello world programming"},
		{ID: "2", Content: "foo bar baz programming"},
	})

	filterStage := pipeline.StageFunc(func(
		ctx context.Context, input any,
	) (any, error) {
		docs, ok := input.([]retriever.Document)
		if !ok {
			return input, nil
		}
		filtered := make([]retriever.Document, 0)
		for _, doc := range docs {
			if doc.Score > 0 {
				filtered = append(filtered, doc)
			}
		}
		return filtered, nil
	})

	p, err := pipeline.NewPipeline().
		Retrieve(kr).
		AddStage(filterStage).
		Build()
	require.NoError(t, err)

	ctx := context.Background()
	result, err := p.Execute(ctx, "programming")
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotEmpty(t, result.Documents)
}

func TestKeywordRetrieverRemoveE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	kr := hybrid.NewKeywordRetriever()
	kr.Index([]retriever.Document{
		{ID: "keep", Content: "important document to keep"},
		{ID: "remove", Content: "temporary document to remove"},
	})

	ctx := context.Background()

	results, err := kr.Retrieve(ctx, "document", retriever.Options{
		TopK: 10, MinScore: 0.0,
	})
	require.NoError(t, err)
	assert.Len(t, results, 2)

	kr.Remove("remove")

	results, err = kr.Retrieve(ctx, "document", retriever.Options{
		TopK: 10, MinScore: 0.0,
	})
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "keep", results[0].ID)
}

func TestChunkerPreservesContent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e test in short mode")
	}

	original := "The quick brown fox jumps over the lazy dog. " +
		"Pack my box with five dozen liquor jugs. " +
		"How vexingly quick daft zebras jump."

	c := chunker.NewFixedSizeChunker(chunker.Config{
		ChunkSize: 50,
		Overlap:   0,
	})
	chunks := c.Chunk(original)
	require.NotEmpty(t, chunks)

	reconstructed := ""
	for _, ch := range chunks {
		reconstructed += ch.Content
	}
	assert.Equal(t, original, reconstructed)
}
