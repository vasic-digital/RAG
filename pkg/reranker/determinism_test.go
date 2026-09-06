package reranker

import (
	"context"
	"strings"
	"testing"

	"digital.vasic.rag/pkg/retriever"
)

// determinismRuns is the repetition count for the tie-order tests.
const determinismRuns = 300

func rerankIDs(docs []retriever.Document) string {
	names := make([]string, len(docs))
	for i, d := range docs {
		names[i] = d.ID
	}
	return strings.Join(names, ",")
}

// TestMMRReranker_IsDeterministicOnTies is the §1.1 paired-mutation test for
// MMRReranker.Rerank. Five documents with identical content produce identical
// MMR scores at every greedy step, so the winner is decided entirely by the
// candidate scan order. It FAILS if the candidate set reverts to a
// `map[int]bool` ranged directly — Go randomises that iteration, and the
// pre-fix code produced 113 distinct orderings over 2000 calls.
func TestMMRReranker_IsDeterministicOnTies(t *testing.T) {
	docs := make([]retriever.Document, 0, 5)
	for _, id := range []string{"r1", "r2", "r3", "r4", "r5"} {
		docs = append(docs, retriever.Document{
			ID: id, Content: "alpha beta gamma", Score: 0.5,
		})
	}
	r := NewMMRReranker(Config{Lambda: 0.5, TopK: 5})

	const want = "r1,r2,r3,r4,r5"
	for i := 0; i < determinismRuns; i++ {
		got, err := r.Rerank(context.Background(), "alpha", docs)
		if err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
		if rerankIDs(got) != want {
			t.Fatalf(
				"run %d: byte-identical input produced order %q, want %q "+
					"— an exact MMR tie must resolve to the lowest "+
					"remaining index, never to map iteration order",
				i, rerankIDs(got), want,
			)
		}
	}
}

// TestScoreReranker_IsDeterministicOnTies is the paired-mutation test for
// ScoreReranker.Rerank. The input is deliberately in DESCENDING ID order so
// that a bare `Score >` comparator (unstable sort.Slice, no tiebreak) returns
// the input order and this test fails outright.
func TestScoreReranker_IsDeterministicOnTies(t *testing.T) {
	docs := []retriever.Document{
		{ID: "z", Content: "x", Score: 0.5},
		{ID: "y", Content: "x", Score: 0.5},
		{ID: "a", Content: "x", Score: 0.5},
	}
	r := NewScoreReranker(Config{TopK: 3})

	const want = "a,y,z"
	for i := 0; i < determinismRuns; i++ {
		got, err := r.Rerank(context.Background(), "q", docs)
		if err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
		if rerankIDs(got) != want {
			t.Fatalf(
				"run %d: got %q, want %q — exactly-tied scores must order "+
					"by ascending ID, independently of input order",
				i, rerankIDs(got), want,
			)
		}
	}
}

// TestScoreReranker_TiebreakDoesNotMoveScores proves the tiebreak fires only
// on EXACT equality.
func TestScoreReranker_TiebreakDoesNotMoveScores(t *testing.T) {
	docs := []retriever.Document{
		{ID: "z", Content: "x", Score: 0.9},
		{ID: "a", Content: "x", Score: 0.1},
	}
	got, err := NewScoreReranker(Config{TopK: 10}).
		Rerank(context.Background(), "q", docs)
	if err != nil {
		t.Fatalf("rerank: %v", err)
	}
	if rerankIDs(got) != "z,a" {
		t.Fatalf("strict score order was broken by the ID tiebreak: %q", rerankIDs(got))
	}
	if got[0].Score != 0.9 || got[1].Score != 0.1 {
		t.Fatalf("scores moved: %v, %v", got[0].Score, got[1].Score)
	}
}
