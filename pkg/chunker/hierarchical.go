package chunker

import "unicode/utf8"

// HierarchicalConfig configures HierarchicalChunker. The embedded Config's
// ChunkSize/Overlap/Separators govern the CHILD level — the chunks actually
// returned and embedded. ParentChunkSize governs the larger PARENT section
// each child's Metadata carries as its surrounding context; it is always
// larger than the child ChunkSize (enforced in NewHierarchicalChunker).
type HierarchicalConfig struct {
	Config
	ParentChunkSize int
}

// DefaultHierarchicalConfig returns a default configuration: the standard
// child-level DefaultConfig, with parent sections four times that size.
func DefaultHierarchicalConfig() HierarchicalConfig {
	return HierarchicalConfig{
		Config:          DefaultConfig(),
		ParentChunkSize: DefaultConfig().ChunkSize * 4,
	}
}

// HierarchicalChunker splits a long document into PARENT sections first
// (using RecursiveChunker's own separator-based splitting, at
// ParentChunkSize, with NO parent-to-parent overlap — parents are
// non-overlapping containers, not retrieval units themselves), then splits
// each parent into CHILD chunks at the configured Config.ChunkSize.
//
// Every child's Metadata carries its parent's full text ("parent_content"),
// the parent's byte offsets within the original document
// ("parent_start"/"parent_end"), and which parent it belongs to
// ("parent_index") — the "reference to its parent section's larger context"
// a consumer needs to expand a child hit back out to its containing section.
//
// SIBLING OVERLAP (children of the SAME parent) follows SentenceChunker's
// own proven getOverlapText pattern: the trailing Config.Overlap bytes of one
// child seed the start of the next. This is a deliberate departure from
// RecursiveChunker's own mergeAndOverlap, which this package's own tests
// already document as NOT implementing the overlap its Config.Overlap field
// promises (chunker.go: "Merge small chunks and apply overlap" — it merges,
// it never applies). RecursiveChunker is still used here, but only for the
// PARENT split, and only with Overlap=0 — a value for which that defect is
// unobservable (there is nothing to add), so parent splitting never carries
// the bug forward into this chunker's own output.
//
// PARENTS NEVER OVERLAP EACH OTHER. Only children sharing the same parent do.
// A parent boundary is a real section boundary (the configured separators,
// e.g. "\n\n"); repeating content across it would duplicate whole sections,
// not smooth a mid-sentence cut the way child-level overlap does.
type HierarchicalChunker struct {
	config         HierarchicalConfig
	parentSplitter *RecursiveChunker
}

// NewHierarchicalChunker creates a HierarchicalChunker. Invalid child-level
// fields are normalized exactly as NewRecursiveChunker normalizes them (same
// defaults, same reasoning), so the two chunkers never disagree about what an
// invalid Config means. ParentChunkSize is raised to 4x the (normalized)
// ChunkSize whenever it is not already larger — a parent that could not even
// hold more than one child would defeat the whole point of this chunker.
func NewHierarchicalChunker(config HierarchicalConfig) *HierarchicalChunker {
	if config.ChunkSize <= 0 {
		config.ChunkSize = 1000
	}
	if config.Overlap < 0 {
		config.Overlap = 0
	}
	if config.Overlap >= config.ChunkSize {
		config.Overlap = config.ChunkSize / 4
	}
	if len(config.Separators) == 0 {
		config.Separators = []string{"\n\n", "\n", ". ", " "}
	}
	if config.ParentChunkSize <= config.ChunkSize {
		config.ParentChunkSize = config.ChunkSize * 4
	}
	parentSplitter := NewRecursiveChunker(Config{
		ChunkSize:  config.ParentChunkSize,
		Overlap:    0,
		Separators: config.Separators,
	})
	return &HierarchicalChunker{config: config, parentSplitter: parentSplitter}
}

var _ Chunker = (*HierarchicalChunker)(nil)

// Chunk implements Chunker.
func (c *HierarchicalChunker) Chunk(text string) []Chunk {
	if text == "" {
		return nil
	}

	parents := c.parentSplitter.Chunk(text)
	var out []Chunk
	for parentIdx, parent := range parents {
		for _, child := range c.chunkChildren(parent.Content) {
			out = append(out, Chunk{
				Content: child.content,
				Start:   parent.Start + child.start,
				End:     parent.Start + child.end,
				Metadata: map[string]any{
					"parent_index":   parentIdx,
					"parent_content": parent.Content,
					"parent_start":   parent.Start,
					"parent_end":     parent.End,
				},
			})
		}
	}
	return out
}

// childSpan is a child chunk's content and its byte offsets WITHIN ITS OWN
// PARENT's text (not the original document — Chunk translates these to
// document-absolute offsets using the parent's own Start).
type childSpan struct {
	content    string
	start, end int
}

// chunkChildren splits one parent's text into children at Config.ChunkSize,
// sliding forward by (ChunkSize - Overlap) each step — the same step
// FixedSizeChunker uses, which for a contiguous byte window is exactly
// equivalent to seeding each step with the previous child's trailing
// Overlap bytes (getOverlapText's own definition).
//
// EVERY start AND end IS SNAPPED TO A RUNE BOUNDARY (runeBoundaryAtOrBefore).
// ChunkSize/Overlap are byte counts, and a raw text[start:end] slice at an
// arbitrary byte offset can fall inside a multi-byte UTF-8 rune, producing
// invalid UTF-8 content in BOTH chunks straddling that cut — measured
// directly: a non-ASCII fixture produced mid-rune garbage bytes in the
// majority of its chunks before this snapping existed. Both start and end
// need the adjustment, not just end: start in a later iteration is derived
// from an earlier end, so an unadjusted end would carry the misalignment
// forward into every subsequent child. The snap always rounds DOWN (never
// splits a rune, and never widens past the configured size), so Content is
// never longer than ChunkSize; it can be up to (rune width - 1) bytes
// shorter at a boundary that landed mid-rune, which is the only size
// deviation this introduces.
func (c *HierarchicalChunker) chunkChildren(parentText string) []childSpan {
	if len(parentText) <= c.config.ChunkSize {
		return []childSpan{{content: parentText, start: 0, end: len(parentText)}}
	}

	overlap := c.config.Overlap

	var out []childSpan
	start := 0
	for start < len(parentText) {
		end := start + c.config.ChunkSize
		if end >= len(parentText) {
			end = len(parentText)
		} else {
			end = runeBoundaryAtOrBefore(parentText, end)
		}
		out = append(out, childSpan{content: parentText[start:end], start: start, end: end})
		if end == len(parentText) {
			break
		}

		next := runeBoundaryAtOrBefore(parentText, end-overlap)
		if next <= start {
			// The rune-boundary snap (or a tiny ChunkSize relative to
			// Overlap) left no forward progress. Advance by exactly one
			// whole rune instead of looping forever at the same start.
			_, size := utf8.DecodeRuneInString(parentText[start:])
			next = start + size
		}
		start = next
	}
	return out
}

// runeBoundaryAtOrBefore returns the largest index <= pos that sits on a
// UTF-8 rune boundary in s — i.e. never inside the continuation bytes of a
// multi-byte rune. pos == len(s) (one past the last byte) is itself always
// a valid boundary and is returned unchanged.
func runeBoundaryAtOrBefore(s string, pos int) int {
	if pos >= len(s) {
		return len(s)
	}
	for pos > 0 && !utf8.RuneStart(s[pos]) {
		pos--
	}
	return pos
}
