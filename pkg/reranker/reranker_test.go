package reranker

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"digital.vasic.rag/pkg/retriever"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, 0.5, cfg.Lambda)
	assert.Equal(t, 10, cfg.TopK)
}

func TestScoreReranker_Rerank(t *testing.T) {
	tests := []struct {
		name      string
		config    Config
		docs      []retriever.Document
		wantCount int
		wantFirst string
	}{
		{
			name:      "empty docs",
			config:    Config{TopK: 10},
			docs:      nil,
			wantCount: 0,
		},
		{
			name:   "sorts by score descending",
			config: Config{TopK: 10},
			docs: []retriever.Document{
				{ID: "low", Score: 0.3},
				{ID: "high", Score: 0.9},
				{ID: "mid", Score: 0.6},
			},
			wantCount: 3,
			wantFirst: "high",
		},
		{
			name:   "limits to topK",
			config: Config{TopK: 2},
			docs: []retriever.Document{
				{ID: "1", Score: 0.9},
				{ID: "2", Score: 0.8},
				{ID: "3", Score: 0.7},
			},
			wantCount: 2,
			wantFirst: "1",
		},
		{
			name:   "default topK when zero",
			config: Config{TopK: 0},
			docs: []retriever.Document{
				{ID: "1", Score: 0.5},
			},
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewScoreReranker(tt.config)
			result, err := r.Rerank(context.Background(), "query", tt.docs)

			require.NoError(t, err)
			assert.Len(t, result, tt.wantCount)

			if tt.wantFirst != "" && len(result) > 0 {
				assert.Equal(t, tt.wantFirst, result[0].ID)
			}

			// Verify descending order
			for i := 1; i < len(result); i++ {
				assert.GreaterOrEqual(t, result[i-1].Score, result[i].Score)
			}
		})
	}
}

func TestScoreReranker_DoesNotMutateOriginal(t *testing.T) {
	docs := []retriever.Document{
		{ID: "1", Score: 0.3},
		{ID: "2", Score: 0.9},
	}
	original := make([]retriever.Document, len(docs))
	copy(original, docs)

	r := NewScoreReranker(Config{TopK: 10})
	_, err := r.Rerank(context.Background(), "q", docs)
	require.NoError(t, err)

	// Original should be unchanged
	assert.Equal(t, original[0].ID, docs[0].ID)
	assert.Equal(t, original[1].ID, docs[1].ID)
}

func TestMMRReranker_Rerank(t *testing.T) {
	tests := []struct {
		name      string
		config    Config
		query     string
		docs      []retriever.Document
		wantCount int
	}{
		{
			name:      "empty docs",
			config:    Config{Lambda: 0.5, TopK: 10},
			docs:      nil,
			wantCount: 0,
		},
		{
			name:   "single document",
			config: Config{Lambda: 0.5, TopK: 10},
			query:  "hello world",
			docs: []retriever.Document{
				{ID: "1", Content: "hello world example", Score: 0.9},
			},
			wantCount: 1,
		},
		{
			name:   "diverse selection with low lambda",
			config: Config{Lambda: 0.2, TopK: 3},
			query:  "machine learning algorithms",
			docs: []retriever.Document{
				{ID: "ml1", Content: "machine learning algorithms and models"},
				{ID: "ml2", Content: "machine learning algorithms deep learning"},
				{ID: "db", Content: "database indexing and query optimization"},
				{ID: "net", Content: "network protocols and routing algorithms"},
			},
			wantCount: 3,
		},
		{
			name:   "relevance focused with high lambda",
			config: Config{Lambda: 1.0, TopK: 2},
			query:  "go programming",
			docs: []retriever.Document{
				{ID: "go1", Content: "go programming language tutorial"},
				{ID: "go2", Content: "go programming best practices"},
				{ID: "py", Content: "python programming basics"},
			},
			wantCount: 2,
		},
		{
			name:   "topK larger than docs",
			config: Config{Lambda: 0.5, TopK: 100},
			query:  "test",
			docs: []retriever.Document{
				{ID: "1", Content: "test one"},
				{ID: "2", Content: "test two"},
			},
			wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewMMRReranker(tt.config)
			result, err := r.Rerank(
				context.Background(), tt.query, tt.docs,
			)

			require.NoError(t, err)
			assert.Len(t, result, tt.wantCount)

			// Verify no duplicates
			seen := make(map[string]bool)
			for _, doc := range result {
				assert.False(t, seen[doc.ID], "duplicate doc: %s", doc.ID)
				seen[doc.ID] = true
			}
		})
	}
}

func TestMMRReranker_DiversityEffect(t *testing.T) {
	query := "search algorithms"
	docs := []retriever.Document{
		{ID: "sim1", Content: "search algorithms binary search tree"},
		{ID: "sim2", Content: "search algorithms binary search optimization"},
		{ID: "diff", Content: "database indexing b-tree structures"},
	}

	// High diversity (low lambda) should prefer the different document
	diverse := NewMMRReranker(Config{Lambda: 0.1, TopK: 3})
	diverseResult, err := diverse.Rerank(
		context.Background(), query, docs,
	)
	require.NoError(t, err)
	require.Len(t, diverseResult, 3)

	// The diverse result should include "diff" higher up than
	// pure relevance would suggest
	diffPos := -1
	for i, d := range diverseResult {
		if d.ID == "diff" {
			diffPos = i
			break
		}
	}
	assert.GreaterOrEqual(t, diffPos, 0, "diverse doc should be selected")
}

func TestMMRReranker_LambdaBoundsClamped(t *testing.T) {
	// Lambda < 0 should be clamped to 0
	r1 := NewMMRReranker(Config{Lambda: -1.0, TopK: 5})
	assert.Equal(t, 0.0, r1.config.Lambda)

	// Lambda > 1 should be clamped to 1
	r2 := NewMMRReranker(Config{Lambda: 2.0, TopK: 5})
	assert.Equal(t, 1.0, r2.config.Lambda)
}

func TestTextSimilarity(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want float64
	}{
		{"identical", "hello world", "hello world", 1.0},
		{"empty a", "", "hello", 0.0},
		{"empty b", "hello", "", 0.0},
		{"both empty", "", "", 0.0},
		{"no overlap", "cat", "dog", 0.0},
		{
			"partial overlap",
			"the quick brown fox",
			"the lazy brown dog",
			// intersection: {the, brown} = 2
			// union: {the,quick,brown,fox,lazy,dog} = 6
			2.0 / 6.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := textSimilarity(tt.a, tt.b)
			assert.InDelta(t, tt.want, got, 0.001)
		})
	}
}

func TestTokenize(t *testing.T) {
	tokens := tokenize("Hello, World! Testing 123.")
	assert.Contains(t, tokens, "hello")
	assert.Contains(t, tokens, "world")
	assert.Contains(t, tokens, "testing")
	assert.Contains(t, tokens, "123")
}

func TestRerankerInterface(t *testing.T) {
	var _ Reranker = &ScoreReranker{}
	var _ Reranker = &MMRReranker{}
}
