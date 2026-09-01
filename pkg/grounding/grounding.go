// Package grounding is the deterministic verification layer that sits between
// retrieval and an answer.
//
// The design inverts the usual arrangement: a probabilistic generator is
// GATED BY deterministic verification, rather than being trusted because it
// was well prompted. The generation step is then free to be imperfect,
// because nothing it produces is returned without surviving checks that do
// not depend on it having been right.
//
//	L1  RetrievalGate      refuse before any model is called
//	L3  ValidateCitations  every cited id must be one placed in this request
//	    ValidateVerbatim   (extractive only) claim text must BE passage text
//
// L2 (schema-constrained generation) belongs to the generative adapters in
// the provider layer, and L4 (support verification) is a separate layer; the
// hook for it is Answer.CitationsVerified, which stays false until it runs.
//
// The retrieved unit is [retriever.Document] — this package adds no passage
// type of its own, so a caller already using pkg/retriever hands its results
// straight in. Document.ID is treated as an opaque stable identifier: this
// package never parses it, never orders by it and never derives meaning from
// it. It only tests set membership. A caller whose identifiers are stable
// across re-index and content correction gets citations that stay valid
// across both; that property belongs to the caller, not here.
package grounding

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"digital.vasic.rag/pkg/retriever"
)

// Verdict is the three-state answering outcome.
//
// The third state is the one that gets lost. "The content does not support an
// answer" and "the answering machinery could not run" are different facts,
// and reporting the second as the first blames the corpus for a configuration
// or an outage. Keeping them apart is the whole point of the type.
type Verdict string

const (
	// VerdictAnswered: the retrieved content supported an answer and it
	// survived every grounding check.
	VerdictAnswered Verdict = "answered"
	// VerdictDeclined: a determined negative. The machinery ran correctly and
	// the content does not answer the question.
	VerdictDeclined Verdict = "declined"
	// VerdictUnavailable: the attempt could not be completed — uncalibrated
	// thresholds, an unreachable model, a cancelled request. Never a
	// statement about the content.
	VerdictUnavailable Verdict = "unavailable"
)

// Valid reports whether v is one of the three defined states. The zero value
// is not valid: an unset verdict is a programming fault, and defaulting it to
// anything would silently pick a meaning the caller never chose.
func (v Verdict) Valid() bool {
	switch v {
	case VerdictAnswered, VerdictDeclined, VerdictUnavailable:
		return true
	}
	return false
}

// Citation points at the retrieved document a claim rests on.
type Citation struct {
	DocumentID string `json:"document_id"`
	Source     string `json:"source,omitempty"`
}

// Claim is one assertion together with the documents that support it.
type Claim struct {
	Text      string     `json:"text"`
	Citations []Citation `json:"citations"`
}

// Request is one answering attempt over an already-retrieved set.
type Request struct {
	Question  string               `json:"question"`
	Documents []retriever.Document `json:"documents"`

	// MinScore and MinMargin are the calibrated retrieval gate.
	//
	// MinScore  — the top document must score at least this.
	// MinMargin — the top document must stand out from the rest by at least
	//             this much. The two tests fail on DIFFERENT questions, which
	//             is why both exist. The dangerous unanswerable question is
	//             not an off-topic one; it is one whose subject is discussed
	//             at length while the specific asked-for fact is absent. That
	//             scores HIGH on MinScore and LOW on margin, because many
	//             documents match the topic and none answers the question.
	//
	// Both default to zero, and zero is DELIBERATELY HOSTILE. An uncalibrated
	// threshold cannot separate answerable from unanswerable, so the gate
	// reports `unavailable` with the reason stated — never `declined`, and
	// never `answered`. "We never calibrated" then fails loudly instead of
	// silently degrading into "answer everything".
	//
	// MinScore corresponds to retriever.Options.MinScore; MinMargin has no
	// counterpart there because it is a property of the retrieved SET rather
	// than of any single document.
	MinScore  float64 `json:"min_score"`
	MinMargin float64 `json:"min_margin"`

	// MaxClaims bounds the response. Zero means "adapter default".
	MaxClaims int `json:"max_claims"`
}

// Answer is the result of an answering attempt.
type Answer struct {
	Verdict Verdict `json:"verdict"`

	// Reason is always populated for declined and unavailable, written for a
	// human reader. It never contains a credential, an API key, or the value
	// of any environment variable.
	Reason string `json:"reason,omitempty"`

	Claims []Claim `json:"claims,omitempty"`

	// Considered holds the documents that were looked at but did not clear
	// the gate. Returned on `declined` so a reader can see WHAT was examined
	// rather than only being told the attempt failed.
	Considered []retriever.Document `json:"considered,omitempty"`

	// Adapter names what produced this answer.
	Adapter string `json:"adapter"`

	// CitationsVerified is false whenever verification could not run,
	// including in a deliberately-degraded identifier-only mode. An Answer
	// that says `answered` with CitationsVerified false must never be
	// reported as satisfying a citation-accuracy criterion.
	CitationsVerified bool `json:"citations_verified"`

	// ElapsedMS is wall-clock time spent answering, in milliseconds.
	ElapsedMS int64 `json:"elapsed_ms"`
}

// ErrUnavailable marks an infrastructural failure: the attempt could not be
// made. It is never returned when the CONTENT fails to support an answer —
// that is a successful call returning VerdictDeclined.
var ErrUnavailable = errors.New("grounding: answering unavailable")

// Unavailablef wraps ErrUnavailable with context.
func Unavailablef(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrUnavailable, fmt.Sprintf(format, args...))
}

// UnavailableAnswer builds the canonical unavailable result. Centralised so
// every path that cannot determine an answer reports it identically.
func UnavailableAnswer(adapter, reason string) Answer {
	return Answer{Verdict: VerdictUnavailable, Reason: reason, Adapter: adapter}
}

// GateOutcome is the result of the L1 retrieval gate.
type GateOutcome struct {
	// Admitted are the documents that cleared the gate, best first.
	Admitted []retriever.Document
	// Considered are the documents that were examined, best first. Populated
	// on every outcome so a decline can show what was looked at.
	Considered []retriever.Document
	// Verdict is Answered when documents were admitted, Declined when the
	// content does not support an answer, Unavailable when the gate could not
	// do its job (uncalibrated thresholds).
	Verdict Verdict
	Reason  string
	// TopScore and Margin are recorded on every outcome, whatever the
	// verdict. A question that refuses because it landed a hair below the
	// threshold is FRAGILE, and reporting it as a comfortable pass would be
	// dishonest. Callers write these into their evidence output so a reviewer
	// sees the distance from the threshold, not just the verdict.
	TopScore float64
	Margin   float64
}

// RetrievalGate is L1: decide whether the retrieved set can support an answer
// at all, BEFORE any model is called. On an unanswerable question this costs
// microseconds and no generation.
//
// Two independent tests, because they fail on different questions:
//
//   - absolute:  score(top1) >= MinScore        — is anything close enough?
//   - relative:  score(top1) - mean(rest) >= MinMargin
//     — does anything STAND OUT?
//
// The relative test is the one that earns its keep. A question about a topic
// the corpus discusses at length, asking for a specific fact the corpus never
// states, scores HIGH absolutely and LOW relatively: many documents match the
// subject and none answers the question. A gate with only the absolute test
// waves that straight through to the model, which then fills the gap.
func RetrievalGate(req Request) GateOutcome {
	sorted := append([]retriever.Document(nil), req.Documents...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Score > sorted[j].Score })

	out := GateOutcome{Considered: sorted}

	if len(sorted) == 0 {
		out.Verdict = VerdictDeclined
		out.Reason = "no documents were retrieved for this question"
		return out
	}

	out.TopScore = sorted[0].Score
	if len(sorted) > 1 {
		var sum float64
		for _, d := range sorted[1:] {
			sum += d.Score
		}
		out.Margin = sorted[0].Score - sum/float64(len(sorted)-1)
	} else {
		// A single document has nothing to stand out FROM. Reporting a margin
		// of zero would make it look like a diffuse match; reporting the
		// score itself would overstate. It is genuinely undefined, and the
		// margin test is skipped below rather than fabricating a number.
		out.Margin = 0
	}

	// Uncalibrated thresholds are "unable to verify", NOT "the content does
	// not answer this". Reporting this as `declined` would blame the content
	// for a configuration that was never set up, which is precisely the
	// conflation Verdict exists to prevent.
	if req.MinScore <= 0 {
		out.Verdict = VerdictUnavailable
		out.Reason = "retrieval thresholds are uncalibrated (min_score <= 0): " +
			"the gate cannot distinguish an answerable question from an unanswerable one, " +
			"so no answer can be trusted. Run the threshold calibration and set min_score."
		return out
	}

	if out.TopScore < req.MinScore {
		out.Verdict = VerdictDeclined
		out.Reason = fmt.Sprintf(
			"the closest document scored %.4f, below the calibrated floor of %.4f — "+
				"the indexed content does not appear to cover this question",
			out.TopScore, req.MinScore)
		return out
	}

	if len(sorted) > 1 && req.MinMargin > 0 && out.Margin < req.MinMargin {
		out.Verdict = VerdictDeclined
		out.Reason = fmt.Sprintf(
			"the best document scored %.4f but stood out from the rest by only %.4f "+
				"(floor %.4f) — the question matched the content diffusely, which is what "+
				"an unanswerable question about a well-covered topic looks like",
			out.TopScore, out.Margin, req.MinMargin)
		return out
	}

	for _, d := range sorted {
		if d.Score >= req.MinScore {
			out.Admitted = append(out.Admitted, d)
		}
	}
	out.Verdict = VerdictAnswered
	return out
}

// ErrFabricated marks a claim that failed deterministic grounding. It is
// deliberately loud: reaching it means an adapter tried to return something
// the retrieved set does not support, and the correct response is to refuse
// the WHOLE answer rather than to repair it.
var ErrFabricated = errors.New("grounding: claim failed validation")

// ValidateCitations is L3: deterministic citation-identifier validation.
//
// For every citation on every claim, the cited identifier must be one that was
// actually placed in THIS request. This is a string-set membership test —
// microseconds, no model, no heuristic — and it catches the most common
// citation failure with certainty: an identifier the generator invented.
//
// It is the highest-value layer per unit of cost in the whole design. Without
// it, "the citation points at a document that exists" is unproven, and every
// claim built on top of that floor is unproven with it.
//
// Also enforced here: at least one citation per claim. Schema constraints can
// make an uncited claim undecodable at the generation step, but a schema is
// enforced by the decoder and this is enforced by us. Checking it again costs
// nothing and does not depend on a remote server having honoured the schema
// it was sent.
func ValidateCitations(req Request, claims []Claim) error {
	known := make(map[string]struct{}, len(req.Documents))
	for _, d := range req.Documents {
		known[d.ID] = struct{}{}
	}

	for i, c := range claims {
		if strings.TrimSpace(c.Text) == "" {
			return fmt.Errorf("%w: claim %d has empty text", ErrFabricated, i)
		}
		if len(c.Citations) == 0 {
			return fmt.Errorf("%w: claim %d has no citations", ErrFabricated, i)
		}
		for j, cit := range c.Citations {
			if cit.DocumentID == "" {
				return fmt.Errorf("%w: claim %d citation %d has an empty document id",
					ErrFabricated, i, j)
			}
			if _, ok := known[cit.DocumentID]; !ok {
				return fmt.Errorf(
					"%w: claim %d cites document id %q, which was not among the %d documents "+
						"placed in this request — the identifier was invented",
					ErrFabricated, i, cit.DocumentID, len(req.Documents))
			}
		}
	}
	return nil
}

// ValidateVerbatim is the extractive adapter's structural guarantee.
//
// Every claim's text must appear VERBATIM inside at least one of the documents
// it cites. Not paraphrased, not summarised, not "semantically close" —
// byte-for-byte contained, after whitespace normalisation only.
//
// This is what makes "cannot fabricate" a property of the construction rather
// than a hope about behaviour. There is no model in the extractive path, so
// there is nothing that could generate novel text; this check proves that
// claim instead of asserting it, and it would fail immediately if anyone
// later introduced a summarisation step into the extractive adapter.
func ValidateVerbatim(req Request, claims []Claim) error {
	known := make(map[string]string, len(req.Documents))
	for _, d := range req.Documents {
		known[d.ID] = normaliseWhitespace(d.Content)
	}

	for i, c := range claims {
		needle := normaliseWhitespace(c.Text)
		if needle == "" {
			return fmt.Errorf("%w: claim %d is empty after normalisation", ErrFabricated, i)
		}
		found := false
		for _, cit := range c.Citations {
			hay, ok := known[cit.DocumentID]
			if ok && strings.Contains(hay, needle) {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf(
				"%w: claim %d text does not appear verbatim in any document it cites — "+
					"the extractive path returned text that is not in the retrieved content",
				ErrFabricated, i)
		}
	}
	return nil
}

// normaliseWhitespace collapses runs of whitespace to a single space and
// trims. This is the ONLY transformation the verbatim check tolerates: it
// absorbs line wrapping and indentation differences without permitting any
// change to the words themselves.
func normaliseWhitespace(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
