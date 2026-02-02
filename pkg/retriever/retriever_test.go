package retriever

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockRetriever is a test implementation of Retriever.
type mockRetriever struct {
	docs []Document
	err  error
}

func (m *mockRetriever) Retrieve(
	_ context.Context,
	_ string,
	_ Options,
) ([]Document, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.docs, nil
}

func TestDefaultOptions(t *testing.T) {
	opts := DefaultOptions()
	assert.Equal(t, 10, opts.TopK)
	assert.Equal(t, 0.0, opts.MinScore)
	assert.Nil(t, opts.Filter)
}

func TestDocument_Fields(t *testing.T) {
	doc := Document{
		ID:       "doc-1",
		Content:  "hello world",
		Metadata: map[string]any{"key": "value"},
		Score:    0.95,
		Source:   "test",
	}
	assert.Equal(t, "doc-1", doc.ID)
	assert.Equal(t, "hello world", doc.Content)
	assert.Equal(t, 0.95, doc.Score)
	assert.Equal(t, "test", doc.Source)
	assert.Equal(t, "value", doc.Metadata["key"])
}

func TestMultiRetriever_Retrieve(t *testing.T) {
	tests := []struct {
		name       string
		retrievers []Retriever
		opts       Options
		wantCount  int
		wantErr    bool
		wantIDs    []string
	}{
		{
			name:       "no retrievers",
			retrievers: nil,
			opts:       DefaultOptions(),
			wantErr:    true,
		},
		{
			name: "single retriever",
			retrievers: []Retriever{
				&mockRetriever{docs: []Document{
					{ID: "1", Content: "doc one", Score: 0.9},
					{ID: "2", Content: "doc two", Score: 0.8},
				}},
			},
			opts:      DefaultOptions(),
			wantCount: 2,
			wantIDs:   []string{"1", "2"},
		},
		{
			name: "multiple retrievers with dedup keeping higher score",
			retrievers: []Retriever{
				&mockRetriever{docs: []Document{
					{ID: "1", Content: "doc one", Score: 0.9},
					{ID: "2", Content: "doc two", Score: 0.5},
				}},
				&mockRetriever{docs: []Document{
					{ID: "2", Content: "doc two updated", Score: 0.8},
					{ID: "3", Content: "doc three", Score: 0.7},
				}},
			},
			opts:      DefaultOptions(),
			wantCount: 3,
			wantIDs:   []string{"1", "2", "3"},
		},
		{
			name: "topK limit",
			retrievers: []Retriever{
				&mockRetriever{docs: []Document{
					{ID: "1", Score: 0.9},
					{ID: "2", Score: 0.8},
					{ID: "3", Score: 0.7},
				}},
			},
			opts:      Options{TopK: 2},
			wantCount: 2,
		},
		{
			name: "min score filter",
			retrievers: []Retriever{
				&mockRetriever{docs: []Document{
					{ID: "1", Score: 0.9},
					{ID: "2", Score: 0.3},
					{ID: "3", Score: 0.1},
				}},
			},
			opts:      Options{TopK: 10, MinScore: 0.5},
			wantCount: 1,
		},
		{
			name: "partial failure still returns results",
			retrievers: []Retriever{
				&mockRetriever{docs: []Document{
					{ID: "1", Score: 0.9},
				}},
				&mockRetriever{err: fmt.Errorf("connection error")},
			},
			opts:      DefaultOptions(),
			wantCount: 1,
		},
		{
			name: "all retrievers fail",
			retrievers: []Retriever{
				&mockRetriever{err: fmt.Errorf("error 1")},
				&mockRetriever{err: fmt.Errorf("error 2")},
			},
			opts:    DefaultOptions(),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mr := NewMultiRetriever(tt.retrievers...)
			docs, err := mr.Retrieve(context.Background(), "test query", tt.opts)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Len(t, docs, tt.wantCount)

			if tt.wantIDs != nil {
				gotIDs := make(map[string]bool)
				for _, d := range docs {
					gotIDs[d.ID] = true
				}
				for _, id := range tt.wantIDs {
					assert.True(t, gotIDs[id], "expected doc ID %s", id)
				}
			}

			// Verify sorted by score descending
			for i := 1; i < len(docs); i++ {
				assert.GreaterOrEqual(t, docs[i-1].Score, docs[i].Score)
			}
		})
	}
}

func TestMultiRetriever_DedupKeepsHigherScore(t *testing.T) {
	r1 := &mockRetriever{docs: []Document{
		{ID: "shared", Content: "low score", Score: 0.3},
	}}
	r2 := &mockRetriever{docs: []Document{
		{ID: "shared", Content: "high score", Score: 0.9},
	}}

	mr := NewMultiRetriever(r1, r2)
	docs, err := mr.Retrieve(context.Background(), "q", DefaultOptions())

	require.NoError(t, err)
	require.Len(t, docs, 1)
	assert.Equal(t, 0.9, docs[0].Score)
	assert.Equal(t, "high score", docs[0].Content)
}

func TestMultiRetriever_AddRetriever(t *testing.T) {
	mr := NewMultiRetriever()

	_, err := mr.Retrieve(context.Background(), "q", DefaultOptions())
	require.Error(t, err)

	mr.AddRetriever(&mockRetriever{docs: []Document{
		{ID: "1", Score: 0.5},
	}})

	docs, err := mr.Retrieve(context.Background(), "q", DefaultOptions())
	require.NoError(t, err)
	assert.Len(t, docs, 1)
}
