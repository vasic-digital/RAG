package hybrid

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"digital.vasic.rag/pkg/retriever"
)

// determinismRuns is the repetition count used by every test in this file.
// Go randomises map iteration per range statement, so a single call proves
// nothing; the pre-fix defect produced 5 distinct orderings over 2000 calls.
const determinismRuns = 300

func idsOf(docs []retriever.Document) string {
	names := make([]string, len(docs))
	for i, d := range docs {
		names[i] = d.ID
	}
	return strings.Join(names, ",")
}

// tiedLegs builds five single-document result sets. Each document holds rank
// 0 in its own leg, so every RRF score is exactly 1/(k+1) — five EXACT ties.
func tiedLegs() [][]retriever.Document {
	sets := make([][]retriever.Document, 0, 5)
	for _, id := range []string{"d1", "d2", "d3", "d4", "d5"} {
		sets = append(sets, []retriever.Document{
			{ID: id, Content: id + " alpha beta gamma"},
		})
	}
	return sets
}

// TestRRFStrategy_FuseIsDeterministicOnTies is the §1.1 paired-mutation test
// for RRFStrategy.Fuse. It FAILS if the (score, ID) tiebreak is reduced back
// to a bare `Score >` comparison, or if the slice is rebuilt by ranging the
// score map instead of by sorted ID.
func TestRRFStrategy_FuseIsDeterministicOnTies(t *testing.T) {
	sets := tiedLegs()
	rrf := NewRRFStrategy(60)

	const want = "d1,d2,d3,d4,d5"
	for i := 0; i < determinismRuns; i++ {
		got := idsOf(rrf.Fuse(sets...))
		if got != want {
			t.Fatalf(
				"run %d: byte-identical input produced order %q, want %q "+
					"— exactly-tied scores must order by ascending ID",
				i, got, want,
			)
		}
	}
}

// TestRRFStrategy_FuseTopKSurvivorIsStable is the consequence that actually
// reaches a caller: Fuse output is truncated to TopK, so an undefined tie
// order changes WHICH documents are served.
func TestRRFStrategy_FuseTopKSurvivorIsStable(t *testing.T) {
	sem := &mockRetriever{docs: []retriever.Document{{ID: "h1", Content: "alpha"}}}
	kw := &mockRetriever{docs: []retriever.Document{{ID: "h2", Content: "alpha"}}}
	h := NewHybridRetriever(sem, kw, NewRRFStrategy(60), DefaultHybridConfig())

	for i := 0; i < determinismRuns; i++ {
		got, err := h.Retrieve(
			context.Background(), "alpha", retriever.Options{TopK: 1},
		)
		if err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
		if len(got) != 1 || got[0].ID != "h1" {
			t.Fatalf(
				"run %d: TopK=1 served %q, want h1 — the surviving document "+
					"must not change between identical calls",
				i, idsOf(got),
			)
		}
	}
}

// TestLinearStrategy_FuseIsDeterministicOnTies is the paired-mutation test
// for LinearStrategy.Fuse.
func TestLinearStrategy_FuseIsDeterministicOnTies(t *testing.T) {
	sets := make([][]retriever.Document, 0, 5)
	for _, id := range []string{"d1", "d2", "d3", "d4", "d5"} {
		sets = append(sets, []retriever.Document{{ID: id, Content: id, Score: 1}})
	}
	lin := NewLinearStrategy()

	const want = "d1,d2,d3,d4,d5"
	for i := 0; i < determinismRuns; i++ {
		if got := idsOf(lin.Fuse(sets...)); got != want {
			t.Fatalf("run %d: got %q, want %q", i, got, want)
		}
	}
}

// TestKeywordRetriever_RetrieveIsDeterministicOnTies is the paired-mutation
// test for the BM25 path. Five documents with identical content score
// identically, so their order is decided entirely by the tiebreak.
func TestKeywordRetriever_RetrieveIsDeterministicOnTies(t *testing.T) {
	docs := []retriever.Document{
		{ID: "k1", Content: "alpha beta gamma"},
		{ID: "k2", Content: "alpha beta gamma"},
		{ID: "k3", Content: "alpha beta gamma"},
		{ID: "k4", Content: "alpha beta gamma"},
		{ID: "k5", Content: "alpha beta gamma"},
	}

	const want = "k1,k2,k3,k4,k5"
	for i := 0; i < determinismRuns; i++ {
		r := NewKeywordRetriever()
		r.Index(docs)
		got, err := r.Retrieve(
			context.Background(), "alpha", retriever.Options{TopK: 10},
		)
		if err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
		if idsOf(got) != want {
			t.Fatalf("run %d: got %q, want %q", i, idsOf(got), want)
		}
	}
}

// TestFuse_TiebreakDoesNotMoveScores guards the other half of the contract:
// the tiebreak must define the order among EXACTLY equal scores and change
// nothing else. Every score is checked against an independent evaluation of
// the documented RRF formula, and no pair with differing scores may be
// reordered.
func TestFuse_TiebreakDoesNotMoveScores(t *testing.T) {
	const k = 60.0
	setA := []retriever.Document{
		{ID: "b"}, {ID: "a"}, {ID: "c"}, {ID: "e"},
	}
	setB := []retriever.Document{
		{ID: "d"}, {ID: "a"}, {ID: "f"},
	}

	want := map[string]float64{}
	for _, set := range [][]retriever.Document{setA, setB} {
		for rank, d := range set {
			want[d.ID] += 1.0 / (k + float64(rank+1))
		}
	}

	got := NewRRFStrategy(k).Fuse(setA, setB)
	if len(got) != len(want) {
		t.Fatalf("population changed: got %d documents, want %d", len(got), len(want))
	}
	for _, d := range got {
		if d.Score != want[d.ID] {
			t.Fatalf(
				"score moved for %s: got %.17g, want %.17g — the fix must "+
					"not touch scores",
				d.ID, d.Score, want[d.ID],
			)
		}
	}

	ties := 0
	for i := 1; i < len(got); i++ {
		switch {
		case got[i-1].Score < got[i].Score:
			t.Fatalf("score order violated at %d: %q", i, idsOf(got))
		case got[i-1].Score == got[i].Score:
			ties++
			if got[i-1].ID >= got[i].ID {
				t.Fatalf(
					"exact tie at %d not broken by ascending ID: %q",
					i, idsOf(got),
				)
			}
		}
	}
	if ties == 0 {
		t.Fatal("fixture produced no exact ties — it cannot detect the defect")
	}
}

// scrambledLegs builds m legs of l ranks each. A document at rank r appears in
// exactly one leg, so the m documents sharing rank r tie EXACTLY at
// 1/(60+r+1) while the l rank groups have strictly different scores — the
// shape reciprocal-rank fusion actually produces.
//
// IDs are assigned through a fixed scramble on purpose. If IDs ascended in
// step with the score order the input would already be sorted, Go's pdqsort
// would recognise that and return it untouched, and the test could not tell a
// total comparator from a bare `Score >` one. Measured: with ascending IDs a
// tiebreak-less comparator deviates 0 times in 200; with scrambled IDs and
// 24 documents it deviates 200 times in 200.
func scrambledLegs(m, l int) [][]retriever.Document {
	n := m * l
	sets := make([][]retriever.Document, m)
	for j := 0; j < m; j++ {
		for r := 0; r < l; r++ {
			id := fmt.Sprintf("d%03d", ((r*m+j)*7+3)%n)
			sets[j] = append(sets[j], retriever.Document{ID: id, Content: id})
		}
	}
	return sets
}

// assertTotalOrder checks the two halves of the contract without assuming any
// particular implementation: scores must be non-increasing, and within every
// run of EXACTLY equal scores the IDs must strictly ascend.
func assertTotalOrder(t *testing.T, docs []retriever.Document) {
	t.Helper()
	ties := 0
	for i := 1; i < len(docs); i++ {
		switch {
		case docs[i-1].Score < docs[i].Score:
			t.Fatalf("score order violated at index %d: %q", i, idsOf(docs))
		case docs[i-1].Score == docs[i].Score:
			ties++
			if docs[i-1].ID >= docs[i].ID {
				t.Fatalf(
					"exactly-tied scores at index %d are not in ascending ID "+
						"order — the order among equal scores is undefined: %q",
					i, idsOf(docs),
				)
			}
		}
	}
	if ties == 0 {
		t.Fatal("fixture produced no exact ties — it cannot detect the defect")
	}
}

// TestRRFStrategy_TiebreakIsLoadBearing is the §1.1 mutation target for the
// COMPARATOR half of the fix. Unlike the five-document fixtures above, this
// one is large enough (24 documents) and scrambled enough that Go's unstable
// sort genuinely permutes tied documents, so removing the ID tiebreak from
// sortByScoreThenID makes it fail.
func TestRRFStrategy_TiebreakIsLoadBearing(t *testing.T) {
	sets := scrambledLegs(4, 6)
	rrf := NewRRFStrategy(60)

	first := ""
	for i := 0; i < determinismRuns; i++ {
		got := rrf.Fuse(sets...)
		assertTotalOrder(t, got)
		if i == 0 {
			first = idsOf(got)
			continue
		}
		if idsOf(got) != first {
			t.Fatalf(
				"run %d: byte-identical input produced a different order\n"+
					" run 0: %s\n run %d: %s",
				i, first, i, idsOf(got),
			)
		}
	}
}

// TestLinearStrategy_TiebreakIsLoadBearing is the same mutation target for the
// linear fusion path.
func TestLinearStrategy_TiebreakIsLoadBearing(t *testing.T) {
	sets := scrambledLegs(4, 6)
	for _, set := range sets {
		for i := range set {
			set[i].Score = 1
		}
	}
	lin := NewLinearStrategy()

	first := ""
	for i := 0; i < determinismRuns; i++ {
		got := lin.Fuse(sets...)
		assertTotalOrder(t, got)
		if i == 0 {
			first = idsOf(got)
			continue
		}
		if idsOf(got) != first {
			t.Fatalf("run %d: order changed\n run 0: %s\n run %d: %s",
				i, first, i, idsOf(got))
		}
	}
}
