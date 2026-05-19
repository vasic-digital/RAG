// Command runner is the round-278 RAG Challenge exerciser.
//
// It builds a real RAG pipeline (chunker -> KeywordRetriever -> Reranker)
// for every fixture locale in tests/fixtures/rag/payloads.json and asserts:
//
//  1. the pipeline composes without error (real Builder, real Build),
//  2. retrieval returns at least one document per locale,
//  3. the top-ranked document ID is in the locale's expected set,
//  4. score ordering is monotonically non-increasing (sort stability),
//  5. a custom Stage observed every locale (proves stages actually execute).
//
// No mocks beyond the in-memory KeywordRetriever (which IS real BM25 — it
// indexes the locale's corpus and answers from it). CONST-050(A) compliant:
// runner lives outside any _test.go file, exercises production packages
// directly, and prints captured runtime evidence per CONST-035.
//
// Usage:
//
//	go run ./challenges/runner
//
// Exits non-zero on any invariant violation. Honest exit codes:
//
//	0  — every locale produced valid evidence
//	1  — fixture loading failure or invariant violation
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"digital.vasic.rag/pkg/chunker"
	"digital.vasic.rag/pkg/hybrid"
	"digital.vasic.rag/pkg/pipeline"
	"digital.vasic.rag/pkg/reranker"
	"digital.vasic.rag/pkg/retriever"
)

type fixtureDoc struct {
	ID      string `json:"id"`
	Content string `json:"content"`
}

type localeFixture struct {
	Code           string       `json:"code"`
	Name           string       `json:"name"`
	Query          string       `json:"query"`
	Corpus         []fixtureDoc `json:"corpus"`
	ExpectTopIDIn  []string     `json:"expect_top_id_in"`
}

type payloads struct {
	SchemaVersion string          `json:"schema_version"`
	Purpose       string          `json:"purpose"`
	Locales       []localeFixture `json:"locales"`
}

func main() {
	start := time.Now()
	fmt.Println("=== RAG round-278 Challenge runner ===")

	fixturePath, err := locateFixtures()
	if err != nil {
		fail("locate fixtures: %v", err)
	}
	fmt.Printf("[1/4] Loaded fixtures: %s\n", fixturePath)

	pl, err := loadFixtures(fixturePath)
	if err != nil {
		fail("parse fixtures: %v", err)
	}
	if len(pl.Locales) < 5 {
		fail("expected >= 5 locales for bilingual coverage, got %d", len(pl.Locales))
	}
	fmt.Printf("[2/4] Schema %q, %d locales loaded\n", pl.SchemaVersion, len(pl.Locales))

	observed := map[string]bool{}
	stageHits := 0

	for _, loc := range pl.Locales {
		if err := exerciseLocale(loc, &stageHits); err != nil {
			fail("locale %s (%s): %v", loc.Code, loc.Name, err)
		}
		observed[loc.Code] = true
		fmt.Printf("    PASS locale=%s name=%-22s top_id_expected_in=%v\n",
			loc.Code, loc.Name, loc.ExpectTopIDIn)
	}

	if stageHits != len(pl.Locales) {
		fail("custom Stage executed %d times, expected %d (one per locale)",
			stageHits, len(pl.Locales))
	}
	fmt.Printf("[3/4] All %d locales exercised, custom Stage hits=%d\n",
		len(observed), stageHits)

	exerciseChunkers()
	fmt.Println("[4/4] Chunker trio (FixedSize / Recursive / Sentence) produced real chunks")

	fmt.Printf("\n=== PASS — runtime evidence captured in %s ===\n",
		time.Since(start).Round(time.Millisecond))
}

func locateFixtures() (string, error) {
	candidates := []string{
		"tests/fixtures/rag/payloads.json",
		"../tests/fixtures/rag/payloads.json",
		"../../tests/fixtures/rag/payloads.json",
	}
	for _, c := range candidates {
		abs, err := filepath.Abs(c)
		if err != nil {
			continue
		}
		if _, err := os.Stat(abs); err == nil {
			return abs, nil
		}
	}
	return "", fmt.Errorf("payloads.json not found in %v", candidates)
}

func loadFixtures(path string) (*payloads, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // fixed path inside repo
	if err != nil {
		return nil, err
	}
	var pl payloads
	if err := json.Unmarshal(raw, &pl); err != nil {
		return nil, err
	}
	return &pl, nil
}

func exerciseLocale(loc localeFixture, stageHits *int) error {
	ctx := context.Background()

	// Build a real BM25 KeywordRetriever and index the locale's corpus.
	kw := hybrid.NewKeywordRetriever()
	docs := make([]retriever.Document, 0, len(loc.Corpus))
	for _, d := range loc.Corpus {
		docs = append(docs, retriever.Document{ID: d.ID, Content: d.Content})
	}
	kw.Index(docs)

	// Real ScoreReranker — sorts by retrieval score (deterministic).
	// We also exercise MMRReranker below to prove the diversity-aware variant
	// runs without error, but the top-id assertion uses the score-sorted
	// output so tie-breaking on BM25 score equality is not platform-dependent.
	rr := reranker.NewScoreReranker(reranker.Config{TopK: 3})

	// Smoke-exercise MMR to keep it in the round-278 coverage ledger.
	mmr := reranker.NewMMRReranker(reranker.Config{Lambda: 0.5, TopK: 3})
	if _, mmrErr := mmr.Rerank(ctx, loc.Query, docs); mmrErr != nil {
		return fmt.Errorf("mmr smoke: %w", mmrErr)
	}

	// Custom locale-tagger Stage proves Pipeline.AddStage actually runs.
	tagger := pipeline.StageFunc(func(_ context.Context, input any) (any, error) {
		d, ok := input.([]retriever.Document)
		if !ok {
			return nil, fmt.Errorf("tagger: expected []Document, got %T", input)
		}
		out := make([]retriever.Document, len(d))
		for i, doc := range d {
			if doc.Metadata == nil {
				doc.Metadata = map[string]any{}
			}
			doc.Metadata["locale"] = loc.Code
			out[i] = doc
		}
		*stageHits++
		return out, nil
	})

	pl, err := pipeline.NewPipeline().
		WithConfig(pipeline.Config{
			RetrievalOpts: retriever.Options{TopK: 5, MinScore: 0.0},
		}).
		Retrieve(kw).
		Rerank(rr).
		AddStage(tagger).
		Build()
	if err != nil {
		return fmt.Errorf("build: %w", err)
	}

	result, err := pl.Execute(ctx, loc.Query)
	if err != nil {
		return fmt.Errorf("execute: %w", err)
	}
	if result == nil || len(result.Documents) == 0 {
		return fmt.Errorf("retrieval returned zero documents")
	}

	// Invariant 4: score ordering non-increasing on the retrieval output.
	if !sort.SliceIsSorted(result.Documents, func(i, j int) bool {
		return result.Documents[i].Score > result.Documents[j].Score
	}) {
		return fmt.Errorf("score ordering not non-increasing: %v", scores(result.Documents))
	}

	// Tagger output: confirm the custom stage actually mutated the docs.
	tagged, ok := result.Output.([]retriever.Document)
	if !ok || len(tagged) == 0 {
		return fmt.Errorf("tagger output absent or wrong type: %T", result.Output)
	}
	if got, _ := tagged[0].Metadata["locale"].(string); got != loc.Code {
		return fmt.Errorf("tagger did not stamp locale: got %q want %q", got, loc.Code)
	}

	// Invariant 3: top ID must be in the expected set for the locale.
	topID := tagged[0].ID
	allowed := false
	for _, want := range loc.ExpectTopIDIn {
		if topID == want {
			allowed = true
			break
		}
	}
	if !allowed {
		return fmt.Errorf("top_id=%q not in expected set %v", topID, loc.ExpectTopIDIn)
	}

	return nil
}

func exerciseChunkers() {
	// Real text long enough to force multi-chunk splits across all three strategies.
	text := "Retrieval-augmented generation pipelines compose chunkers, retrievers, " +
		"and rerankers into a single executable unit. Chunking splits a document " +
		"into overlapping segments that an embedding model can encode. The " +
		"retriever scores each chunk against a query. The reranker improves " +
		"the final ordering, balancing relevance against diversity."

	for name, c := range map[string]chunker.Chunker{
		"FixedSize": chunker.NewFixedSizeChunker(chunker.Config{ChunkSize: 80, Overlap: 16}),
		"Recursive": chunker.NewRecursiveChunker(chunker.Config{ChunkSize: 80, Overlap: 16}),
		"Sentence":  chunker.NewSentenceChunker(chunker.Config{ChunkSize: 100, Overlap: 0}),
	} {
		chunks := c.Chunk(text)
		if len(chunks) == 0 {
			fail("chunker %s returned zero chunks for non-empty input", name)
		}
		for i, ch := range chunks {
			if ch.End < ch.Start {
				fail("chunker %s chunk[%d] has End<Start (%d<%d)", name, i, ch.End, ch.Start)
			}
		}
	}
}

func scores(docs []retriever.Document) []float64 {
	out := make([]float64, len(docs))
	for i, d := range docs {
		out[i] = d.Score
	}
	return out
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "FAIL: "+format+"\n", args...)
	os.Exit(1)
}
