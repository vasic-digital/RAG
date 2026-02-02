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
