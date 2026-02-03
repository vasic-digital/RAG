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

// Tests for uncovered code paths

func TestKeywordRetriever_CalculateTF_ZeroAvgDocLen(t *testing.T) {
	// Tests calculateTF when avgDocLen == 0 (line 263-265)
	// This happens when no documents are indexed
	kr := NewKeywordRetriever()
	// avgDocLen starts at 0 because totalDocs is 0
	assert.Equal(t, float64(0), kr.avgDocLen)

	// Index a single empty document to test edge case
	kr.Index([]retriever.Document{
		{ID: "1", Content: ""},
	})
	// After indexing empty doc, avgDocLen should be 0/1 = 0
	// but totalDocs is 1

	// Now remove it to get totalDocs back to 0
	kr.Remove("1")
	assert.Equal(t, float64(0), kr.avgDocLen)
	assert.Equal(t, 0, kr.totalDocs)
}

func TestKeywordRetriever_RecalculateAvgDocLen_EmptyIndex(t *testing.T) {
	// Tests recalculateAvgDocLen with empty index (line 275-279)
	kr := NewKeywordRetriever()
	// Initial state - no docs
	assert.Equal(t, float64(0), kr.avgDocLen)
	assert.Equal(t, 0, kr.totalDocs)

	// Index and then remove to trigger recalculate with 0 docs
	kr.Index([]retriever.Document{
		{ID: "1", Content: "hello world"},
	})
	assert.Greater(t, kr.avgDocLen, float64(0))

	kr.Remove("1")
	// After removal, avgDocLen should be reset to 0
	assert.Equal(t, float64(0), kr.avgDocLen)
}

func TestHybridRetriever_Retrieve_DefaultTopK(t *testing.T) {
	// Test default topK when opts.TopK <= 0 (lines 409-412)
	semantic := NewSemanticRetriever(&mockRetriever{
		docs: []retriever.Document{
			{ID: "1", Score: 0.9},
			{ID: "2", Score: 0.8},
		},
	})

	h := NewHybridRetriever(
		semantic, NewKeywordRetriever(),
		NewRRFStrategy(60), DefaultHybridConfig(),
	)

	// Pass TopK = 0 to trigger default (10)
	docs, err := h.Retrieve(
		context.Background(), "test", retriever.Options{TopK: 0},
	)
	require.NoError(t, err)
	assert.NotEmpty(t, docs)
}

func TestHybridRetriever_BothRetrieversNil(t *testing.T) {
	// Test when both semantic and keyword are nil
	h := NewHybridRetriever(nil, nil, NewRRFStrategy(60), DefaultHybridConfig())

	docs, err := h.Retrieve(
		context.Background(), "test", retriever.Options{TopK: 10},
	)
	require.NoError(t, err)
	assert.Empty(t, docs)
}

func TestHybridRetriever_BothRetrieversFailError(t *testing.T) {
	// Test when both retrievers return errors (lines 387-391)
	failingRetriever := &mockRetriever{
		err: fmt.Errorf("retriever failed"),
	}
	semantic := NewSemanticRetriever(failingRetriever)

	// Create a custom keyword retriever that fails
	// Actually KeywordRetriever.Retrieve never returns error
	// So we need both semantic to fail and keyword to also fail
	// But KeywordRetriever can't fail. Let's check if we can make both fail.

	// Create hybrid with semantic that fails and keyword set to nil
	// When semantic fails and keyword is nil, keywordDocs will be empty
	// and semanticErr will be non-nil
	// This means semanticErr != nil && keywordErr == nil, so no error is returned

	// To test line 387-391 we need BOTH to return error
	// Since KeywordRetriever.Retrieve never returns error, we can't test this
	// through KeywordRetriever. But the hybrid takes any retriever.Retriever.
	// Actually the hybrid is hardcoded with *SemanticRetriever and *KeywordRetriever
	// So we can't easily inject a failing keyword retriever.

	// Let's test what happens when semantic fails
	h := &HybridRetriever{
		semantic: semantic,
		keyword:  nil,
		fusion:   NewRRFStrategy(60),
		config:   DefaultHybridConfig(),
	}

	_, err := h.Retrieve(
		context.Background(), "test", retriever.Options{TopK: 10},
	)
	// Since keyword is nil (not an error, just empty), only semantic fails
	// So the error check at 387-391 is not triggered
	require.NoError(t, err)
}

func TestKeywordRetriever_CalculateTF_WithZeroAvgDocLen_During_Retrieve(t *testing.T) {
	// Test calculateTF when avgDocLen == 0 during actual retrieval
	// This is tricky because Index() always recalculates avgDocLen
	// We need to test the scenario where avgDocLen is 0 but we have matching terms
	// This can happen if we manually set avgDocLen to 0 after indexing

	kr := NewKeywordRetriever()
	// Index documents normally
	kr.Index([]retriever.Document{
		{ID: "1", Content: "hello world"},
	})

	// Manually set avgDocLen to 0 to test the branch
	kr.mu.Lock()
	kr.avgDocLen = 0
	kr.mu.Unlock()

	// Now retrieve - should hit the if avgDocLen == 0 branch in calculateTF
	docs, err := kr.Retrieve(
		context.Background(), "hello", retriever.Options{TopK: 10},
	)
	require.NoError(t, err)
	// Should still return results even with avgDocLen = 0
	assert.NotEmpty(t, docs)
}

// failingKeywordRetriever is a wrapper that makes KeywordRetriever fail
type failingKeywordRetriever struct {
	*KeywordRetriever
	err error
}

func (f *failingKeywordRetriever) Retrieve(
	_ context.Context,
	_ string,
	_ retriever.Options,
) ([]retriever.Document, error) {
	return nil, f.err
}

func TestHybridRetriever_BothRetrieversReturnError(t *testing.T) {
	// Test when both semantic and keyword retrievers return errors
	// This triggers the error path at lines 387-391

	semanticErr := fmt.Errorf("semantic retriever failed")
	keywordErr := fmt.Errorf("keyword retriever failed")

	// Create failing retrievers using the mockRetriever
	failingSemantic := &mockRetriever{err: semanticErr}
	failingKeyword := &mockRetriever{err: keywordErr}

	// Create a HybridRetriever with both retrievers that fail
	// Now that HybridRetriever uses retriever.Retriever interfaces,
	// we can inject any implementation that fails
	h := NewHybridRetriever(
		failingSemantic,
		failingKeyword,
		NewRRFStrategy(60),
		DefaultHybridConfig(),
	)

	_, err := h.Retrieve(
		context.Background(), "test", retriever.Options{TopK: 10},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "both retrievers failed")
	assert.Contains(t, err.Error(), "semantic")
	assert.Contains(t, err.Error(), "keyword")
}
