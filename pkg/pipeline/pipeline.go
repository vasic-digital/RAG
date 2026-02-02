// Package pipeline provides RAG pipeline composition with a fluent
// builder API for chaining retrieval, reranking, and formatting stages.
package pipeline

import (
	"context"
	"fmt"

	"digital.vasic.rag/pkg/retriever"
)

// Stage defines a processing stage in the RAG pipeline.
type Stage interface {
	// Process takes input data and returns processed output.
	Process(ctx context.Context, input any) (any, error)
}

// StageFunc adapts a function to the Stage interface.
type StageFunc func(ctx context.Context, input any) (any, error)

// Process calls the underlying function.
func (f StageFunc) Process(ctx context.Context, input any) (any, error) {
	return f(ctx, input)
}

// Config holds per-stage configuration for the pipeline.
type Config struct {
	// RetrievalOpts configures the retrieval stage.
	RetrievalOpts retriever.Options `json:"retrieval_opts"`
	// MaxStages limits the number of stages (0 = unlimited).
	MaxStages int `json:"max_stages"`
}

// DefaultConfig returns a default pipeline configuration.
func DefaultConfig() Config {
	return Config{
		RetrievalOpts: retriever.DefaultOptions(),
	}
}

// Pipeline chains multiple stages: Retriever -> Reranker -> Formatter
// and optional custom stages.
type Pipeline struct {
	retriever retriever.Retriever
	stages    []Stage
	config    Config
}

// Result holds the output of a pipeline execution.
type Result struct {
	Documents []retriever.Document `json:"documents"`
	Output    any                  `json:"output,omitempty"`
}

// Execute runs the pipeline: retrieves documents, then passes them
// through each stage in order.
func (p *Pipeline) Execute(
	ctx context.Context,
	query string,
) (*Result, error) {
	if p.retriever == nil {
		return nil, fmt.Errorf("pipeline has no retriever configured")
	}

	// Retrieval stage
	docs, err := p.retriever.Retrieve(ctx, query, p.config.RetrievalOpts)
	if err != nil {
		return nil, fmt.Errorf("retrieval failed: %w", err)
	}

	result := &Result{Documents: docs}

	// Run through stages
	var current any = docs
	for i, stage := range p.stages {
		output, err := stage.Process(ctx, current)
		if err != nil {
			return nil, fmt.Errorf("stage %d failed: %w", i, err)
		}
		current = output
	}

	result.Output = current
	return result, nil
}

// Builder provides a fluent API for constructing a Pipeline.
type Builder struct {
	retriever retriever.Retriever
	stages    []Stage
	config    Config
	err       error
}

// NewPipeline starts building a new Pipeline.
func NewPipeline() *Builder {
	return &Builder{
		config: DefaultConfig(),
	}
}

// WithConfig sets the pipeline configuration.
func (b *Builder) WithConfig(config Config) *Builder {
	b.config = config
	return b
}

// Retrieve sets the retriever for the pipeline.
func (b *Builder) Retrieve(r retriever.Retriever) *Builder {
	if r == nil {
		b.err = fmt.Errorf("retriever cannot be nil")
		return b
	}
	b.retriever = r
	return b
}

// Rerank adds a reranking stage. The stage expects []retriever.Document
// as input and returns []retriever.Document.
func (b *Builder) Rerank(reranker RerankerStage) *Builder {
	if reranker == nil {
		b.err = fmt.Errorf("reranker cannot be nil")
		return b
	}
	b.stages = append(b.stages, &rerankerAdapter{reranker: reranker})
	return b
}

// Format adds a formatting stage. The stage expects []retriever.Document
// as input and returns formatted output (string or any).
func (b *Builder) Format(formatter FormatterStage) *Builder {
	if formatter == nil {
		b.err = fmt.Errorf("formatter cannot be nil")
		return b
	}
	b.stages = append(b.stages, &formatterAdapter{formatter: formatter})
	return b
}

// AddStage adds a custom processing stage.
func (b *Builder) AddStage(stage Stage) *Builder {
	if stage == nil {
		b.err = fmt.Errorf("stage cannot be nil")
		return b
	}
	b.stages = append(b.stages, stage)
	return b
}

// Build constructs the Pipeline from the builder configuration.
func (b *Builder) Build() (*Pipeline, error) {
	if b.err != nil {
		return nil, b.err
	}
	if b.retriever == nil {
		return nil, fmt.Errorf("pipeline requires a retriever")
	}
	if b.config.MaxStages > 0 && len(b.stages) > b.config.MaxStages {
		return nil, fmt.Errorf(
			"too many stages: %d > max %d",
			len(b.stages), b.config.MaxStages,
		)
	}

	return &Pipeline{
		retriever: b.retriever,
		stages:    b.stages,
		config:    b.config,
	}, nil
}

// RerankerStage defines the reranking contract for pipeline integration.
type RerankerStage interface {
	Rerank(
		ctx context.Context,
		query string,
		docs []retriever.Document,
	) ([]retriever.Document, error)
}

// FormatterStage defines the formatting contract for pipeline integration.
type FormatterStage interface {
	Format(ctx context.Context, docs []retriever.Document) (any, error)
}

// rerankerAdapter wraps a RerankerStage as a Stage.
type rerankerAdapter struct {
	reranker RerankerStage
}

func (a *rerankerAdapter) Process(
	ctx context.Context,
	input any,
) (any, error) {
	docs, ok := input.([]retriever.Document)
	if !ok {
		return nil, fmt.Errorf(
			"reranker stage expected []retriever.Document, got %T", input,
		)
	}
	// Extract query from context if available, otherwise use empty string
	query, _ := ctx.Value(queryContextKey{}).(string)
	return a.reranker.Rerank(ctx, query, docs)
}

// formatterAdapter wraps a FormatterStage as a Stage.
type formatterAdapter struct {
	formatter FormatterStage
}

func (a *formatterAdapter) Process(
	ctx context.Context,
	input any,
) (any, error) {
	docs, ok := input.([]retriever.Document)
	if !ok {
		return nil, fmt.Errorf(
			"formatter stage expected []retriever.Document, got %T", input,
		)
	}
	return a.formatter.Format(ctx, docs)
}

// queryContextKey is used to pass the query through context to stages.
type queryContextKey struct{}

// WithQuery returns a context carrying the query string for pipeline stages.
func WithQuery(ctx context.Context, query string) context.Context {
	return context.WithValue(ctx, queryContextKey{}, query)
}

// QueryFromContext extracts the query string from context.
func QueryFromContext(ctx context.Context) (string, bool) {
	query, ok := ctx.Value(queryContextKey{}).(string)
	return query, ok
}
