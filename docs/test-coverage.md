# RAG Test Coverage Ledger — round-278

CONST-050(B) symbol → test ledger for `digital.vasic.rag`. Every exported
symbol in `pkg/{chunker,retriever,reranker,hybrid,pipeline}` is listed
alongside the unit test, integration test, and Challenge that exercise it
end-to-end. A gap in this table is a CONST-050(B) violation of the same
severity as a §11.4 PASS-bluff at the release-gate layer.

> Verbatim 2026-05-19 operator mandate: *"all existing tests and Challenges
> do work in anti-bluff manner — they MUST confirm that all tested codebase
> really works as expected!"*

## Conventions

- **Unit** — `pkg/<pkg>/<file>_test.go`, mocks permitted per CONST-050(A).
- **Integration** — `tests/integration/`, real chunkers/retrievers/rerankers
  composed; no mocks beyond the unit boundary.
- **E2E** — `tests/e2e/`, full pipeline executions on real corpora.
- **Stress** — `tests/stress/`, sustained load on chunker/reranker hot paths.
- **Security** — `tests/security/`, input fuzzing + payload-size limits.
- **Bench** — `tests/benchmark/`, perf SLO ratchets for chunker/MMR.
- **Challenge** — `challenges/scripts/*.sh` + round-278
  `challenges/runner/main.go` + `challenges/rag_describe_challenge.sh`.

## pkg/chunker

| Symbol | Unit | Stress | E2E / Challenge |
|---|---|---|---|
| `type Chunk struct{}` | `chunker_test.go` | `stress_test.go` | `runner/main.go` (asserts non-empty chunks per fixture) |
| `type Chunker interface{}` | `chunker_test.go` | — | `runner/main.go` (drives 3 implementations) |
| `type Config struct{}` | `chunker_test.go` (DefaultConfig) | `stress_test.go` | `runner/main.go` (custom ChunkSize per locale) |
| `DefaultConfig()` | `chunker_test.go` | — | `rag_describe_challenge.sh` (clean exit 0) |
| `FixedSizeChunker` + `NewFixedSizeChunker` + `Chunk` | `chunker_test.go` | `stress_test.go` | `runner/main.go` (English/Spanish prompts) |
| `RecursiveChunker` + `NewRecursiveChunker` + `Chunk` + `SplitRecursiveForTesting` + `MergeAndOverlapForTesting` | `chunker_test.go` | `stress_test.go` | `runner/main.go` (German/Japanese prompts) |
| `SentenceChunker` + `NewSentenceChunker` + `Chunk` | `chunker_test.go` | `stress_test.go` | `runner/main.go` (Serbian Cyrillic prompts) |

## pkg/retriever

| Symbol | Unit | Integration | E2E / Challenge |
|---|---|---|---|
| `type Document struct{}` | `retriever_test.go` | `tests/integration` | `runner/main.go` (asserts ID/Content/Score round-trip) |
| `type Options struct{}` + `DefaultOptions()` | `retriever_test.go` | `tests/integration` | `runner/main.go` (TopK=3 per locale) |
| `type Retriever interface{}` | `retriever_test.go` | `tests/integration` | `runner/main.go` (in-memory impl) |
| `MultiRetriever` + `NewMultiRetriever` + `AddRetriever` + `Retrieve` | `retriever_test.go` (parallel + dedup + sort) | `tests/integration` (2-retriever merge) | `runner/main.go` (3-retriever fan-out) |

## pkg/reranker

| Symbol | Unit | Integration | E2E / Challenge |
|---|---|---|---|
| `type Reranker interface{}` | `reranker_test.go` | `tests/integration` | `runner/main.go` (score + MMR variants) |
| `type Config struct{}` + `DefaultConfig()` | `reranker_test.go` | — | `runner/main.go` (Lambda=0.5) |
| `ScoreReranker` + `NewScoreReranker` + `Rerank` | `reranker_test.go` (sort stability) | `tests/integration` | `runner/main.go` (English/Spanish) |
| `MMRReranker` + `NewMMRReranker` + `Rerank` | `reranker_test.go` (diversity vs relevance) | `tests/integration` | `runner/main.go` (German/Japanese diversity proof) |

## pkg/hybrid

| Symbol | Unit | Integration | E2E / Challenge |
|---|---|---|---|
| `KeywordRetriever` (BM25) | `hybrid_test.go` | `tests/integration` | `runner/main.go` (keyword leg) |
| `SemanticRetriever` (interface) | `hybrid_test.go` | `tests/integration` | `runner/main.go` (in-memory embedding stub) |
| `HybridRetriever` + `Retrieve` | `hybrid_test.go` (fusion correctness) | `tests/integration` | `runner/main.go` (RRF + linear) |
| `ReciprocalRankFusion` + `LinearCombination` | `hybrid_test.go` | — | `rag_describe_challenge.sh` (sanity invocation) |

## pkg/pipeline

| Symbol | Unit | Integration | E2E / Challenge |
|---|---|---|---|
| `type Stage interface{}` + `StageFunc` | `pipeline_test.go` | `tests/integration` | `runner/main.go` (custom locale-tagger Stage) |
| `type Config struct{}` + `DefaultConfig()` | `pipeline_test.go` | — | `runner/main.go` |
| `Pipeline` + `Execute` | `pipeline_test.go` | `tests/integration` | `runner/main.go` (end-to-end per locale) |
| `Builder` + `NewPipeline` + `WithConfig` + `Retrieve` + `Rerank` + `Format` + `AddStage` + `Build` | `pipeline_test.go` (nil-guard + max-stages) | `tests/integration` | `runner/main.go` (full fluent chain) |
| `RerankerStage` + `FormatterStage` | `pipeline_test.go` (adapter) | `tests/integration` | `runner/main.go` |

## Anti-bluff invariants this ledger enforces

1. **No symbol left untested.** Adding a public function to `pkg/`
   without a row here is a CONST-050(B) violation surfaced at the
   release-gate sweep.
2. **No mock-only coverage beyond Unit column.** The Integration / E2E /
   Challenge columns reference real implementations exercising real text
   and real corpora (in-memory but functionally complete).
3. **5-locale bilingual proof.** The Challenge column always points to
   `runner/main.go` which drives fixtures from `tests/fixtures/rag/payloads.json`
   across en/sr/ja/es/de.
4. **Paired-mutation evidence.** `rag_describe_challenge.sh` proves the
   gate has the ability to fail by returning exit 99 on
   `--anti-bluff-mutate`.

## How to regenerate this table

The table is currently hand-maintained. Future round will add
`scripts/generate-test-coverage.sh` walking `go doc -all ./pkg/...`
output and cross-referencing test source. Until then: every PR adding
or removing a public symbol MUST update the matching row in the same
commit, per CONST-049 step 4 / CONST-050(B) ledger discipline.
