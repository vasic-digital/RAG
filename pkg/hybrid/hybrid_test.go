package hybrid

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"digital.vasic.rag/pkg/retriever"
)

// mockRetriever for testing SemanticRetriever wrapping.
type mockRetriever struct {
	docs []retriever.Document
	err  error
}

func (m *mockRetriever) Retrieve(
	_ context.Context,
	_ string,
	_ retriever.Options,
) ([]retriever.Document, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.docs, nil
}

func TestRRFStrategy_Fuse(t *testing.T) {
	tests := []struct {
		name       string
		k          float64
		resultSets [][]retriever.Document
		wantCount  int
		wantFirst  string
	}{
		{
			name:      "empty sets",
			k:         60,
			wantCount: 0,
		},
		{
			name: "single set",
			k:    60,
			resultSets: [][]retriever.Document{
				{
					{ID: "1", Score: 0.9},
					{ID: "2", Score: 0.5},
				},
			},
			wantCount: 2,
			wantFirst: "1",
		},
		{
			name: "two sets with overlap",
			k:    60,
			resultSets: [][]retriever.Document{
				{
					{ID: "a", Score: 0.9},
					{ID: "b", Score: 0.5},
				},
				{
					{ID: "b", Score: 0.8},
					{ID: "c", Score: 0.3},
				},
			},
			wantCount: 3,
			// "b" appears in both, so should have higher RRF score
			wantFirst: "b",
		},
		{
			name: "default k when zero",
			k:    0,
			resultSets: [][]retriever.Document{
				{{ID: "1", Score: 0.5}},
			},
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rrf := NewRRFStrategy(tt.k)
			result := rrf.Fuse(tt.resultSets...)

			assert.Len(t, result, tt.wantCount)
			if tt.wantFirst != "" && len(result) > 0 {
				assert.Equal(t, tt.wantFirst, result[0].ID)
			}

			// Verify sorted descending
			for i := 1; i < len(result); i++ {
				assert.GreaterOrEqual(t, result[i-1].Score, result[i].Score)
			}
		})
	}
}

func TestLinearStrategy_Fuse(t *testing.T) {
	tests := []struct {
		name       string
		weights    []float64
		resultSets [][]retriever.Document
		wantCount  int
	}{
		{
			name:      "empty sets",
			weights:   []float64{0.7, 0.3},
			wantCount: 0,
		},
		{
			name:    "weighted combination",
			weights: []float64{0.7, 0.3},
			resultSets: [][]retriever.Document{
				{
					{ID: "a", Score: 1.0},
					{ID: "b", Score: 0.5},
				},
				{
					{ID: "b", Score: 1.0},
					{ID: "c", Score: 0.8},
				},
			},
			wantCount: 3,
		},
		{
			name:    "equal weights by default",
			weights: nil,
			resultSets: [][]retriever.Document{
				{{ID: "a", Score: 0.9}},
				{{ID: "b", Score: 0.8}},
			},
			wantCount: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ls := NewLinearStrategy(tt.weights...)
			result := ls.Fuse(tt.resultSets...)

			assert.Len(t, result, tt.wantCount)

			// Verify sorted descending
			for i := 1; i < len(result); i++ {
				assert.GreaterOrEqual(t, result[i-1].Score, result[i].Score)
			}
		})
	}
}

func TestLinearStrategy_ZeroMaxScore(t *testing.T) {
	ls := NewLinearStrategy(0.5, 0.5)
	result := ls.Fuse(
		[]retriever.Document{{ID: "a", Score: 0.0}},
		[]retriever.Document{{ID: "b", Score: 0.0}},
	)
	assert.Len(t, result, 2)
	for _, doc := range result {
		assert.Equal(t, 0.0, doc.Score)
	}
}

func TestKeywordRetriever_Retrieve(t *testing.T) {
	kr := NewKeywordRetriever()
	kr.Index([]retriever.Document{
		{ID: "1", Content: "the quick brown fox jumps over the lazy dog"},
		{ID: "2", Content: "machine learning algorithms and neural networks"},
		{ID: "3", Content: "the brown dog sleeps in the sun"},
	})

	tests := []struct {
		name      string
		query     string
		opts      retriever.Options
		wantCount int
	}{
		{
			name:      "matching query",
			query:     "brown dog",
			opts:      retriever.Options{TopK: 10},
			wantCount: 2, // docs 1 and 3 have both terms
		},
		{
			name:      "no match",
			query:     "quantum computing",
			opts:      retriever.Options{TopK: 10},
			wantCount: 0,
		},
		{
			name:      "topK limit",
			query:     "the",
			opts:      retriever.Options{TopK: 1},
			wantCount: 1,
		},
		{
			name:      "min score filter",
			query:     "brown fox dog",
			opts:      retriever.Options{TopK: 10, MinScore: 100.0},
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			docs, err := kr.Retrieve(
				context.Background(), tt.query, tt.opts,
			)
			require.NoError(t, err)
			assert.Len(t, docs, tt.wantCount)

			// Verify sorted descending
			for i := 1; i < len(docs); i++ {
				assert.GreaterOrEqual(t, docs[i-1].Score, docs[i].Score)
			}
		})
	}
}

func TestKeywordRetriever_Remove(t *testing.T) {
	kr := NewKeywordRetriever()
	kr.Index([]retriever.Document{
		{ID: "1", Content: "hello world"},
		{ID: "2", Content: "hello there"},
	})

	docs, err := kr.Retrieve(
		context.Background(), "hello", retriever.Options{TopK: 10},
	)
	require.NoError(t, err)
	assert.Len(t, docs, 2)

	kr.Remove("1")

	docs, err = kr.Retrieve(
		context.Background(), "hello", retriever.Options{TopK: 10},
	)
	require.NoError(t, err)
	assert.Len(t, docs, 1)
	assert.Equal(t, "2", docs[0].ID)
}

func TestKeywordRetriever_RemoveNonExistent(t *testing.T) {
	kr := NewKeywordRetriever()
	kr.Remove("nonexistent") // Should not panic
}

func TestSemanticRetriever_Retrieve(t *testing.T) {
	inner := &mockRetriever{docs: []retriever.Document{
		{ID: "1", Content: "test", Score: 0.9},
	}}

	sr := NewSemanticRetriever(inner)
	docs, err := sr.Retrieve(
		context.Background(), "query", retriever.DefaultOptions(),
	)

	require.NoError(t, err)
	assert.Len(t, docs, 1)
	assert.Equal(t, "1", docs[0].ID)
}

func TestSemanticRetriever_Error(t *testing.T) {
	inner := &mockRetriever{err: fmt.Errorf("embedding failed")}
	sr := NewSemanticRetriever(inner)

	_, err := sr.Retrieve(
		context.Background(), "query", retriever.DefaultOptions(),
	)
	require.Error(t, err)
}

func TestHybridRetriever_BothSucceed(t *testing.T) {
	semantic := NewSemanticRetriever(&mockRetriever{
		docs: []retriever.Document{
			{ID: "a", Content: "semantic result a", Score: 0.9},
			{ID: "b", Content: "semantic result b", Score: 0.7},
		},
	})

	keyword := NewKeywordRetriever()
	keyword.Index([]retriever.Document{
		{ID: "b", Content: "keyword result b about search"},
		{ID: "c", Content: "keyword result c about search"},
	})

	h := NewHybridRetriever(
		semantic, keyword, NewRRFStrategy(60), DefaultHybridConfig(),
	)

	docs, err := h.Retrieve(
		context.Background(), "search", retriever.Options{TopK: 10},
	)
	require.NoError(t, err)
	// Should have docs from both retrievers
	assert.GreaterOrEqual(t, len(docs), 2)
}

func TestHybridRetriever_SemanticOnly(t *testing.T) {
	semantic := NewSemanticRetriever(&mockRetriever{
		docs: []retriever.Document{
			{ID: "1", Content: "test", Score: 0.9},
		},
	})
	keyword := NewKeywordRetriever() // Empty index

	h := NewHybridRetriever(
		semantic, keyword, NewRRFStrategy(60), DefaultHybridConfig(),
	)

	docs, err := h.Retrieve(
		context.Background(), "test", retriever.Options{TopK: 10},
	)
	require.NoError(t, err)
	assert.Len(t, docs, 1)
}

func TestHybridRetriever_SemanticFails(t *testing.T) {
	semantic := NewSemanticRetriever(&mockRetriever{
		err: fmt.Errorf("semantic error"),
	})
	keyword := NewKeywordRetriever()
	keyword.Index([]retriever.Document{
		{ID: "1", Content: "keyword search test query"},
	})

	h := NewHybridRetriever(
		semantic, keyword, NewRRFStrategy(60), DefaultHybridConfig(),
	)

	docs, err := h.Retrieve(
		context.Background(), "keyword search", retriever.Options{TopK: 10},
	)
	require.NoError(t, err)
	assert.GreaterOrEqual(t, len(docs), 1)
}

func TestHybridRetriever_BothFail(t *testing.T) {
	semantic := NewSemanticRetriever(&mockRetriever{
		err: fmt.Errorf("semantic error"),
	})

	// nil keyword means it won't be called, so we get partial failure.
	// To get both to fail, we set semantic to fail and keyword to nil.
	h := &HybridRetriever{
		semantic: semantic,
		keyword:  nil,
		fusion:   NewRRFStrategy(60),
		config:   DefaultHybridConfig(),
	}

	docs, err := h.Retrieve(
		context.Background(), "query", retriever.Options{TopK: 10},
	)
	// Only semantic fails, keyword is nil (returns empty). Fuse produces
	// empty, which is not an error.
	require.NoError(t, err)
	assert.Empty(t, docs)
}

func TestHybridRetriever_TopK(t *testing.T) {
	semantic := NewSemanticRetriever(&mockRetriever{
		docs: []retriever.Document{
			{ID: "1", Score: 0.9},
			{ID: "2", Score: 0.8},
			{ID: "3", Score: 0.7},
		},
	})

	h := NewHybridRetriever(
		semantic, NewKeywordRetriever(),
		NewRRFStrategy(60), DefaultHybridConfig(),
	)

	docs, err := h.Retrieve(
		context.Background(), "test", retriever.Options{TopK: 2},
	)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(docs), 2)
}

func TestHybridRetriever_MinScoreFilter(t *testing.T) {
	semantic := NewSemanticRetriever(&mockRetriever{
		docs: []retriever.Document{
			{ID: "1", Score: 0.9},
			{ID: "2", Score: 0.1},
		},
	})

	h := NewHybridRetriever(
		semantic, NewKeywordRetriever(),
		NewRRFStrategy(60), DefaultHybridConfig(),
	)

	docs, err := h.Retrieve(
		context.Background(), "test",
		retriever.Options{TopK: 10, MinScore: 0.01},
	)
	require.NoError(t, err)
	// RRF scores will be small (1/(60+rank)), so with minScore 0.01
	// some may be filtered
	for _, doc := range docs {
		assert.GreaterOrEqual(t, doc.Score, 0.01)
	}
}

func TestHybridRetriever_DefaultConfig(t *testing.T) {
	cfg := DefaultHybridConfig()
	assert.Equal(t, 3, cfg.PreRetrieveMultiplier)
}

func TestHybridRetriever_DefaultFusion(t *testing.T) {
	h := NewHybridRetriever(nil, nil, nil, HybridConfig{})
	assert.NotNil(t, h.fusion)
}

func TestTokenize(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []string
	}{
		{
			name: "simple words",
			text: "Hello World",
			want: []string{"hello", "world"},
		},
		{
			name: "with punctuation",
			text: "Hello, World! Testing.",
			want: []string{"hello", "world", "testing"},
		},
		{
			name: "empty",
			text: "",
			want: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tokenize(tt.text)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFusionStrategyInterface(t *testing.T) {
	var _ FusionStrategy = &RRFStrategy{}
	var _ FusionStrategy = &LinearStrategy{}
}

func TestRetrieverInterface(t *testing.T) {
	var _ retriever.Retriever = &KeywordRetriever{}
	var _ retriever.Retriever = &SemanticRetriever{}
	var _ retriever.Retriever = &HybridRetriever{}
}

func TestKeywordRetriever_EmptyIndex(t *testing.T) {
	kr := NewKeywordRetriever()
	docs, err := kr.Retrieve(
		context.Background(), "test", retriever.Options{TopK: 10},
	)
	require.NoError(t, err)
	assert.Empty(t, docs)
}

func TestKeywordRetriever_DefaultTopK(t *testing.T) {
	kr := NewKeywordRetriever()
	kr.Index([]retriever.Document{
		{ID: "1", Content: "test document"},
	})

	docs, err := kr.Retrieve(
		context.Background(), "test", retriever.Options{TopK: 0},
	)
	require.NoError(t, err)
	assert.NotEmpty(t, docs)
}

func TestRRFStrategy_DocumentPreservation(t *testing.T) {
	rrf := NewRRFStrategy(60)
	result := rrf.Fuse(
		[]retriever.Document{
			{ID: "1", Content: "first version", Source: "set-a"},
		},
		[]retriever.Document{
			{ID: "1", Content: "second version", Source: "set-b"},
		},
	)

	require.Len(t, result, 1)
	assert.Equal(t, "first version", result[0].Content)
	assert.Equal(t, "set-a", result[0].Source)
}
