package grounding

import (
	"context"
	"errors"
	"strings"
	"testing"

	"digital.vasic.rag/pkg/retriever"
)

// Every check below is paired with a MUTATION that breaks the condition the
// check guards and asserts the check then reports failure. A gate with no
// demonstrated failure mode proves nothing: it might be passing because the
// property holds, or because the check never looked.
//
// All fixture content is SYNTHETIC. No material from any private corpus,
// recording or transcript appears in this file, and none may be added.

func documents() []retriever.Document {
	return []retriever.Document{
		{ID: "0f3c9a1e", Content: "The valve closes when the cistern level drops below 20 percent.", Source: "docs/a.md", Score: 0.81},
		{ID: "7b21d4c0", Content: "Zone three uses probe model AX-107.", Source: "docs/b.md", Score: 0.44},
		{ID: "11aa22bb", Content: "Scheduled runs begin at civil dawn.", Source: "docs/c.md", Score: 0.41},
	}
}

// ---------------------------------------------------------------------------
// CHECK 1 — extractive cannot fabricate: a question with no supporting document
// must be DECLINED, and no claim may be returned.
// ---------------------------------------------------------------------------

func TestExtractive_DeclinesWhenNoDocumentSupports(t *testing.T) {
	e := NewExtractive(3)
	// Calibrated floor above every passage score: nothing supports the question.
	req := Request{
		Question:  "What firmware version is installed?",
		Documents: documents(),
		MinScore:  0.90,
	}
	got, err := e.Answer(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Verdict != VerdictDeclined {
		t.Fatalf("verdict = %q, want %q", got.Verdict, VerdictDeclined)
	}
	if len(got.Claims) != 0 {
		t.Fatalf("declined answer carried %d claims; a decline must carry none", len(got.Claims))
	}
	if got.Reason == "" {
		t.Fatal("decline carried no reason")
	}
	// The documents examined are still shown, so a reader can see WHAT was
	// looked at rather than only being told it failed.
	if len(got.Considered) != 3 {
		t.Fatalf("considered = %d, want 3", len(got.Considered))
	}
}

// MUTATION for CHECK 1: drop the retrieval floor to admit everything. The
// adapter now answers a question nothing supports, and the check above must
// report FAIL. This proves CHECK 1 is load-bearing rather than incidentally
// green.
func TestMutation_Check1FailsWhenGateDisabled(t *testing.T) {
	e := NewExtractive(3)
	req := Request{
		Question:  "What firmware version is installed?",
		Documents: documents(),
		MinScore:  0.01, // MUTATION: floor below every score
	}
	got, _ := e.Answer(context.Background(), req)
	if got.Verdict == VerdictDeclined {
		t.Fatal("MUTATION INEFFECTIVE: still declined with the gate disabled; " +
			"CHECK 1 would pass either way and therefore proves nothing")
	}
	if got.Verdict != VerdictAnswered {
		t.Fatalf("MUTATION: expected the disabled gate to answer, got %q", got.Verdict)
	}
	t.Logf("PAIRED PROOF OK: with min_score=0.01 the adapter answered "+
		"(%d claims) a question the calibrated gate declines — CHECK 1 detects the difference",
		len(got.Claims))
}

// ---------------------------------------------------------------------------
// CHECK 2 — `unavailable` is distinguishable from `declined`.
// Conflating them makes a missing provider look like a content gap.
// ---------------------------------------------------------------------------

func TestUnavailableIsDistinctFromDeclined(t *testing.T) {
	e := NewExtractive(3)

	// Uncalibrated thresholds: the gate cannot do its job. This is "unable to
	// verify", NOT "the content does not answer this".
	unavail, _ := e.Answer(context.Background(), Request{
		Question: "anything", Documents: documents(), MinScore: 0, // uncalibrated
	})
	// Calibrated, but nothing clears the floor: a genuine content gap.
	declined, _ := e.Answer(context.Background(), Request{
		Question: "anything", Documents: documents(), MinScore: 0.90,
	})

	if unavail.Verdict != VerdictUnavailable {
		t.Fatalf("uncalibrated gate gave %q, want %q", unavail.Verdict, VerdictUnavailable)
	}
	if declined.Verdict != VerdictDeclined {
		t.Fatalf("calibrated miss gave %q, want %q", declined.Verdict, VerdictDeclined)
	}
	if unavail.Verdict == declined.Verdict {
		t.Fatal("the two states are indistinguishable")
	}
	if !strings.Contains(unavail.Reason, "uncalibrated") {
		t.Fatalf("unavailable reason does not name the cause: %q", unavail.Reason)
	}
	t.Logf("unavailable=%q\ndeclined=%q", unavail.Reason, declined.Reason)
}

// MUTATION for CHECK 2: treat an uncalibrated gate as an ordinary decline —
// the exact conflation this project has fixed repeatedly. CHECK 2 must fail.
func TestMutation_Check2FailsWhenStatesAreConflated(t *testing.T) {
	conflated := func(req Request) Verdict {
		g := RetrievalGate(req)
		if g.Verdict == VerdictUnavailable {
			return VerdictDeclined // MUTATION: fold "cannot tell" into "no"
		}
		return g.Verdict
	}
	u := conflated(Request{Documents: documents(), MinScore: 0})
	d := conflated(Request{Documents: documents(), MinScore: 0.90})
	if u != d {
		t.Fatalf("MUTATION INEFFECTIVE: states still differ (%q vs %q)", u, d)
	}
	t.Logf("PAIRED PROOF OK: with the conflation in place both paths report %q — "+
		"a missing/uncalibrated provider becomes indistinguishable from a content gap, "+
		"which is exactly what CHECK 2 detects", u)
}

// ---------------------------------------------------------------------------
// CHECK 3 — deterministic citation validation rejects an invented identifier.
// ---------------------------------------------------------------------------

func TestValidateCitations_RejectsInventedIdentifier(t *testing.T) {
	req := Request{Documents: documents()}
	err := ValidateCitations(req, []Claim{{
		Text:      "The valve closes when the cistern level drops below 20 percent.",
		Citations: []Citation{{DocumentID: "deadbeef"}}, // never placed in the request
	}})
	if err == nil {
		t.Fatal("an invented document id was accepted")
	}
	if !errors.Is(err, ErrFabricated) {
		t.Fatalf("error not classified as fabrication: %v", err)
	}
	if err2 := ValidateCitations(req, []Claim{{Text: "x", Citations: nil}}); err2 == nil {
		t.Fatal("a claim with zero citations was accepted")
	}
}

// MUTATION for CHECK 3: a validator that only checks the id is non-empty —
// the plausible-looking weakening. CHECK 3 must fail against it.
func TestMutation_Check3FailsWithNonEmptyOnlyValidator(t *testing.T) {
	weak := func(_ Request, claims []Claim) error {
		for _, c := range claims {
			for _, cit := range c.Citations {
				if cit.DocumentID == "" {
					return ErrFabricated
				}
			}
		}
		return nil // MUTATION: never checks set membership
	}
	err := weak(Request{Documents: documents()}, []Claim{{
		Text: "invented", Citations: []Citation{{DocumentID: "deadbeef"}},
	}})
	if err != nil {
		t.Fatal("MUTATION INEFFECTIVE: the weakened validator still rejected the id")
	}
	t.Log("PAIRED PROOF OK: a non-empty-only validator accepts the invented id " +
		"`deadbeef`; CHECK 3's set-membership test is what rejects it")
}

// ---------------------------------------------------------------------------
// CHECK 4 — the verbatim guarantee: extractive claim text IS document text.
// ---------------------------------------------------------------------------

func TestExtractive_ClaimsAreVerbatimDocumentText(t *testing.T) {
	e := NewExtractive(3)
	req := Request{Question: "cistern?", Documents: documents(), MinScore: 0.40}
	got, _ := e.Answer(context.Background(), req)
	if got.Verdict != VerdictAnswered {
		t.Fatalf("verdict = %q, want answered", got.Verdict)
	}
	if err := ValidateVerbatim(req, got.Claims); err != nil {
		t.Fatalf("extractive produced non-verbatim text: %v", err)
	}
	byID := map[string]string{}
	for _, d := range req.Documents {
		byID[d.ID] = d.Content
	}
	for i, c := range got.Claims {
		if c.Text != byID[c.Citations[0].DocumentID] {
			t.Fatalf("claim %d is not byte-identical to its cited document", i)
		}
	}
}

// MUTATION for CHECK 4: a summarising extractor — the change someone would
// plausibly make to "improve" the output. CHECK 4 must catch it.
func TestMutation_Check4FailsWhenExtractorSummarises(t *testing.T) {
	req := Request{Documents: documents()}
	summarised := []Claim{{
		Text:      "The valve closes at low cistern level.", // MUTATION: paraphrase
		Citations: []Citation{{DocumentID: "0f3c9a1e"}},
	}}
	// Citation validation alone does NOT catch this: the id is real.
	if err := ValidateCitations(req, summarised); err != nil {
		t.Fatalf("citation check unexpectedly rejected a real id: %v", err)
	}
	// The verbatim check is what catches it.
	err := ValidateVerbatim(req, summarised)
	if err == nil {
		t.Fatal("MUTATION INEFFECTIVE: paraphrased text passed the verbatim check")
	}
	t.Logf("PAIRED PROOF OK: paraphrase passes the citation check (the id is real) "+
		"but fails the verbatim check — %v", err)
}

// ---------------------------------------------------------------------------
// CHECK 5 — the margin test catches the near-miss the absolute floor cannot.
// ---------------------------------------------------------------------------

func TestRetrievalGate_MarginCatchesDiffuseMatch(t *testing.T) {
	// Every document scores high and near-identically: the question matched the
	// corpus diffusely. High absolute score, low margin — an unanswerable
	// question about a well-covered topic.
	diffuse := []retriever.Document{
		{ID: "a1", Content: "Docker is used for the build.", Score: 0.86},
		{ID: "a2", Content: "Docker images are cached.", Score: 0.85},
		{ID: "a3", Content: "Docker runs the test suite.", Score: 0.84},
	}
	req := Request{Documents: diffuse, MinScore: 0.50, MinMargin: 0.10}
	g := RetrievalGate(req)
	if g.Verdict != VerdictDeclined {
		t.Fatalf("verdict = %q, want declined (margin %.3f < 0.10)", g.Verdict, g.Margin)
	}
	// Without the margin test the absolute floor waves it straight through.
	loose := RetrievalGate(Request{Documents: diffuse, MinScore: 0.50, MinMargin: 0})
	if loose.Verdict != VerdictAnswered {
		t.Fatalf("absolute-only gate gave %q; expected it to admit the diffuse match", loose.Verdict)
	}
	t.Logf("PAIRED PROOF OK: top=%.3f margin=%.3f — the absolute floor admits it, "+
		"the margin test declines it", g.TopScore, g.Margin)
}
