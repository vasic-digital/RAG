package grounding

import (
	"context"
	"time"
)

// Extractive answers by returning retrieved documents verbatim, each with a
// citation to the document it came from. There is no model of any kind.
//
// WHY THIS IS THE DEFAULT WORKING PATH
//
// It cannot fabricate BY CONSTRUCTION. Every claim it emits is a copy of text
// that was already in the Request, and that property is enforced twice: once
// structurally, because the only string assignment in Answer copies
// Document.Content and no code path here synthesises text; and once by
// assertion, because ValidateVerbatim re-checks the finished Answer against
// the Request before it is returned. A correctness criterion satisfied
// structurally does not depend on prompt discipline, model size, temperature,
// or a vendor's behaviour on the day.
//
// It also has no infrastructure to fail. It works with zero generative
// capability, needs no daemon, opens no socket, and costs the time it takes to
// sort a slice. On a host with no generative model installed — which is the
// state of a fresh clone — it is the only adapter that can answer at all.
//
// HONEST BOUNDARY: this is not a synthesised answer. It returns the documents
// that answer the question, not prose that weaves them together. A consumer
// must present it as such and must not imply that more happened.
type Extractive struct {
	// MaxClaims caps the documents returned when a Request does not specify.
	MaxClaims int
}

// NewExtractive builds the extractive adapter. It cannot fail: there is
// nothing to connect to and nothing to validate.
func NewExtractive(maxClaims int) *Extractive {
	if maxClaims <= 0 {
		maxClaims = 3
	}
	return &Extractive{MaxClaims: maxClaims}
}

// ExtractiveName is the adapter name recorded on every Answer it produces.
const ExtractiveName = "extractive"

// Answer applies the L1 retrieval gate and, if it passes, returns the
// admitted documents verbatim as cited claims.
func (e *Extractive) Answer(ctx context.Context, req Request) (Answer, error) {
	start := time.Now()

	// Honour cancellation even though the work is fast: a caller's deadline is
	// the caller's to enforce, and silently ignoring it makes timeouts a lie.
	if err := ctx.Err(); err != nil {
		return UnavailableAnswer(ExtractiveName, "request cancelled before answering began"), nil
	}

	gate := RetrievalGate(req)
	switch gate.Verdict {
	case VerdictUnavailable:
		a := UnavailableAnswer(ExtractiveName, gate.Reason)
		a.Considered = gate.Considered
		a.ElapsedMS = time.Since(start).Milliseconds()
		return a, nil
	case VerdictDeclined:
		return Answer{
			Verdict:    VerdictDeclined,
			Reason:     gate.Reason,
			Considered: gate.Considered,
			Adapter:    ExtractiveName,
			// Vacuously true: there are no citations, so none is unverified.
			CitationsVerified: true,
			ElapsedMS:         time.Since(start).Milliseconds(),
		}, nil
	}

	limit := req.MaxClaims
	if limit <= 0 {
		limit = e.MaxClaims
	}

	claims := make([]Claim, 0, limit)
	for _, d := range gate.Admitted {
		if len(claims) >= limit {
			break
		}
		claims = append(claims, Claim{
			// The ONLY string assignment in this adapter, and it is a copy.
			// Nothing here concatenates, summarises, templates or rewrites.
			Text:      d.Content,
			Citations: []Citation{{DocumentID: d.ID, Source: d.Source}},
		})
	}

	if len(claims) == 0 {
		// Reachable only if the gate admitted documents and the limit was
		// somehow zero. Refusing is correct; returning an empty `answered`
		// would be an answer with no content presented as success.
		return Answer{
			Verdict:           VerdictDeclined,
			Reason:            "no document survived extraction",
			Considered:        gate.Considered,
			Adapter:           ExtractiveName,
			CitationsVerified: true,
			ElapsedMS:         time.Since(start).Milliseconds(),
		}, nil
	}

	// Post-conditions. These re-derive the guarantee from the finished Answer
	// and the original Request rather than trusting the loop above. If a
	// future change ever introduces synthesis here, this is what catches it —
	// at the boundary, before a caller sees anything.
	if err := ValidateCitations(req, claims); err != nil {
		return UnavailableAnswer(ExtractiveName,
			"internal grounding check failed: "+err.Error()), nil
	}
	if err := ValidateVerbatim(req, claims); err != nil {
		return UnavailableAnswer(ExtractiveName,
			"internal verbatim check failed: "+err.Error()), nil
	}

	return Answer{
		Verdict: VerdictAnswered,
		Claims:  claims,
		// True, and earned rather than assumed: every citation was checked
		// for set membership AND the text was checked to be the cited
		// document's own. There is no entailment gap to verify, because the
		// claim IS the evidence.
		CitationsVerified: true,
		Adapter:           ExtractiveName,
		ElapsedMS:         time.Since(start).Milliseconds(),
	}, nil
}
