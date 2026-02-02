// Package reranker provides result reranking strategies for improving
// retrieval quality in RAG systems.
package reranker

import (
	"context"
	"math"
	"sort"
	"strings"

	"digital.vasic.rag/pkg/retriever"
)

// Reranker defines the interface for reranking retrieved documents.
type Reranker interface {
	// Rerank reorders documents based on their relevance to the query.
	Rerank(
		ctx context.Context,
		query string,
		docs []retriever.Document,
	) ([]retriever.Document, error)
}

// Config holds configuration for rerankers.
type Config struct {
	// Lambda is the diversity factor for MMR (0=max diversity, 1=max relevance).
	Lambda float64 `json:"lambda"`
	// TopK limits the number of results returned.
	TopK int `json:"top_k"`
}

// DefaultConfig returns a default reranker configuration.
func DefaultConfig() Config {
	return Config{
		Lambda: 0.5,
		TopK:   10,
	}
}

// ScoreReranker reranks documents by their existing score (passthrough).
type ScoreReranker struct {
	config Config
}

// NewScoreReranker creates a ScoreReranker with the given configuration.
func NewScoreReranker(config Config) *ScoreReranker {
	if config.TopK <= 0 {
		config.TopK = 10
	}
	return &ScoreReranker{config: config}
}

// Rerank sorts documents by score descending and limits to TopK.
func (r *ScoreReranker) Rerank(
	_ context.Context,
	_ string,
	docs []retriever.Document,
) ([]retriever.Document, error) {
	if len(docs) == 0 {
		return docs, nil
	}

	result := make([]retriever.Document, len(docs))
	copy(result, docs)

	sort.Slice(result, func(i, j int) bool {
		return result[i].Score > result[j].Score
	})

	if r.config.TopK > 0 && len(result) > r.config.TopK {
		result = result[:r.config.TopK]
	}

	return result, nil
}

// MMRReranker implements Maximal Marginal Relevance for balancing
// relevance and diversity in search results.
type MMRReranker struct {
	config Config
}

// NewMMRReranker creates an MMRReranker with the given configuration.
func NewMMRReranker(config Config) *MMRReranker {
	if config.TopK <= 0 {
		config.TopK = 10
	}
	if config.Lambda < 0 {
		config.Lambda = 0
	}
	if config.Lambda > 1 {
		config.Lambda = 1
	}
	return &MMRReranker{config: config}
}

// Rerank selects documents using the MMR algorithm:
// MMR(d) = Lambda * Sim(d, query) - (1 - Lambda) * max(Sim(d, d_selected))
//
// This balances relevance (similarity to query) against diversity
// (dissimilarity from already-selected documents).
func (r *MMRReranker) Rerank(
	_ context.Context,
	query string,
	docs []retriever.Document,
) ([]retriever.Document, error) {
	if len(docs) == 0 {
		return docs, nil
	}

	n := len(docs)
	topK := r.config.TopK
	if topK > n {
		topK = n
	}

	// Compute query-document similarities
	querySims := make([]float64, n)
	for i, doc := range docs {
		querySims[i] = textSimilarity(query, doc.Content)
	}

	// Compute pairwise document similarities
	docSims := make([][]float64, n)
	for i := range docSims {
		docSims[i] = make([]float64, n)
		for j := range docSims[i] {
			if i == j {
				docSims[i][j] = 1.0
			} else if j < i {
				docSims[i][j] = docSims[j][i]
			} else {
				docSims[i][j] = textSimilarity(
					docs[i].Content, docs[j].Content,
				)
			}
		}
	}

	// Greedily select documents using MMR
	selected := make([]int, 0, topK)
	unselected := make(map[int]bool, n)
	for i := 0; i < n; i++ {
		unselected[i] = true
	}

	for len(selected) < topK && len(unselected) > 0 {
		bestIdx := -1
		bestScore := -math.MaxFloat64

		for idx := range unselected {
			// Relevance component
			relevance := querySims[idx]

			// Diversity component: max similarity to any selected doc
			maxSimToSelected := 0.0
			for _, selIdx := range selected {
				sim := docSims[idx][selIdx]
				if sim > maxSimToSelected {
					maxSimToSelected = sim
				}
			}

			mmrScore := r.config.Lambda*relevance -
				(1-r.config.Lambda)*maxSimToSelected

			if mmrScore > bestScore {
				bestScore = mmrScore
				bestIdx = idx
			}
		}

		if bestIdx >= 0 {
			selected = append(selected, bestIdx)
			delete(unselected, bestIdx)
		}
	}

	// Build result with updated scores
	result := make([]retriever.Document, len(selected))
	for i, idx := range selected {
		doc := docs[idx]
		doc.Score = querySims[idx]
		result[i] = doc
	}

	return result, nil
}

// textSimilarity computes a simple word-overlap similarity between
// two texts, normalized to [0, 1].
func textSimilarity(a, b string) float64 {
	aWords := tokenize(a)
	bWords := tokenize(b)

	if len(aWords) == 0 || len(bWords) == 0 {
		return 0.0
	}

	aSet := make(map[string]bool, len(aWords))
	for _, w := range aWords {
		aSet[w] = true
	}

	bSet := make(map[string]bool, len(bWords))
	for _, w := range bWords {
		bSet[w] = true
	}

	// Jaccard similarity
	intersection := 0
	for w := range aSet {
		if bSet[w] {
			intersection++
		}
	}

	union := len(aSet) + len(bSet) - intersection
	if union == 0 {
		return 0.0
	}

	return float64(intersection) / float64(union)
}

// tokenize splits text into lowercase word tokens.
func tokenize(text string) []string {
	words := strings.FieldsFunc(
		strings.ToLower(text),
		func(r rune) bool {
			return !((r >= 'a' && r <= 'z') ||
				(r >= '0' && r <= '9') ||
				r == '_' || r == '-')
		},
	)
	return words
}
