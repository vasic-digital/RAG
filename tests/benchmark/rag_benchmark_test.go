package benchmark

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"digital.vasic.rag/pkg/chunker"
	"digital.vasic.rag/pkg/hybrid"
	"digital.vasic.rag/pkg/reranker"
	"digital.vasic.rag/pkg/retriever"
)

func BenchmarkFixedSizeChunker(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping benchmark test in short mode")
	}

	text := strings.Repeat("The quick brown fox jumps over the lazy dog. ", 200)
	c := chunker.NewFixedSizeChunker(chunker.Config{
		ChunkSize: 100,
		Overlap:   20,
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.Chunk(text)
	}
}

func BenchmarkRecursiveChunker(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping benchmark test in short mode")
	}

	text := strings.Repeat("First sentence. Second sentence. ", 100)
	c := chunker.NewRecursiveChunker(chunker.Config{
		ChunkSize:  150,
		Overlap:    30,
		Separators: []string{"\n\n", "\n", ". ", " "},
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.Chunk(text)
	}
}

func BenchmarkSentenceChunker(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping benchmark test in short mode")
	}

	text := strings.Repeat("This is a sentence. Another one here! Question? ", 100)
	c := chunker.NewSentenceChunker(chunker.Config{
		ChunkSize: 120,
		Overlap:   20,
	})

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.Chunk(text)
	}
}

func BenchmarkKeywordRetrieval(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping benchmark test in short mode")
	}

	kr := hybrid.NewKeywordRetriever()
	docs := make([]retriever.Document, 500)
	for i := 0; i < 500; i++ {
		docs[i] = retriever.Document{
			ID:      fmt.Sprintf("doc-%d", i),
			Content: fmt.Sprintf("document %d about topic %d with keywords", i, i%20),
		}
	}
	kr.Index(docs)

	ctx := context.Background()
	opts := retriever.Options{TopK: 10, MinScore: 0.0}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = kr.Retrieve(ctx, "topic keywords", opts)
	}
}

func BenchmarkScoreReranker(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping benchmark test in short mode")
	}

	docs := make([]retriever.Document, 100)
	for i := 0; i < 100; i++ {
		docs[i] = retriever.Document{
			ID:      fmt.Sprintf("doc-%d", i),
			Content: fmt.Sprintf("content %d", i),
			Score:   float64(100-i) * 0.01,
		}
	}

	rr := reranker.NewScoreReranker(reranker.Config{TopK: 10})
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = rr.Rerank(ctx, "query", docs)
	}
}

func BenchmarkMMRReranker(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping benchmark test in short mode")
	}

	docs := make([]retriever.Document, 20)
	for i := 0; i < 20; i++ {
		docs[i] = retriever.Document{
			ID:      fmt.Sprintf("doc-%d", i),
			Content: fmt.Sprintf("unique content about topic %d with details %d", i%5, i),
			Score:   float64(20-i) * 0.05,
		}
	}

	rr := reranker.NewMMRReranker(reranker.Config{Lambda: 0.5, TopK: 5})
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = rr.Rerank(ctx, "unique content topic", docs)
	}
}

func BenchmarkRRFFusion(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping benchmark test in short mode")
	}

	set1 := make([]retriever.Document, 50)
	set2 := make([]retriever.Document, 50)
	for i := 0; i < 50; i++ {
		set1[i] = retriever.Document{
			ID: fmt.Sprintf("s1-%d", i), Content: fmt.Sprintf("set1 doc %d", i),
			Score: float64(50-i) * 0.02,
		}
		set2[i] = retriever.Document{
			ID: fmt.Sprintf("s2-%d", i), Content: fmt.Sprintf("set2 doc %d", i),
			Score: float64(50-i) * 0.02,
		}
	}

	rrf := hybrid.NewRRFStrategy(60)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = rrf.Fuse(set1, set2)
	}
}

func BenchmarkLinearFusion(b *testing.B) {
	if testing.Short() {
		b.Skip("skipping benchmark test in short mode")
	}

	set1 := make([]retriever.Document, 50)
	set2 := make([]retriever.Document, 50)
	for i := 0; i < 50; i++ {
		set1[i] = retriever.Document{
			ID: fmt.Sprintf("s1-%d", i), Content: fmt.Sprintf("set1 doc %d", i),
			Score: float64(50-i) * 0.02,
		}
		set2[i] = retriever.Document{
			ID: fmt.Sprintf("s2-%d", i), Content: fmt.Sprintf("set2 doc %d", i),
			Score: float64(50-i) * 0.02,
		}
	}

	linear := hybrid.NewLinearStrategy(0.7, 0.3)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = linear.Fuse(set1, set2)
	}
}
