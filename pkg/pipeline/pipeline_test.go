package pipeline

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"digital.vasic.rag/pkg/retriever"
)

// testRetriever is a mock retriever for testing.
type testRetriever struct {
	docs []retriever.Document
	err  error
}

func (r *testRetriever) Retrieve(
	_ context.Context,
	_ string,
	_ retriever.Options,
) ([]retriever.Document, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.docs, nil
}

// testReranker is a mock reranker for testing.
type testReranker struct {
	called bool
}

func (r *testReranker) Rerank(
	_ context.Context,
	_ string,
	docs []retriever.Document,
) ([]retriever.Document, error) {
	r.called = true
	// Reverse the order as a simple reranking
	result := make([]retriever.Document, len(docs))
	for i, d := range docs {
		result[len(docs)-1-i] = d
	}
	return result, nil
}

// testFormatter is a mock formatter for testing.
type testFormatter struct {
	called bool
}

func (f *testFormatter) Format(
	_ context.Context,
	docs []retriever.Document,
) (any, error) {
	f.called = true
	var parts []string
	for _, d := range docs {
		parts = append(parts, d.Content)
	}
	return strings.Join(parts, "\n"), nil
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, 10, cfg.RetrievalOpts.TopK)
}

func TestBuilder_Build(t *testing.T) {
	tests := []struct {
		name    string
		build   func() (*Pipeline, error)
		wantErr bool
	}{
		{
			name: "minimal valid pipeline",
			build: func() (*Pipeline, error) {
				return NewPipeline().
					Retrieve(&testRetriever{}).
					Build()
			},
		},
		{
			name: "pipeline with all stages",
			build: func() (*Pipeline, error) {
				return NewPipeline().
					Retrieve(&testRetriever{}).
					Rerank(&testReranker{}).
					Format(&testFormatter{}).
					Build()
			},
		},
		{
			name: "no retriever",
			build: func() (*Pipeline, error) {
				return NewPipeline().Build()
			},
			wantErr: true,
		},
		{
			name: "nil retriever",
			build: func() (*Pipeline, error) {
				return NewPipeline().
					Retrieve(nil).
					Build()
			},
			wantErr: true,
		},
		{
			name: "nil reranker",
			build: func() (*Pipeline, error) {
				return NewPipeline().
					Retrieve(&testRetriever{}).
					Rerank(nil).
					Build()
			},
			wantErr: true,
		},
		{
			name: "nil formatter",
			build: func() (*Pipeline, error) {
				return NewPipeline().
					Retrieve(&testRetriever{}).
					Format(nil).
					Build()
			},
			wantErr: true,
		},
		{
			name: "max stages exceeded",
			build: func() (*Pipeline, error) {
				return NewPipeline().
					WithConfig(Config{
						RetrievalOpts: retriever.DefaultOptions(),
						MaxStages:     1,
					}).
					Retrieve(&testRetriever{}).
					Rerank(&testReranker{}).
					Format(&testFormatter{}).
					Build()
			},
			wantErr: true,
		},
		{
			name: "custom stage",
			build: func() (*Pipeline, error) {
				return NewPipeline().
					Retrieve(&testRetriever{}).
					AddStage(StageFunc(func(
						_ context.Context,
						input any,
					) (any, error) {
						return input, nil
					})).
					Build()
			},
		},
		{
			name: "nil custom stage",
			build: func() (*Pipeline, error) {
				return NewPipeline().
					Retrieve(&testRetriever{}).
					AddStage(nil).
					Build()
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, err := tt.build()
			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, p)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, p)
			}
		})
	}
}

func TestPipeline_Execute(t *testing.T) {
	tests := []struct {
		name      string
		retriever *testRetriever
		reranker  *testReranker
		formatter *testFormatter
		wantErr   bool
		wantDocs  int
		wantFmt   string
	}{
		{
			name: "retrieve only",
			retriever: &testRetriever{docs: []retriever.Document{
				{ID: "1", Content: "hello"},
				{ID: "2", Content: "world"},
			}},
			wantDocs: 2,
		},
		{
			name: "retrieve and rerank",
			retriever: &testRetriever{docs: []retriever.Document{
				{ID: "1", Content: "first"},
				{ID: "2", Content: "second"},
			}},
			reranker: &testReranker{},
			wantDocs: 2,
		},
		{
			name: "full pipeline with formatter",
			retriever: &testRetriever{docs: []retriever.Document{
				{ID: "1", Content: "hello"},
				{ID: "2", Content: "world"},
			}},
			reranker:  &testReranker{},
			formatter: &testFormatter{},
			wantDocs:  2,
			wantFmt:   "world\nhello",
		},
		{
			name:      "retriever error",
			retriever: &testRetriever{err: fmt.Errorf("connection failed")},
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := NewPipeline().Retrieve(tt.retriever)
			if tt.reranker != nil {
				b = b.Rerank(tt.reranker)
			}
			if tt.formatter != nil {
				b = b.Format(tt.formatter)
			}

			p, err := b.Build()
			require.NoError(t, err)

			result, err := p.Execute(context.Background(), "test query")

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Len(t, result.Documents, tt.wantDocs)

			if tt.reranker != nil {
				assert.True(t, tt.reranker.called)
			}
			if tt.formatter != nil {
				assert.True(t, tt.formatter.called)
				assert.Equal(t, tt.wantFmt, result.Output)
			}
		})
	}
}

func TestPipeline_Execute_NoRetriever(t *testing.T) {
	// Build a pipeline but somehow without retriever (shouldn't happen via
	// builder, but test the Execute path)
	p := &Pipeline{
		retriever: nil,
		stages:    nil,
		config:    DefaultConfig(),
	}
	_, err := p.Execute(context.Background(), "query")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no retriever")
}

func TestWithQuery(t *testing.T) {
	ctx := WithQuery(context.Background(), "test query")
	query, ok := QueryFromContext(ctx)
	assert.True(t, ok)
	assert.Equal(t, "test query", query)
}

func TestQueryFromContext_Missing(t *testing.T) {
	_, ok := QueryFromContext(context.Background())
	assert.False(t, ok)
}

func TestStageFunc(t *testing.T) {
	stage := StageFunc(func(
		_ context.Context,
		input any,
	) (any, error) {
		docs := input.([]retriever.Document)
		return len(docs), nil
	})

	docs := []retriever.Document{{ID: "1"}, {ID: "2"}}
	result, err := stage.Process(context.Background(), docs)
	require.NoError(t, err)
	assert.Equal(t, 2, result)
}

func TestRerankerAdapter_WrongType(t *testing.T) {
	adapter := &rerankerAdapter{reranker: &testReranker{}}
	_, err := adapter.Process(context.Background(), "not docs")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected []retriever.Document")
}

func TestFormatterAdapter_WrongType(t *testing.T) {
	adapter := &formatterAdapter{formatter: &testFormatter{}}
	_, err := adapter.Process(context.Background(), 42)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "expected []retriever.Document")
}

func TestPipeline_CustomStageError(t *testing.T) {
	p, err := NewPipeline().
		Retrieve(&testRetriever{docs: []retriever.Document{
			{ID: "1", Content: "test"},
		}}).
		AddStage(StageFunc(func(
			_ context.Context,
			_ any,
		) (any, error) {
			return nil, fmt.Errorf("stage failed")
		})).
		Build()

	require.NoError(t, err)

	_, err = p.Execute(context.Background(), "query")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "stage 0 failed")
}
