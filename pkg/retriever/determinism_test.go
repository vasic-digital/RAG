package retriever

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// determinismRuns is the repetition count for the tie-order tests. Go
// randomises map iteration per range statement, so one call proves nothing.
const determinismRuns = 300

func idsOf(docs []Document) string {
	names := make([]string, len(docs))
	for i, d := range docs {
		names[i] = d.ID
	}
	return strings.Join(names, ",")
}

// TestMultiRetriever_RetrieveIsDeterministicOnTies is the §1.1
// paired-mutation test for MultiRetriever.Retrieve. It FAILS if the merged
// slice is rebuilt by ranging docMap, or if the comparator is reduced back to
// a bare `Score >` with no tiebreak.
func TestMultiRetriever_RetrieveIsDeterministicOnTies(t *testing.T) {
	a := &mockRetriever{docs: []Document{
		{ID: "m1", Content: "x", Score: 0.5},
		{ID: "m2", Content: "x", Score: 0.5},
		{ID: "m3", Content: "x", Score: 0.5},
	}}
	b := &mockRetriever{docs: []Document{
		{ID: "m4", Content: "x", Score: 0.5},
		{ID: "m5", Content: "x", Score: 0.5},
	}}
	m := NewMultiRetriever(a, b)

	const want = "m1,m2,m3,m4,m5"
	for i := 0; i < determinismRuns; i++ {
		got, err := m.Retrieve(context.Background(), "x", Options{TopK: 10})
		if err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
		if idsOf(got) != want {
			t.Fatalf(
				"run %d: byte-identical input produced order %q, want %q "+
					"— exactly-tied scores must order by ascending ID",
				i, idsOf(got), want,
			)
		}
	}
}

// TestMultiRetriever_TopKSurvivorIsStable is the consequence a caller sees:
// TopK truncation over an undefined tie order changes WHICH documents are
// returned.
func TestMultiRetriever_TopKSurvivorIsStable(t *testing.T) {
	a := &mockRetriever{docs: []Document{
		{ID: "m1", Content: "x", Score: 0.5},
		{ID: "m2", Content: "x", Score: 0.5},
		{ID: "m3", Content: "x", Score: 0.5},
	}}
	m := NewMultiRetriever(a)

	for i := 0; i < determinismRuns; i++ {
		got, err := m.Retrieve(context.Background(), "x", Options{TopK: 1})
		if err != nil {
			t.Fatalf("run %d: %v", i, err)
		}
		if len(got) != 1 || got[0].ID != "m1" {
			t.Fatalf("run %d: TopK=1 served %q, want m1", i, idsOf(got))
		}
	}
}

// TestMultiRetriever_TiebreakDoesNotMoveScores proves the tiebreak fires only
// on EXACT equality: strictly ordered scores keep their order and their
// values.
func TestMultiRetriever_TiebreakDoesNotMoveScores(t *testing.T) {
	a := &mockRetriever{docs: []Document{
		{ID: "z", Content: "x", Score: 0.9},
		{ID: "a", Content: "x", Score: 0.1},
	}}
	m := NewMultiRetriever(a)

	got, err := m.Retrieve(context.Background(), "x", Options{TopK: 10})
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if idsOf(got) != "z,a" {
		t.Fatalf("strict score order was broken by the ID tiebreak: %q", idsOf(got))
	}
	if got[0].Score != 0.9 || got[1].Score != 0.1 {
		t.Fatalf("scores moved: %v, %v", got[0].Score, got[1].Score)
	}
}

// scrambledDocs builds groups of exactly-tied documents separated by strictly
// different scores, with IDs assigned through a fixed scramble so that
// ID-ascending order is NOT already score-descending order. Without the
// scramble the input arrives pre-sorted, Go's pdqsort returns it untouched,
// and the test cannot distinguish a total comparator from a bare `Score >`
// one. 24 documents also clears pdqsort's 12-element insertion-sort cutoff.
func scrambledDocs(groups, perGroup int) []Document {
	n := groups * perGroup
	docs := make([]Document, 0, n)
	for g := 0; g < groups; g++ {
		for k := 0; k < perGroup; k++ {
			docs = append(docs, Document{
				ID:      fmt.Sprintf("d%03d", ((g*perGroup+k)*7+3)%n),
				Content: "x",
				Score:   float64(groups-g) / 10.0,
			})
		}
	}
	return docs
}

// TestMultiRetriever_TiebreakIsLoadBearing is the §1.1 mutation target for the
// COMPARATOR half of the MultiRetriever fix: it fails if the comparator is
// reduced to a bare `Score >` with no ID tiebreak.
func TestMultiRetriever_TiebreakIsLoadBearing(t *testing.T) {
	m := NewMultiRetriever(&mockRetriever{docs: scrambledDocs(6, 4)})

	first := ""
	for i := 0; i < determinismRuns; i++ {
		got, err := m.Retrieve(context.Background(), "x", Options{TopK: 100})
		if err != nil {
			t.Fatalf("run %d: %v", i, err)
		}

		ties := 0
		for j := 1; j < len(got); j++ {
			switch {
			case got[j-1].Score < got[j].Score:
				t.Fatalf("score order violated at %d: %q", j, idsOf(got))
			case got[j-1].Score == got[j].Score:
				ties++
				if got[j-1].ID >= got[j].ID {
					t.Fatalf(
						"exactly-tied scores at %d are not in ascending ID "+
							"order — the order among equal scores is "+
							"undefined: %q",
						j, idsOf(got),
					)
				}
			}
		}
		if ties == 0 {
			t.Fatal("fixture produced no exact ties — it cannot detect the defect")
		}

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
