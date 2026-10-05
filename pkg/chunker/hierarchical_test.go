package chunker

import (
	"strconv"
	"strings"
	"testing"
)

// TestHierarchicalChunkerProducesChildrenWithParentContext is the core T031
// claim: given a long structured document, child chunks are produced at the
// configured (child) size AND each one carries a reference to its parent
// section's larger context.
func TestHierarchicalChunkerProducesChildrenWithParentContext(t *testing.T) {
	// Two parent sections, each far larger than the child chunk size, so each
	// parent is guaranteed to split into multiple children.
	sectionA := strings.Repeat("alpha ", 100) // 600 bytes
	sectionB := strings.Repeat("beta ", 100)  // 500 bytes
	text := sectionA + "\n\n" + sectionB

	cfg := HierarchicalConfig{
		Config:          Config{ChunkSize: 150, Overlap: 20},
		ParentChunkSize: 700,
	}
	c := NewHierarchicalChunker(cfg)
	chunks := c.Chunk(text)

	if len(chunks) < 4 {
		t.Fatalf("want at least 4 children (2 parents x >=2 children each), got %d", len(chunks))
	}

	for i, ch := range chunks {
		if len(ch.Content) == 0 {
			t.Fatalf("chunk %d is empty", i)
		}
		if len(ch.Content) > cfg.ChunkSize {
			t.Fatalf("chunk %d exceeds configured child size: %d > %d", i, len(ch.Content), cfg.ChunkSize)
		}
		parentContent, ok := ch.Metadata["parent_content"].(string)
		if !ok || parentContent == "" {
			t.Fatalf("chunk %d carries no parent_content in Metadata: %+v", i, ch.Metadata)
		}
		if !strings.Contains(parentContent, strings.TrimSpace(ch.Content)) {
			t.Fatalf("chunk %d's own content is not found inside its claimed parent_content", i)
		}
		if _, ok := ch.Metadata["parent_index"]; !ok {
			t.Fatalf("chunk %d carries no parent_index in Metadata", i)
		}
	}

	// Children of the FIRST parent must all share the SAME parent_content,
	// and it must be (close to) sectionA — proving they were grouped under
	// their actual containing section, not just numbered arbitrarily.
	firstParent := chunks[0].Metadata["parent_content"].(string)
	for _, ch := range chunks {
		if ch.Metadata["parent_index"] == chunks[0].Metadata["parent_index"] {
			if ch.Metadata["parent_content"].(string) != firstParent {
				t.Fatalf("two children with the same parent_index carry DIFFERENT parent_content")
			}
		}
	}
	if !strings.Contains(firstParent, "alpha") || strings.Contains(firstParent, "beta") {
		t.Fatalf("first parent's content does not correspond to sectionA alone: %q", firstParent)
	}
}

// TestHierarchicalChunkerSiblingOverlapFollowsGetOverlapTextPattern is the
// overlap half of T031: overlap between SIBLING child chunks (same parent)
// must be REAL — the trailing bytes of one child reappear at the start of
// the next, exactly as SentenceChunker's own getOverlapText produces,
// unlike RecursiveChunker's documented-broken mergeAndOverlap (which adds
// no overlap at all despite a non-zero Config.Overlap).
func TestHierarchicalChunkerSiblingOverlapFollowsGetOverlapTextPattern(t *testing.T) {
	text := strings.Repeat("word ", 200) // 1000 bytes, one parent section
	cfg := HierarchicalConfig{
		Config:          Config{ChunkSize: 100, Overlap: 30},
		ParentChunkSize: 2000,
	}
	c := NewHierarchicalChunker(cfg)
	chunks := c.Chunk(text)
	if len(chunks) < 2 {
		t.Fatalf("want at least 2 sibling children to compare overlap, got %d", len(chunks))
	}
	for i := 1; i < len(chunks); i++ {
		prev, cur := chunks[i-1].Content, chunks[i].Content
		overlapWant := getOverlapText(prev, cfg.Overlap)
		if !strings.HasPrefix(cur, overlapWant) {
			t.Fatalf("child %d does not start with child %d's trailing %d-byte overlap "+
				"(getOverlapText pattern): want prefix %q, got %q", i, i-1, cfg.Overlap,
				overlapWant, cur[:min(len(cur), len(overlapWant)+5)])
		}
	}
}

// TestHierarchicalChunkerNoOverlapWhenConfigured proves overlap is genuinely
// configurable (Overlap=0 produces no shared bytes between siblings), so the
// overlap above is a real, controllable behavior and not an accidental
// side effect of the splitting itself.
func TestHierarchicalChunkerNoOverlapWhenConfigured(t *testing.T) {
	// A NON-repeating sequence (word0, word1, word2, ...) so a boundary match
	// can only happen via real overlap, never by coincidence of a repeated
	// pattern (which a plain "word word word..." fixture would risk).
	words := make([]string, 200)
	for i := range words {
		words[i] = "word" + strconv.Itoa(i)
	}
	text := strings.Join(words, " ")
	cfg := HierarchicalConfig{
		Config:          Config{ChunkSize: 100, Overlap: 0},
		ParentChunkSize: 2000,
	}
	c := NewHierarchicalChunker(cfg)
	chunks := c.Chunk(text)
	if len(chunks) < 2 {
		t.Fatalf("want at least 2 children, got %d", len(chunks))
	}
	for i := 1; i < len(chunks); i++ {
		boundary := chunks[i-1].Content[len(chunks[i-1].Content)-5:]
		if strings.HasPrefix(chunks[i].Content, boundary) {
			t.Fatalf("child %d starts with child %d's own trailing bytes (%q) despite Overlap=0 — "+
				"real overlap leaked in with no overlap configured", i, i-1, boundary)
		}
	}
}

// TestHierarchicalChunkerShortTextIsOneChunkOneParent is the degenerate case:
// text shorter than both the parent and child size is returned as a single
// chunk, carrying itself as its own parent context.
func TestHierarchicalChunkerShortTextIsOneChunkOneParent(t *testing.T) {
	c := NewHierarchicalChunker(DefaultHierarchicalConfig())
	chunks := c.Chunk("a short passage")
	if len(chunks) != 1 {
		t.Fatalf("want exactly 1 chunk for short text, got %d", len(chunks))
	}
	if chunks[0].Content != "a short passage" {
		t.Fatalf("content mismatch: %q", chunks[0].Content)
	}
	if chunks[0].Metadata["parent_content"].(string) != "a short passage" {
		t.Fatalf("a single-chunk document's own parent_content must be itself")
	}
}

// TestHierarchicalChunkerEmptyTextReturnsNoChunks mirrors every other
// chunker's own empty-input contract in this package.
func TestHierarchicalChunkerEmptyTextReturnsNoChunks(t *testing.T) {
	c := NewHierarchicalChunker(DefaultHierarchicalConfig())
	if chunks := c.Chunk(""); chunks != nil {
		t.Fatalf("want nil for empty text, got %v", chunks)
	}
}

// TestHierarchicalChunkerImplementsChunkerInterface is a compile-time-ish
// assertion, checked at runtime so a signature drift fails a test rather
// than every caller's build.
func TestHierarchicalChunkerImplementsChunkerInterface(t *testing.T) {
	var _ Chunker = NewHierarchicalChunker(DefaultHierarchicalConfig())
}
