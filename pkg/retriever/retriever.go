// Package retriever provides core retrieval interfaces and types for
// Retrieval-Augmented Generation (RAG) systems.
package retriever

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// Document represents a retrieved document in the RAG system.
type Document struct {
	ID       string         `json:"id"`
	Content  string         `json:"content"`
	Metadata map[string]any `json:"metadata,omitempty"`
	Score    float64        `json:"score,omitempty"`
	Source   string         `json:"source,omitempty"`
}

// Options configures retrieval behavior.
type Options struct {
	TopK     int            `json:"top_k"`
	MinScore float64        `json:"min_score"`
	Filter   map[string]any `json:"filter,omitempty"`
}

// DefaultOptions returns default retrieval options.
func DefaultOptions() Options {
	return Options{
		TopK:     10,
		MinScore: 0.0,
	}
}

// Retriever defines the interface for document retrieval.
type Retriever interface {
	// Retrieve searches for relevant documents matching the query.
	Retrieve(ctx context.Context, query string, opts Options) ([]Document, error)
}

// MultiRetriever combines multiple retrievers and merges their results
// using score-based deduplication and ranking.
type MultiRetriever struct {
	retrievers []Retriever
	mu         sync.RWMutex
}

// NewMultiRetriever creates a MultiRetriever from the given retrievers.
func NewMultiRetriever(retrievers ...Retriever) *MultiRetriever {
	return &MultiRetriever{
		retrievers: retrievers,
	}
}

// AddRetriever adds a retriever to the multi-retriever.
func (m *MultiRetriever) AddRetriever(r Retriever) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.retrievers = append(m.retrievers, r)
}

// Retrieve queries all underlying retrievers in parallel, deduplicates
// results by document ID (keeping the highest score), and returns
// the merged results sorted by score descending.
func (m *MultiRetriever) Retrieve(
	ctx context.Context,
	query string,
	opts Options,
) ([]Document, error) {
	m.mu.RLock()
	retrievers := make([]Retriever, len(m.retrievers))
	copy(retrievers, m.retrievers)
	m.mu.RUnlock()

	if len(retrievers) == 0 {
		return nil, fmt.Errorf("no retrievers configured")
	}

	type result struct {
		docs []Document
		err  error
	}

	results := make([]result, len(retrievers))
	var wg sync.WaitGroup

	for i, r := range retrievers {
		wg.Add(1)
		go func(idx int, ret Retriever) {
			defer wg.Done()
			docs, err := ret.Retrieve(ctx, query, opts)
			results[idx] = result{docs: docs, err: err}
		}(i, r)
	}

	wg.Wait()

	// Collect errors and merge results
	var errs []error
	docMap := make(map[string]Document)

	for _, res := range results {
		if res.err != nil {
			errs = append(errs, res.err)
			continue
		}
		for _, doc := range res.docs {
			if existing, ok := docMap[doc.ID]; ok {
				if doc.Score > existing.Score {
					docMap[doc.ID] = doc
				}
			} else {
				docMap[doc.ID] = doc
			}
		}
	}

	// If all retrievers failed, return combined error
	if len(errs) == len(retrievers) {
		return nil, fmt.Errorf("all retrievers failed: %v", errs)
	}

	// Convert to sorted slice
	docs := make([]Document, 0, len(docMap))
	for _, doc := range docMap {
		if doc.Score >= opts.MinScore {
			docs = append(docs, doc)
		}
	}

	sort.Slice(docs, func(i, j int) bool {
		return docs[i].Score > docs[j].Score
	})

	if opts.TopK > 0 && len(docs) > opts.TopK {
		docs = docs[:opts.TopK]
	}

	return docs, nil
}
