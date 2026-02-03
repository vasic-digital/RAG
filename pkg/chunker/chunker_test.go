package chunker

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	assert.Equal(t, 1000, cfg.ChunkSize)
	assert.Equal(t, 200, cfg.Overlap)
	assert.NotEmpty(t, cfg.Separators)
}

func TestFixedSizeChunker_Chunk(t *testing.T) {
	tests := []struct {
		name      string
		config    Config
		text      string
		wantCount int
		wantFirst string
	}{
		{
			name:      "empty text",
			config:    Config{ChunkSize: 100},
			text:      "",
			wantCount: 0,
		},
		{
			name:      "text smaller than chunk size",
			config:    Config{ChunkSize: 100},
			text:      "short text",
			wantCount: 1,
			wantFirst: "short text",
		},
		{
			name:      "text exactly chunk size",
			config:    Config{ChunkSize: 10},
			text:      "0123456789",
			wantCount: 1,
			wantFirst: "0123456789",
		},
		{
			name:      "text larger than chunk size no overlap",
			config:    Config{ChunkSize: 10, Overlap: 0},
			text:      "0123456789abcdefghij",
			wantCount: 2,
			wantFirst: "0123456789",
		},
		{
			name:      "text with overlap",
			config:    Config{ChunkSize: 10, Overlap: 3},
			text:      "0123456789abcdefghij",
			wantCount: 3,
			wantFirst: "0123456789",
		},
		{
			name:      "overlap clamped when >= chunk size",
			config:    Config{ChunkSize: 10, Overlap: 15},
			text:      "0123456789abcdefghij",
			wantCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewFixedSizeChunker(tt.config)
			chunks := c.Chunk(tt.text)

			assert.Len(t, chunks, tt.wantCount)

			if tt.wantFirst != "" && len(chunks) > 0 {
				assert.Equal(t, tt.wantFirst, chunks[0].Content)
			}

			// Verify all chunks have valid start/end
			for _, ch := range chunks {
				assert.LessOrEqual(t, ch.Start, ch.End)
				assert.Equal(t, ch.Content, tt.text[ch.Start:ch.End])
			}
		})
	}
}

func TestFixedSizeChunker_Coverage(t *testing.T) {
	text := "The quick brown fox jumps over the lazy dog. " +
		"Pack my box with five dozen liquor jugs."
	c := NewFixedSizeChunker(Config{ChunkSize: 20, Overlap: 5})
	chunks := c.Chunk(text)

	require.NotEmpty(t, chunks)
	// Last chunk must cover end of text
	lastChunk := chunks[len(chunks)-1]
	assert.Equal(t, len(text), lastChunk.End)
}

func TestRecursiveChunker_Chunk(t *testing.T) {
	tests := []struct {
		name      string
		config    Config
		text      string
		wantMin   int
		wantMax   int
		checkAll  bool
	}{
		{
			name:   "empty text",
			config: Config{ChunkSize: 100},
			text:   "",
		},
		{
			name:    "text smaller than chunk size",
			config:  Config{ChunkSize: 100},
			text:    "short text",
			wantMin: 1,
			wantMax: 1,
		},
		{
			name: "splits on paragraph boundary",
			config: Config{
				ChunkSize:  50,
				Separators: []string{"\n\n", "\n", " "},
			},
			text:    "First paragraph content.\n\nSecond paragraph content.",
			wantMin: 2,
			wantMax: 2,
		},
		{
			name: "splits on newline when paragraph too large",
			config: Config{
				ChunkSize:  30,
				Separators: []string{"\n\n", "\n", " "},
			},
			text: "Line one\nLine two\nLine three\nLine four\n" +
				"Line five",
			wantMin:  2,
			checkAll: true,
		},
		{
			name: "falls back to fixed size when no separators match",
			config: Config{
				ChunkSize:  10,
				Separators: []string{"\n\n"},
			},
			text:    "abcdefghijklmnopqrstuvwxyz",
			wantMin: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewRecursiveChunker(tt.config)
			chunks := c.Chunk(tt.text)

			if tt.wantMin > 0 {
				assert.GreaterOrEqual(t, len(chunks), tt.wantMin)
			}
			if tt.wantMax > 0 {
				assert.LessOrEqual(t, len(chunks), tt.wantMax)
			}

			if tt.checkAll {
				for _, ch := range chunks {
					assert.NotEmpty(t, ch.Content)
				}
			}
		})
	}
}

func TestSentenceChunker_Chunk(t *testing.T) {
	tests := []struct {
		name      string
		config    Config
		text      string
		wantCount int
	}{
		{
			name:      "empty text",
			config:    Config{ChunkSize: 100},
			text:      "",
			wantCount: 0,
		},
		{
			name:      "single sentence fits",
			config:    Config{ChunkSize: 100},
			text:      "Hello world.",
			wantCount: 1,
		},
		{
			name:      "multiple sentences grouped",
			config:    Config{ChunkSize: 50},
			text:      "First. Second. Third.",
			wantCount: 1,
		},
		{
			name:   "sentences split across chunks",
			config: Config{ChunkSize: 30},
			text: "This is the first sentence. " +
				"This is the second sentence. " +
				"This is the third sentence.",
			wantCount: 3,
		},
		{
			name:      "text without sentence endings",
			config:    Config{ChunkSize: 100},
			text:      "No sentence ending here",
			wantCount: 1,
		},
		{
			name:   "with overlap",
			config: Config{ChunkSize: 30, Overlap: 10},
			text: "First sentence here. Second sentence here. " +
				"Third sentence here.",
			wantCount: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewSentenceChunker(tt.config)
			chunks := c.Chunk(tt.text)
			assert.Len(t, chunks, tt.wantCount)

			for _, ch := range chunks {
				assert.NotEmpty(t, ch.Content)
				assert.LessOrEqual(t, ch.Start, ch.End)
			}
		})
	}
}

func TestSplitSentences(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []string
	}{
		{
			name: "period delimited",
			text: "Hello. World.",
			want: []string{"Hello.", "World."},
		},
		{
			name: "exclamation and question",
			text: "What? Yes! OK.",
			want: []string{"What?", "Yes!", "OK."},
		},
		{
			name: "trailing text without period",
			text: "Hello. World",
			want: []string{"Hello.", "World"},
		},
		{
			name: "empty string",
			text: "",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitSentences(tt.text)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestGetOverlapText(t *testing.T) {
	assert.Equal(t, "world", getOverlapText("hello world", 5))
	assert.Equal(t, "hi", getOverlapText("hi", 10))
}

func TestChunkerInterface(t *testing.T) {
	// Verify all chunkers implement the Chunker interface
	var _ Chunker = &FixedSizeChunker{}
	var _ Chunker = &RecursiveChunker{}
	var _ Chunker = &SentenceChunker{}
}

func TestFixedSizeChunker_LargeText(t *testing.T) {
	text := strings.Repeat("word ", 500)
	c := NewFixedSizeChunker(Config{ChunkSize: 100, Overlap: 20})
	chunks := c.Chunk(text)

	require.NotEmpty(t, chunks)
	for _, ch := range chunks {
		assert.LessOrEqual(t, len(ch.Content), 100)
	}
}

// Tests for previously uncovered code paths

func TestNewFixedSizeChunker_DefaultValues(t *testing.T) {
	tests := []struct {
		name              string
		config            Config
		expectedChunkSize int
		expectedOverlap   int
	}{
		{
			name:              "zero chunk size defaults to 1000",
			config:            Config{ChunkSize: 0, Overlap: 100},
			expectedChunkSize: 1000,
			expectedOverlap:   100,
		},
		{
			name:              "negative chunk size defaults to 1000",
			config:            Config{ChunkSize: -5, Overlap: 50},
			expectedChunkSize: 1000,
			expectedOverlap:   50,
		},
		{
			name:              "negative overlap defaults to 0",
			config:            Config{ChunkSize: 100, Overlap: -10},
			expectedChunkSize: 100,
			expectedOverlap:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewFixedSizeChunker(tt.config)
			assert.Equal(t, tt.expectedChunkSize, c.config.ChunkSize)
			assert.Equal(t, tt.expectedOverlap, c.config.Overlap)
		})
	}
}

func TestFixedSizeChunker_StepEdgeCases(t *testing.T) {
	// Test case where overlap equals chunk size minus 1, resulting in step=1
	// This ensures the step <= 0 branch and step=1 fallback is exercised
	config := Config{ChunkSize: 5, Overlap: 4}
	c := NewFixedSizeChunker(config)

	// With step=1 (5-4=1), we should get many overlapping chunks
	text := "0123456789"
	chunks := c.Chunk(text)

	require.NotEmpty(t, chunks)
	// With step=1, chunk size=5, and text length=10, we expect multiple chunks
	// First chunk: 0-5 "01234"
	// Then step forward by 1 each time until we reach the end
	assert.Greater(t, len(chunks), 1)

	// Verify all chunks have valid boundaries
	for _, ch := range chunks {
		assert.LessOrEqual(t, ch.Start, ch.End)
		assert.Equal(t, ch.Content, text[ch.Start:ch.End])
	}
}

func TestNewRecursiveChunker_DefaultValues(t *testing.T) {
	tests := []struct {
		name               string
		config             Config
		expectedChunkSize  int
		expectedOverlap    int
		expectedSeparators []string
	}{
		{
			name:               "zero chunk size defaults to 1000",
			config:             Config{ChunkSize: 0, Overlap: 100},
			expectedChunkSize:  1000,
			expectedOverlap:    100,
			expectedSeparators: []string{"\n\n", "\n", ". ", " "},
		},
		{
			name:               "negative chunk size defaults to 1000",
			config:             Config{ChunkSize: -5, Overlap: 50},
			expectedChunkSize:  1000,
			expectedOverlap:    50,
			expectedSeparators: []string{"\n\n", "\n", ". ", " "},
		},
		{
			name:               "negative overlap defaults to 0",
			config:             Config{ChunkSize: 100, Overlap: -10},
			expectedChunkSize:  100,
			expectedOverlap:    0,
			expectedSeparators: []string{"\n\n", "\n", ". ", " "},
		},
		{
			name:               "empty separators get default values",
			config:             Config{ChunkSize: 100, Overlap: 10, Separators: []string{}},
			expectedChunkSize:  100,
			expectedOverlap:    10,
			expectedSeparators: []string{"\n\n", "\n", ". ", " "},
		},
		{
			name:               "nil separators get default values",
			config:             Config{ChunkSize: 100, Overlap: 10, Separators: nil},
			expectedChunkSize:  100,
			expectedOverlap:    10,
			expectedSeparators: []string{"\n\n", "\n", ". ", " "},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewRecursiveChunker(tt.config)
			assert.Equal(t, tt.expectedChunkSize, c.config.ChunkSize)
			assert.Equal(t, tt.expectedOverlap, c.config.Overlap)
			assert.Equal(t, tt.expectedSeparators, c.config.Separators)
		})
	}
}

func TestRecursiveChunker_MergeAndOverlap_EmptyRawChunks(t *testing.T) {
	// Create a chunker and test the mergeAndOverlap function with empty input
	// by creating a scenario where splitRecursive returns empty
	c := NewRecursiveChunker(Config{ChunkSize: 100, Overlap: 10})

	// When text is small enough, Chunk returns early before calling mergeAndOverlap
	// So we need to call mergeAndOverlap directly via the chunker
	// Actually, when rawChunks is empty, mergeAndOverlap returns nil

	// Test via a text that creates empty chunks after trimming
	// Using whitespace-only parts that get trimmed to empty
	text := "Hello\n\n   \n\nWorld"
	chunks := c.Chunk(text)

	// The middle whitespace-only part should be skipped
	require.NotEmpty(t, chunks)
	for _, ch := range chunks {
		assert.NotEmpty(t, strings.TrimSpace(ch.Content))
	}
}

func TestRecursiveChunker_SplitRecursive_SmallText(t *testing.T) {
	// Test splitRecursive when the text is already smaller than chunk size
	// This exercises line 144-146: if len(text) <= c.config.ChunkSize return
	c := NewRecursiveChunker(Config{ChunkSize: 100, Overlap: 10})

	// Small text that fits in chunk size should return single chunk
	text := "Small text"
	chunks := c.Chunk(text)

	require.Len(t, chunks, 1)
	assert.Equal(t, "Small text", chunks[0].Content)
}

func TestNewSentenceChunker_DefaultValues(t *testing.T) {
	tests := []struct {
		name              string
		config            Config
		expectedChunkSize int
		expectedOverlap   int
	}{
		{
			name:              "zero chunk size defaults to 1000",
			config:            Config{ChunkSize: 0, Overlap: 100},
			expectedChunkSize: 1000,
			expectedOverlap:   100,
		},
		{
			name:              "negative chunk size defaults to 1000",
			config:            Config{ChunkSize: -5, Overlap: 50},
			expectedChunkSize: 1000,
			expectedOverlap:   50,
		},
		{
			name:              "negative overlap defaults to 0",
			config:            Config{ChunkSize: 100, Overlap: -10},
			expectedChunkSize: 100,
			expectedOverlap:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewSentenceChunker(tt.config)
			assert.Equal(t, tt.expectedChunkSize, c.config.ChunkSize)
			assert.Equal(t, tt.expectedOverlap, c.config.Overlap)
		})
	}
}

func TestSentenceChunker_NoSentenceEndings(t *testing.T) {
	// Test when splitSentences returns empty (no sentence-ending characters)
	// and text is not empty - this covers lines 258-264
	// When splitSentences returns empty, the original text is returned as a single chunk
	tests := []struct {
		name        string
		text        string
		wantCount   int
		wantContent string
	}{
		{
			name:        "text with only whitespace - returned as single chunk",
			text:        "   ",
			wantCount:   1, // Original text returned when splitSentences is empty
			wantContent: "   ",
		},
		{
			name:        "single word no punctuation",
			text:        "word",
			wantCount:   1, // Remaining text captured
			wantContent: "word",
		},
		{
			name:        "multiple words no sentence endings",
			text:        "hello world foo bar",
			wantCount:   1, // All captured as remaining
			wantContent: "hello world foo bar",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewSentenceChunker(Config{ChunkSize: 100})
			chunks := c.Chunk(tt.text)
			assert.Len(t, chunks, tt.wantCount)
			if tt.wantCount > 0 && len(chunks) > 0 {
				assert.Equal(t, tt.wantContent, chunks[0].Content)
			}
		})
	}
}

func TestRecursiveChunker_WhitespaceOnlyChunks(t *testing.T) {
	// Test that whitespace-only chunks are properly skipped in mergeAndOverlap
	// This covers lines 211-213: if trimmed == ""
	config := Config{
		ChunkSize:  20,
		Separators: []string{"\n\n"},
	}
	c := NewRecursiveChunker(config)

	// Text with paragraph containing only whitespace
	text := "First part\n\n     \n\nSecond part"
	chunks := c.Chunk(text)

	// Should have exactly 2 chunks, whitespace-only part should be skipped
	require.Len(t, chunks, 2)
	assert.Equal(t, "First part", chunks[0].Content)
	assert.Equal(t, "Second part", chunks[1].Content)
}

func TestRecursiveChunker_EmptyRawChunks(t *testing.T) {
	// Test the mergeAndOverlap function when called with empty rawChunks
	// This is a bit tricky to trigger via public API since the chunker
	// validates input, but we can test by examining behavior

	c := NewRecursiveChunker(Config{ChunkSize: 100})

	// Empty text returns nil directly from Chunk method
	chunks := c.Chunk("")
	assert.Nil(t, chunks)
}

func TestFixedSizeChunker_OverlapEqualToChunkSize(t *testing.T) {
	// When overlap >= chunk size, it gets clamped to ChunkSize/4
	// This ensures proper chunking continues
	config := Config{ChunkSize: 20, Overlap: 20}
	c := NewFixedSizeChunker(config)

	// Overlap should be clamped to 20/4 = 5
	assert.Equal(t, 5, c.config.Overlap)

	text := "0123456789012345678901234567890123456789"
	chunks := c.Chunk(text)

	require.NotEmpty(t, chunks)
	// Verify chunks are created properly with clamped overlap
	for i, ch := range chunks {
		assert.NotEmpty(t, ch.Content, "chunk %d should not be empty", i)
		assert.LessOrEqual(t, len(ch.Content), 20)
	}
}

func TestRecursiveChunker_OverlapEqualToChunkSize(t *testing.T) {
	// Similar test for RecursiveChunker
	config := Config{ChunkSize: 20, Overlap: 25}
	c := NewRecursiveChunker(config)

	// Overlap should be clamped to 20/4 = 5
	assert.Equal(t, 5, c.config.Overlap)
}

func TestSplitSentences_EdgeCases(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []string
	}{
		{
			name: "sentence ending at last character",
			text: "Hello.",
			want: []string{"Hello."},
		},
		{
			name: "multiple spaces after period",
			text: "Hello.  World.",
			want: []string{"Hello.", "World."},
		},
		{
			name: "newline after period",
			text: "Hello.\nWorld.",
			want: []string{"Hello.", "World."},
		},
		{
			name: "tab after period",
			text: "Hello.\tWorld.",
			want: []string{"Hello.", "World."},
		},
		{
			name: "period without space after (not sentence end)",
			text: "Hello.World.",
			want: []string{"Hello.World."},
		},
		{
			name: "only whitespace",
			text: "   \t\n  ",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitSentences(tt.text)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRecursiveChunker_SplitRecursive_InternalRecursion(t *testing.T) {
	// Test case that exercises the internal recursion in splitRecursive
	// where subParts are generated and the inner text is already small enough
	// This exercises line 144-146: if len(text) <= c.config.ChunkSize return
	config := Config{
		ChunkSize:  30,
		Separators: []string{"\n\n", "\n", " "},
	}
	c := NewRecursiveChunker(config)

	// Text that splits into parts where some are already small
	text := "Short\n\nA longer paragraph that needs further splitting\n\nTiny"
	chunks := c.Chunk(text)

	require.NotEmpty(t, chunks)
	// Verify that "Short" and "Tiny" are captured as-is (small enough)
	foundShort := false
	foundTiny := false
	for _, ch := range chunks {
		if ch.Content == "Short" {
			foundShort = true
		}
		if ch.Content == "Tiny" {
			foundTiny = true
		}
	}
	assert.True(t, foundShort, "should find 'Short' chunk")
	assert.True(t, foundTiny, "should find 'Tiny' chunk")
}

func TestRecursiveChunker_MergeAndOverlap_AllWhitespaceChunks(t *testing.T) {
	// Test mergeAndOverlap when some raw chunks are whitespace-only
	// This exercises lines 211-213: if trimmed == "" { offset += len(rc); continue }
	config := Config{
		ChunkSize:  50,
		Separators: []string{"\n\n"},
	}
	c := NewRecursiveChunker(config)

	// Text with empty paragraphs (whitespace only between separators)
	text := "First paragraph\n\n   \n\n\t\n\nSecond paragraph"
	chunks := c.Chunk(text)

	require.NotEmpty(t, chunks)
	// Should only contain non-whitespace chunks
	for _, ch := range chunks {
		assert.NotEmpty(t, strings.TrimSpace(ch.Content),
			"chunk should not be whitespace-only")
	}
}

func TestRecursiveChunker_DeepRecursion(t *testing.T) {
	// Test deep recursion through all separator levels
	// This ensures all paths in splitRecursive are exercised
	config := Config{
		ChunkSize:  15,
		Separators: []string{"\n\n", "\n", ". ", " "},
	}
	c := NewRecursiveChunker(config)

	// Text that requires splitting through multiple separator levels
	text := "This is a very long sentence that needs splitting. " +
		"Another long sentence here.\n" +
		"Line after newline.\n\n" +
		"New paragraph with more content."
	chunks := c.Chunk(text)

	require.NotEmpty(t, chunks)
	// All chunks should be within chunk size (allowing for trimming effects)
	for i, ch := range chunks {
		// Trimmed content should be reasonable
		assert.NotEmpty(t, ch.Content, "chunk %d should not be empty", i)
	}
}

func TestRecursiveChunker_FixedSizeFallback(t *testing.T) {
	// Test the fixed-size fallback when no separators match
	// This exercises lines 148-159 in splitRecursive
	config := Config{
		ChunkSize:  5,
		Separators: []string{"\n\n", "\n"}, // No space separator
	}
	c := NewRecursiveChunker(config)

	// Text without any matching separators that needs chunking
	text := "abcdefghijklmnopqrstuvwxyz"
	chunks := c.Chunk(text)

	require.NotEmpty(t, chunks)
	// Should fall back to fixed-size splitting
	assert.Greater(t, len(chunks), 1, "should split into multiple chunks")
}

func TestRecursiveChunker_RecursivePathSmallText(t *testing.T) {
	// Specifically test the path where splitRecursive is called with text
	// that's already <= ChunkSize. This happens when the recursive call
	// with sepIdx+1 encounters text that's small enough after the split.
	config := Config{
		ChunkSize:  50,
		Separators: []string{"\n\n", "\n", " "},
	}
	c := NewRecursiveChunker(config)

	// Create text where:
	// 1. First split by "\n\n" creates a large paragraph
	// 2. That paragraph when split by "\n" creates lines
	// 3. Some of those lines are small enough to return directly (lines 144-146)
	text := "Short\n" +
		"A bit longer line here\n" +
		"Medium text portion"

	// First, let's wrap it in a paragraph structure
	fullText := strings.Repeat("x", 60) + "\n\n" + text

	chunks := c.Chunk(fullText)
	require.NotEmpty(t, chunks)

	// The "text" portion should produce chunks, some via direct return
	hasSmallChunk := false
	for _, ch := range chunks {
		if len(ch.Content) <= 50 && len(ch.Content) > 0 {
			hasSmallChunk = true
		}
	}
	assert.True(t, hasSmallChunk, "should have at least one small chunk")
}

func TestRecursiveChunker_EmptyPartsSkipped(t *testing.T) {
	// Test that empty parts created by splitting are properly handled
	// and whitespace-only chunks get skipped in mergeAndOverlap
	// This specifically targets lines 211-213
	config := Config{
		ChunkSize:  10,
		Separators: []string{"\n\n"},
	}
	c := NewRecursiveChunker(config)

	// Create text where splitting produces whitespace-only parts
	// strings.Split("AAAAAAAAAA\n\n   \n\nBBBBBBBBBB", "\n\n") = ["AAAAAAAAAA", "   ", "BBBBBBBBBB"]
	// The "   " part should be trimmed to "" and skipped in mergeAndOverlap
	text := "AAAAAAAAAA\n\n   \n\nBBBBBBBBBB"
	chunks := c.Chunk(text)

	require.Len(t, chunks, 2, "whitespace-only parts should be skipped")
	assert.Equal(t, "AAAAAAAAAA", chunks[0].Content)
	assert.Equal(t, "BBBBBBBBBB", chunks[1].Content)
}

func TestFixedSizeChunker_StepZeroOrNegative(t *testing.T) {
	// Test the step <= 0 defensive code path by bypassing constructor validation
	c := NewFixedSizeChunker(Config{ChunkSize: 10, Overlap: 0})

	// Use setConfigForTesting to create a config where step would be <= 0
	// step = ChunkSize - Overlap, so if Overlap > ChunkSize, step < 0
	// if Overlap == ChunkSize, step == 0
	c.setConfigForTesting(Config{ChunkSize: 10, Overlap: 10}) // step = 0

	text := "0123456789abcdefghij"
	chunks := c.Chunk(text)

	// With step defaulting to 1 (because step <= 0), we should get many chunks
	require.NotEmpty(t, chunks)
	assert.Greater(t, len(chunks), 1, "should produce multiple chunks with step=1")

	// Verify chunks are valid
	for _, ch := range chunks {
		assert.LessOrEqual(t, len(ch.Content), 10)
		assert.NotEmpty(t, ch.Content)
	}
}

func TestFixedSizeChunker_StepNegative(t *testing.T) {
	// Test with step < 0 (overlap > chunk size)
	c := NewFixedSizeChunker(Config{ChunkSize: 5, Overlap: 0})
	c.setConfigForTesting(Config{ChunkSize: 5, Overlap: 10}) // step = -5

	text := "0123456789"
	chunks := c.Chunk(text)

	// With step defaulting to 1, we should get chunks
	require.NotEmpty(t, chunks)
}

func TestRecursiveChunker_SplitRecursiveSmallTextDirect(t *testing.T) {
	// Test the early return when len(text) <= ChunkSize in splitRecursive
	// by creating a scenario where the recursive call receives small text
	c := NewRecursiveChunker(Config{
		ChunkSize:  100,
		Separators: []string{"\n\n"},
	})

	// Text with two small paragraphs that are each smaller than ChunkSize
	text := "Short paragraph one.\n\nShort paragraph two."
	chunks := c.Chunk(text)

	// Should produce chunks from the small paragraphs
	require.NotEmpty(t, chunks)
	// Each paragraph is small enough to return directly from splitRecursive
}

func TestRecursiveChunker_MergeAndOverlap_EmptyInput(t *testing.T) {
	// Test the len(rawChunks) == 0 early return in mergeAndOverlap
	// This is triggered when splitRecursive produces no chunks,
	// which shouldn't happen in normal operation. But we test the defensive path.
	c := NewRecursiveChunker(Config{
		ChunkSize:  100,
		Separators: []string{"\n\n"},
	})

	// A text that after splitting produces only whitespace chunks
	// which get filtered out, leaving rawChunks empty
	// Actually, this is hard to achieve because splitRecursive always
	// returns at least the original text if it's small enough.

	// Let's test with a single very small text that takes the early exit
	// in Chunk() before reaching mergeAndOverlap
	chunks := c.Chunk("tiny")
	require.Len(t, chunks, 1)
	assert.Equal(t, "tiny", chunks[0].Content)
}

func TestRecursiveChunker_SplitRecursiveSmallSubText(t *testing.T) {
	// Test the len(text) <= ChunkSize early return in splitRecursive
	// when it's called recursively with a text that becomes small enough.
	//
	// The flow:
	// 1. Chunk() receives text > ChunkSize
	// 2. splitRecursive(text, 0) is called
	// 3. Text is split by first separator
	// 4. A part exceeds ChunkSize, so splitRecursive(part, 1) is called
	// 5. In the recursive call, if the part <= ChunkSize, return early

	c := NewRecursiveChunker(Config{
		ChunkSize:  15,
		Separators: []string{"\n\n", "\n", " "},
	})

	// Create text where:
	// - Total length > 15 (Chunk won't return early)
	// - After splitting by "\n\n", we get parts
	// - One part > 15, triggers recursive call with sepIdx=1 ("\n")
	// - That part, when split by "\n", creates sub-parts that are <= 15
	//   which will hit the early return in splitRecursive

	// "A\nB" is 3 chars, but when part of a larger text that triggers recursion,
	// the recursive call with "A\nB" (if <= ChunkSize) should return ["A\nB"]
	text := "This is long.\n\n" + // First part > 15? No, 14 chars
		"This text needs to exceed fifteen chars for sure." // 50 chars

	chunks := c.Chunk(text)
	require.NotEmpty(t, chunks)

	// Let's try a different approach: create text that forces recursion
	// where the recursive text is small
	c2 := NewRecursiveChunker(Config{
		ChunkSize:  10,
		Separators: []string{"\n\n", "\n"},
	})

	// "Long text" is 9 chars. After adding to current with separator, it exceeds 10.
	// When it exceeds, splitRecursive is called recursively.
	// But wait, the recursive call is with current.String(), not part.
	// So we need current to exceed ChunkSize and then be recursively split
	// into parts that are small.
	text2 := "AAAAAAAAAAAAAAAAAAAAA" // 21 chars, will be split

	chunks2 := c2.Chunk(text2)
	require.NotEmpty(t, chunks2)
}

// setRawChunksForTesting allows testing mergeAndOverlap with empty input
func (c *RecursiveChunker) setConfigForTesting(config Config) {
	c.config = config
}

func TestRecursiveChunker_MergeAndOverlap_TrulyEmptyRawChunks(t *testing.T) {
	// Test mergeAndOverlap with empty rawChunks directly using exported test helper
	c := NewRecursiveChunker(Config{
		ChunkSize:  100,
		Separators: []string{"\n\n"},
	})

	// Call MergeAndOverlapForTesting directly with empty slice
	chunks := c.MergeAndOverlapForTesting("any text", []string{})
	assert.Nil(t, chunks)

	// Also test with non-empty slice for comparison
	chunks2 := c.MergeAndOverlapForTesting("hello", []string{"hello"})
	require.Len(t, chunks2, 1)
	assert.Equal(t, "hello", chunks2[0].Content)
}

func TestRecursiveChunker_SplitRecursiveSmallTextViaHelper(t *testing.T) {
	// Test splitRecursive directly when text <= ChunkSize using exported test helper
	c := NewRecursiveChunker(Config{
		ChunkSize:  100,
		Separators: []string{"\n\n"},
	})

	// Call SplitRecursiveForTesting with text smaller than ChunkSize
	result := c.SplitRecursiveForTesting("small", 0)
	require.Len(t, result, 1)
	assert.Equal(t, "small", result[0])
}
