# Contributing

Thank you for your interest in contributing to the `digital.vasic.rag` module. This guide describes the process and standards for contributions.

## Prerequisites

- Go 1.24 or later
- `gofmt` and `go vet` (included with Go)
- `github.com/stretchr/testify` (test dependency, managed via `go.mod`)

## Getting Started

```bash
# Clone the repository
git clone <repository-url>
cd RAG

# Verify the build and tests pass
go build ./...
go test ./... -count=1 -race
```

## Development Workflow

### 1. Create a Branch

Use conventional branch naming:

```bash
git checkout -b feat/new-chunking-strategy
git checkout -b fix/mmr-score-normalization
git checkout -b test/hybrid-edge-cases
git checkout -b docs/update-api-reference
```

### 2. Make Changes

Follow the code style conventions described below. Each package is a single file pair:

```
pkg/<package>/
    <package>.go        # All production code
    <package>_test.go   # All tests
```

Do not split packages into multiple source files without prior approval.

### 3. Run Checks

```bash
# Format code
gofmt -w .

# Run vet
go vet ./...

# Run all tests with race detection
go test ./... -count=1 -race
```

### 4. Commit

Use Conventional Commits format:

```
<type>(<package>): <description>

# Types: feat, fix, test, refactor, docs, chore
# Examples:
feat(chunker): add semantic-aware chunking strategy
fix(reranker): correct MMR score normalization
test(hybrid): add BM25 edge case coverage
refactor(pipeline): simplify builder validation logic
docs(retriever): update Document field descriptions
```

### 5. Submit a Pull Request

- Ensure all tests pass with race detection
- Provide a clear description of the change and its motivation
- Reference any related issues

## Code Style

### General

- Standard Go conventions per [Effective Go](https://go.dev/doc/effective_go)
- `gofmt` formatting (no exceptions)
- Line length: 100 characters maximum for readability
- Imports grouped: stdlib, third-party, internal (separated by blank lines)

### Naming

- `camelCase` for unexported identifiers
- `PascalCase` for exported identifiers
- `UPPER_SNAKE_CASE` for constants
- Acronyms in all-caps: `ID`, `URL`, `HTTP`, `BM25`, `MMR`, `RRF`
- Receivers: 1-2 letters (e.g., `c` for chunker, `r` for retriever/reranker, `m` for multi-retriever, `h` for hybrid, `b` for builder, `p` for pipeline)

### Error Handling

- Always check errors
- Wrap errors with context: `fmt.Errorf("retrieval failed: %w", err)`
- Use `defer` for cleanup

### Interfaces

- Keep interfaces small and focused (1-2 methods)
- Accept interfaces, return structs
- Define interfaces in the package that uses them (except `retriever.Retriever` which is the shared contract)

### Concurrency

- Always pass `context.Context` as the first parameter for methods that may block
- Use `sync.RWMutex` for read-heavy shared state
- Use `sync.WaitGroup` for parallel operations
- Never leak goroutines

## Testing Standards

### Test Naming

```go
func Test<Struct>_<Method>_<Scenario>(t *testing.T) {
    // ...
}
```

Examples:
- `TestMultiRetriever_Retrieve`
- `TestFixedSizeChunker_Chunk_EmptyText`
- `TestMMRReranker_Rerank_SingleDocument`

### Table-Driven Tests

Prefer table-driven tests for methods with multiple scenarios:

```go
func TestMyType_Method(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    int
        wantErr bool
    }{
        {name: "valid input", input: "hello", want: 5},
        {name: "empty input", input: "", wantErr: true},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := myType.Method(tt.input)
            if tt.wantErr {
                require.Error(t, err)
                return
            }
            require.NoError(t, err)
            assert.Equal(t, tt.want, got)
        })
    }
}
```

### Assertions

Use `testify`:
- `require.NoError` / `require.Error` for error checks (stops test on failure)
- `assert.Equal`, `assert.Len`, `assert.Contains` for value checks (continues on failure)

### Race Detection

All tests must pass with the race detector:

```bash
go test ./... -count=1 -race
```

### Mocks

Mock implementations are permitted only in `_test.go` files. Never place mocks, stubs, or fakes in production code.

## Adding New Components

### New Chunking Strategy

1. Add the struct and constructor to `pkg/chunker/chunker.go`
2. Implement the `Chunker` interface
3. Validate configuration in the constructor (clamp invalid values)
4. Add comprehensive tests to `pkg/chunker/chunker_test.go`
5. Update `docs/API_REFERENCE.md` and `docs/USER_GUIDE.md`

### New Reranking Strategy

1. Add the struct and constructor to `pkg/reranker/reranker.go`
2. Implement the `Reranker` interface
3. Return copies of the input documents (never mutate the input slice)
4. Add tests to `pkg/reranker/reranker_test.go`
5. Update documentation

### New Fusion Strategy

1. Add the struct and constructor to `pkg/hybrid/hybrid.go`
2. Implement the `FusionStrategy` interface
3. Return results sorted by fused score descending
4. Add tests to `pkg/hybrid/hybrid_test.go`
5. Update documentation

## Boundaries

### What This Module Is

- Generic, reusable RAG primitives
- Zero runtime dependencies (testify is test-only)
- Foundation layer for application-specific RAG systems

### What This Module Is Not

- Not a complete RAG application
- Not a vector database client (implementations belong in consumers)
- Not an embedding provider (implementations belong in consumers)
- Not an LLM client (implementations belong in consumers)

Do not add application-specific logic, external service integrations, or runtime dependencies.

## Questions

If you are unsure about a change or need guidance, open an issue for discussion before starting work.
