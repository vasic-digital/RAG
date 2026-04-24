package stress

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"digital.vasic.rag/pkg/chunker"
	"digital.vasic.rag/pkg/hybrid"
	"digital.vasic.rag/pkg/reranker"
	"digital.vasic.rag/pkg/retriever"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConcurrentKeywordRetrieval(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")  // SKIP-OK: #short-mode
	}

	kr := hybrid.NewKeywordRetriever()
	docs := make([]retriever.Document, 100)
	for i := 0; i < 100; i++ {
		docs[i] = retriever.Document{
			ID:      fmt.Sprintf("doc-%d", i),
			Content: fmt.Sprintf("document number %d with topic %d content", i, i%10),
		}
	}
	kr.Index(docs)

	var wg sync.WaitGroup
	const goroutines = 80
	ctx := context.Background()

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			query := fmt.Sprintf("topic %d", id%10)
			results, err := kr.Retrieve(ctx, query, retriever.Options{
				TopK:     5,
				MinScore: 0.0,
			})
			assert.NoError(t, err)
			assert.NotEmpty(t, results)
		}(i)
	}

	wg.Wait()
}

func TestConcurrentChunking(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")  // SKIP-OK: #short-mode
	}

	var wg sync.WaitGroup
	const goroutines = 60

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			text := strings.Repeat(
				fmt.Sprintf("Sentence number %d. ", id), 50,
			)
			c := chunker.NewSentenceChunker(chunker.Config{
				ChunkSize: 100,
				Overlap:   20,
			})
			chunks := c.Chunk(text)
			assert.NotEmpty(t, chunks)
		}(i)
	}

	wg.Wait()
}

func TestConcurrentMultiRetriever(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")  // SKIP-OK: #short-mode
	}

	retrievers := make([]retriever.Retriever, 5)
	for i := 0; i < 5; i++ {
		kr := hybrid.NewKeywordRetriever()
		docs := make([]retriever.Document, 20)
		for j := 0; j < 20; j++ {
			docs[j] = retriever.Document{
				ID:      fmt.Sprintf("r%d-doc%d", i, j),
				Content: fmt.Sprintf("retriever %d document %d keyword search", i, j),
			}
		}
		kr.Index(docs)
		retrievers[i] = kr
	}

	multi := retriever.NewMultiRetriever(retrievers...)

	var wg sync.WaitGroup
	const goroutines = 50
	ctx := context.Background()

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results, err := multi.Retrieve(ctx, "keyword search", retriever.Options{
				TopK:     10,
				MinScore: 0.0,
			})
			assert.NoError(t, err)
			assert.NotEmpty(t, results)
		}()
	}

	wg.Wait()
}

func TestConcurrentReranking(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")  // SKIP-OK: #short-mode
	}

	docs := make([]retriever.Document, 50)
	for i := 0; i < 50; i++ {
		docs[i] = retriever.Document{
			ID:      fmt.Sprintf("doc-%d", i),
			Content: fmt.Sprintf("content about topic %d with details %d", i%5, i),
			Score:   float64(50-i) * 0.02,
		}
	}

	var wg sync.WaitGroup
	const goroutines = 60
	ctx := context.Background()

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			query := fmt.Sprintf("topic %d", id%5)

			if id%2 == 0 {
				rr := reranker.NewScoreReranker(reranker.Config{TopK: 5})
				result, err := rr.Rerank(ctx, query, docs)
				assert.NoError(t, err)
				assert.LessOrEqual(t, len(result), 5)
			} else {
				rr := reranker.NewMMRReranker(reranker.Config{
					Lambda: 0.5, TopK: 5,
				})
				result, err := rr.Rerank(ctx, query, docs)
				assert.NoError(t, err)
				assert.LessOrEqual(t, len(result), 5)
			}
		}(i)
	}

	wg.Wait()
}

func TestConcurrentHybridRetrieval(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")  // SKIP-OK: #short-mode
	}

	sharedDocs := make([]retriever.Document, 30)
	for i := 0; i < 30; i++ {
		sharedDocs[i] = retriever.Document{
			ID:      fmt.Sprintf("shared-%d", i),
			Content: fmt.Sprintf("shared document %d about topic %d", i, i%5),
		}
	}

	kr := hybrid.NewKeywordRetriever()
	kr.Index(sharedDocs)

	semanticKR := hybrid.NewKeywordRetriever()
	semanticKR.Index(sharedDocs)
	semantic := hybrid.NewSemanticRetriever(semanticKR)

	hr := hybrid.NewHybridRetriever(
		semantic, kr,
		hybrid.NewRRFStrategy(60),
		hybrid.DefaultHybridConfig(),
	)

	var wg sync.WaitGroup
	const goroutines = 50
	ctx := context.Background()

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			results, err := hr.Retrieve(ctx, fmt.Sprintf("topic %d", id%5), retriever.Options{
				TopK:     5,
				MinScore: 0.0,
			})
			assert.NoError(t, err)
			_ = results
		}(i)
	}

	wg.Wait()
}

func TestConcurrentIndexAndRetrieve(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping stress test in short mode")  // SKIP-OK: #short-mode
	}

	kr := hybrid.NewKeywordRetriever()

	var wg sync.WaitGroup
	const writers = 50
	const readers = 50
	ctx := context.Background()

	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			kr.Index([]retriever.Document{
				{
					ID:      fmt.Sprintf("writer-%d", id),
					Content: fmt.Sprintf("written by writer %d concurrent test", id),
				},
			})
		}(i)
	}

	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := kr.Retrieve(ctx, "concurrent test", retriever.Options{
				TopK:     5,
				MinScore: 0.0,
			})
			// May or may not find results depending on timing
			_ = err
		}()
	}

	wg.Wait()

	// After all writes complete, verify retrieval works
	results, err := kr.Retrieve(ctx, "concurrent test", retriever.Options{
		TopK:     100,
		MinScore: 0.0,
	})
	require.NoError(t, err)
	assert.NotEmpty(t, results)
}
