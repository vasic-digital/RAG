// Package hybrid provides hybrid retrieval combining semantic (vector)
// and keyword (BM25) search with configurable fusion strategies.
package hybrid

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"

	"digital.vasic.rag/pkg/retriever"
)

// FusionStrategy defines how to combine results from multiple retrievers.
type FusionStrategy interface {
	// Fuse combines results from multiple retriever result sets.
	Fuse(resultSets ...[]retriever.Document) []retriever.Document
}

// RRFStrategy implements Reciprocal Rank Fusion.
// RRF(d) = sum(1 / (k + rank(d))) across all result sets.
type RRFStrategy struct {
	// K is the RRF constant (typically 60).
	K float64
}

// NewRRFStrategy creates an RRF fusion strategy.
func NewRRFStrategy(k float64) *RRFStrategy {
	if k <= 0 {
		k = 60
	}
	return &RRFStrategy{K: k}
}

// Fuse merges result sets using Reciprocal Rank Fusion.
func (r *RRFStrategy) Fuse(
	resultSets ...[]retriever.Document,
) []retriever.Document {
	scoreMap := make(map[string]float64)
	docMap := make(map[string]retriever.Document)

	for _, results := range resultSets {
		for rank, doc := range results {
			scoreMap[doc.ID] += 1.0 / (r.K + float64(rank+1))
			if _, exists := docMap[doc.ID]; !exists {
				docMap[doc.ID] = doc
			}
		}
	}

	docs := make([]retriever.Document, 0, len(scoreMap))
	for id, score := range scoreMap {
		doc := docMap[id]
		doc.Score = score
		docs = append(docs, doc)
	}

	sort.Slice(docs, func(i, j int) bool {
		return docs[i].Score > docs[j].Score
	})

	return docs
}

// LinearStrategy combines results using weighted linear combination
// of normalized scores.
type LinearStrategy struct {
	// Weights for each result set. If fewer weights than result sets,
	// remaining sets get equal weight.
	Weights []float64
}

// NewLinearStrategy creates a linear fusion strategy with given weights.
func NewLinearStrategy(weights ...float64) *LinearStrategy {
	return &LinearStrategy{Weights: weights}
}

// Fuse merges result sets using weighted linear combination.
func (l *LinearStrategy) Fuse(
	resultSets ...[]retriever.Document,
) []retriever.Document {
	scoreMap := make(map[string]float64)
	docMap := make(map[string]retriever.Document)

	for setIdx, results := range resultSets {
		weight := 1.0 / float64(len(resultSets))
		if setIdx < len(l.Weights) {
			weight = l.Weights[setIdx]
		}

		// Normalize scores within this set
		maxScore := 0.0
		for _, doc := range results {
			if doc.Score > maxScore {
				maxScore = doc.Score
			}
		}

		for _, doc := range results {
			normalized := 0.0
			if maxScore > 0 {
				normalized = doc.Score / maxScore
			}
			scoreMap[doc.ID] += weight * normalized
			if _, exists := docMap[doc.ID]; !exists {
				docMap[doc.ID] = doc
			}
		}
	}

	docs := make([]retriever.Document, 0, len(scoreMap))
	for id, score := range scoreMap {
		doc := docMap[id]
		doc.Score = score
		docs = append(docs, doc)
	}

	sort.Slice(docs, func(i, j int) bool {
		return docs[i].Score > docs[j].Score
	})

	return docs
}

// KeywordRetriever implements BM25-style keyword retrieval.
type KeywordRetriever struct {
	documents  map[string]retriever.Document
	termFreqs  map[string]map[string]int // docID -> term -> freq
	docFreqs   map[string]int            // term -> doc count
	docLengths map[string]int
	avgDocLen  float64
	totalDocs  int
	k1         float64
	b          float64
	mu         sync.RWMutex
}

// NewKeywordRetriever creates a new BM25-based keyword retriever.
func NewKeywordRetriever() *KeywordRetriever {
	return &KeywordRetriever{
		documents:  make(map[string]retriever.Document),
		termFreqs:  make(map[string]map[string]int),
		docFreqs:   make(map[string]int),
		docLengths: make(map[string]int),
		k1:         1.2,
		b:          0.75,
	}
}

// Index adds documents to the BM25 index.
func (r *KeywordRetriever) Index(docs []retriever.Document) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for _, doc := range docs {
		terms := tokenize(doc.Content)
		r.documents[doc.ID] = doc
		r.termFreqs[doc.ID] = make(map[string]int)
		r.docLengths[doc.ID] = len(terms)

		seen := make(map[string]bool)
		for _, term := range terms {
			r.termFreqs[doc.ID][term]++
			if !seen[term] {
				r.docFreqs[term]++
				seen[term] = true
			}
		}
		r.totalDocs++
	}

	r.recalculateAvgDocLen()
}

// Remove removes a document from the index.
func (r *KeywordRetriever) Remove(id string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.documents[id]; !exists {
		return
	}

	for term := range r.termFreqs[id] {
		r.docFreqs[term]--
		if r.docFreqs[term] <= 0 {
			delete(r.docFreqs, term)
		}
	}

	delete(r.documents, id)
	delete(r.termFreqs, id)
	delete(r.docLengths, id)
	r.totalDocs--
	r.recalculateAvgDocLen()
}

// Retrieve performs BM25 keyword search.
func (r *KeywordRetriever) Retrieve(
	_ context.Context,
	query string,
	opts retriever.Options,
) ([]retriever.Document, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	queryTerms := tokenize(query)
	scores := make(map[string]float64)

	for _, term := range queryTerms {
		df, exists := r.docFreqs[term]
		if !exists {
			continue
		}

		idf := r.calculateIDF(df)

		for docID, tf := range r.termFreqs {
			termFreq, ok := tf[term]
			if !ok {
				continue
			}

			docLen := float64(r.docLengths[docID])
			tfScore := r.calculateTF(float64(termFreq), docLen)
			scores[docID] += idf * tfScore
		}
	}

	docs := make([]retriever.Document, 0, len(scores))
	for docID, score := range scores {
		if score < opts.MinScore {
			continue
		}
		doc := r.documents[docID]
		doc.Score = score
		docs = append(docs, doc)
	}

	sort.Slice(docs, func(i, j int) bool {
		return docs[i].Score > docs[j].Score
	})

	topK := opts.TopK
	if topK <= 0 {
		topK = 10
	}
	if len(docs) > topK {
		docs = docs[:topK]
	}

	return docs, nil
}

func (r *KeywordRetriever) calculateIDF(df int) float64 {
	n := float64(r.totalDocs)
	return math.Log((n-float64(df)+0.5)/(float64(df)+0.5) + 1)
}

func (r *KeywordRetriever) calculateTF(tf, docLen float64) float64 {
	if r.avgDocLen == 0 {
		return tf
	}
	return (tf * (r.k1 + 1)) /
		(tf + r.k1*(1-r.b+r.b*(docLen/r.avgDocLen)))
}

func (r *KeywordRetriever) recalculateAvgDocLen() {
	total := 0
	for _, length := range r.docLengths {
		total += length
	}
	if r.totalDocs > 0 {
		r.avgDocLen = float64(total) / float64(r.totalDocs)
	} else {
		r.avgDocLen = 0
	}
}

// SemanticRetriever wraps any retriever.Retriever to provide a semantic
// search interface. In a real system, this would use vector embeddings;
// here it delegates to the underlying retriever.
type SemanticRetriever struct {
	inner retriever.Retriever
}

// NewSemanticRetriever wraps an existing Retriever as a semantic retriever.
func NewSemanticRetriever(inner retriever.Retriever) *SemanticRetriever {
	return &SemanticRetriever{inner: inner}
}

// Retrieve delegates to the inner retriever.
func (s *SemanticRetriever) Retrieve(
	ctx context.Context,
	query string,
	opts retriever.Options,
) ([]retriever.Document, error) {
	return s.inner.Retrieve(ctx, query, opts)
}

// HybridRetriever combines semantic and keyword retrievers using
// a configurable fusion strategy.
type HybridRetriever struct {
	semantic retriever.Retriever
	keyword  retriever.Retriever
	fusion   FusionStrategy
	config   HybridConfig
}

// HybridConfig configures the hybrid retriever.
type HybridConfig struct {
	// PreRetrieveMultiplier retrieves N*topK before fusion.
	PreRetrieveMultiplier int `json:"pre_retrieve_multiplier"`
}

// DefaultHybridConfig returns a default hybrid retrieval configuration.
func DefaultHybridConfig() HybridConfig {
	return HybridConfig{
		PreRetrieveMultiplier: 3,
	}
}

// NewHybridRetriever creates a hybrid retriever combining semantic and
// keyword search. Both semantic and keyword can be any retriever.Retriever
// implementation (e.g., *SemanticRetriever, *KeywordRetriever).
func NewHybridRetriever(
	semantic retriever.Retriever,
	keyword retriever.Retriever,
	fusion FusionStrategy,
	config HybridConfig,
) *HybridRetriever {
	if fusion == nil {
		fusion = NewRRFStrategy(60)
	}
	if config.PreRetrieveMultiplier <= 0 {
		config.PreRetrieveMultiplier = 3
	}
	return &HybridRetriever{
		semantic: semantic,
		keyword:  keyword,
		fusion:   fusion,
		config:   config,
	}
}

// Retrieve performs hybrid search by querying both semantic and keyword
// retrievers in parallel, then fusing results.
func (h *HybridRetriever) Retrieve(
	ctx context.Context,
	query string,
	opts retriever.Options,
) ([]retriever.Document, error) {
	preOpts := opts
	preOpts.TopK = opts.TopK * h.config.PreRetrieveMultiplier

	var (
		semanticDocs []retriever.Document
		keywordDocs  []retriever.Document
		semanticErr  error
		keywordErr   error
		wg           sync.WaitGroup
	)

	wg.Add(2)

	go func() {
		defer wg.Done()
		if h.semantic != nil {
			semanticDocs, semanticErr = h.semantic.Retrieve(
				ctx, query, preOpts,
			)
		}
	}()

	go func() {
		defer wg.Done()
		if h.keyword != nil {
			keywordDocs, keywordErr = h.keyword.Retrieve(
				ctx, query, preOpts,
			)
		}
	}()

	wg.Wait()

	if semanticErr != nil && keywordErr != nil {
		return nil, fmt.Errorf(
			"both retrievers failed: semantic=%v, keyword=%v",
			semanticErr, keywordErr,
		)
	}

	// Fuse results
	fused := h.fusion.Fuse(semanticDocs, keywordDocs)

	// Apply min score filter
	if opts.MinScore > 0 {
		filtered := make([]retriever.Document, 0, len(fused))
		for _, doc := range fused {
			if doc.Score >= opts.MinScore {
				filtered = append(filtered, doc)
			}
		}
		fused = filtered
	}

	// Limit to topK
	topK := opts.TopK
	if topK <= 0 {
		topK = 10
	}
	if len(fused) > topK {
		fused = fused[:topK]
	}

	return fused, nil
}

// tokenize splits text into lowercase word tokens for BM25.
func tokenize(text string) []string {
	text = strings.ToLower(text)
	words := strings.Fields(text)

	tokens := make([]string, 0, len(words))
	for _, word := range words {
		cleaned := strings.Trim(
			word, ".,!?;:\"'()[]{}#$%&*+-/<>=@\\^_`|~",
		)
		if len(cleaned) > 0 {
			tokens = append(tokens, cleaned)
		}
	}
	return tokens
}
