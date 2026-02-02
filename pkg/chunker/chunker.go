// Package chunker provides document chunking strategies for splitting
// text into smaller, overlapping pieces suitable for embedding and retrieval.
package chunker

import (
	"strings"
	"unicode"
)

// Chunk represents a piece of text produced by a Chunker.
type Chunk struct {
	Content  string         `json:"content"`
	Start    int            `json:"start"`
	End      int            `json:"end"`
	Metadata map[string]any `json:"metadata,omitempty"`
}

// Chunker defines the interface for splitting text into chunks.
type Chunker interface {
	// Chunk splits the given text into a slice of Chunks.
	Chunk(text string) []Chunk
}

// Config holds configuration for chunkers.
type Config struct {
	ChunkSize  int      `json:"chunk_size"`
	Overlap    int      `json:"overlap"`
	Separators []string `json:"separators,omitempty"`
}

// DefaultConfig returns a default chunker configuration.
func DefaultConfig() Config {
	return Config{
		ChunkSize:  1000,
		Overlap:    200,
		Separators: []string{"\n\n", "\n", ". ", " "},
	}
}

// FixedSizeChunker splits text into fixed-size chunks with configurable overlap.
type FixedSizeChunker struct {
	config Config
}

// NewFixedSizeChunker creates a FixedSizeChunker with the given configuration.
func NewFixedSizeChunker(config Config) *FixedSizeChunker {
	if config.ChunkSize <= 0 {
		config.ChunkSize = 1000
	}
	if config.Overlap < 0 {
		config.Overlap = 0
	}
	if config.Overlap >= config.ChunkSize {
		config.Overlap = config.ChunkSize / 4
	}
	return &FixedSizeChunker{config: config}
}

// Chunk splits text into fixed-size chunks with overlap.
func (c *FixedSizeChunker) Chunk(text string) []Chunk {
	if text == "" {
		return nil
	}

	if len(text) <= c.config.ChunkSize {
		return []Chunk{{
			Content: text,
			Start:   0,
			End:     len(text),
		}}
	}

	var chunks []Chunk
	step := c.config.ChunkSize - c.config.Overlap
	if step <= 0 {
		step = 1
	}

	for start := 0; start < len(text); start += step {
		end := start + c.config.ChunkSize
		if end > len(text) {
			end = len(text)
		}

		content := text[start:end]
		chunks = append(chunks, Chunk{
			Content: content,
			Start:   start,
			End:     end,
		})

		if end == len(text) {
			break
		}
	}

	return chunks
}

// RecursiveChunker splits text by trying separators in order,
// falling back to the next separator when chunks are too large.
type RecursiveChunker struct {
	config Config
}

// NewRecursiveChunker creates a RecursiveChunker with the given configuration.
func NewRecursiveChunker(config Config) *RecursiveChunker {
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
	return &RecursiveChunker{config: config}
}

// Chunk splits text recursively using the configured separators.
func (c *RecursiveChunker) Chunk(text string) []Chunk {
	if text == "" {
		return nil
	}

	if len(text) <= c.config.ChunkSize {
		return []Chunk{{
			Content: text,
			Start:   0,
			End:     len(text),
		}}
	}

	rawChunks := c.splitRecursive(text, 0)

	// Merge small chunks and apply overlap
	return c.mergeAndOverlap(text, rawChunks)
}

func (c *RecursiveChunker) splitRecursive(text string, sepIdx int) []string {
	if len(text) <= c.config.ChunkSize {
		return []string{text}
	}

	if sepIdx >= len(c.config.Separators) {
		// No more separators; fall back to fixed-size splitting
		var parts []string
		for i := 0; i < len(text); i += c.config.ChunkSize {
			end := i + c.config.ChunkSize
			if end > len(text) {
				end = len(text)
			}
			parts = append(parts, text[i:end])
		}
		return parts
	}

	sep := c.config.Separators[sepIdx]
	parts := strings.Split(text, sep)

	var result []string
	var current strings.Builder

	for i, part := range parts {
		// If adding this part would exceed chunk size
		if current.Len() > 0 &&
			current.Len()+len(sep)+len(part) > c.config.ChunkSize {
			// Save what we have
			result = append(result, current.String())
			current.Reset()
		}

		if current.Len() > 0 {
			current.WriteString(sep)
		}
		current.WriteString(part)

		// If current is too large even alone, split with next separator
		if current.Len() > c.config.ChunkSize {
			subParts := c.splitRecursive(current.String(), sepIdx+1)
			result = append(result, subParts...)
			current.Reset()
			continue
		}

		// Flush on last part
		if i == len(parts)-1 && current.Len() > 0 {
			result = append(result, current.String())
		}
	}

	return result
}

func (c *RecursiveChunker) mergeAndOverlap(
	originalText string,
	rawChunks []string,
) []Chunk {
	if len(rawChunks) == 0 {
		return nil
	}

	chunks := make([]Chunk, 0, len(rawChunks))
	offset := 0

	for _, rc := range rawChunks {
		trimmed := strings.TrimSpace(rc)
		if trimmed == "" {
			offset += len(rc)
			continue
		}

		// Find actual position in original text
		idx := strings.Index(originalText[offset:], trimmed)
		start := offset
		if idx >= 0 {
			start = offset + idx
		}

		chunks = append(chunks, Chunk{
			Content: trimmed,
			Start:   start,
			End:     start + len(trimmed),
		})
		offset = start + len(trimmed)
	}

	return chunks
}

// SentenceChunker splits text by sentence boundaries, grouping
// sentences into chunks that do not exceed the configured size.
type SentenceChunker struct {
	config Config
}

// NewSentenceChunker creates a SentenceChunker with the given configuration.
func NewSentenceChunker(config Config) *SentenceChunker {
	if config.ChunkSize <= 0 {
		config.ChunkSize = 1000
	}
	if config.Overlap < 0 {
		config.Overlap = 0
	}
	return &SentenceChunker{config: config}
}

// Chunk splits text into chunks aligned to sentence boundaries.
func (c *SentenceChunker) Chunk(text string) []Chunk {
	if text == "" {
		return nil
	}

	sentences := splitSentences(text)
	if len(sentences) == 0 {
		return []Chunk{{
			Content: text,
			Start:   0,
			End:     len(text),
		}}
	}

	var chunks []Chunk
	var current strings.Builder
	startPos := 0
	currentStart := 0

	for _, sent := range sentences {
		sentWithSpace := sent
		if current.Len() > 0 {
			sentWithSpace = " " + sent
		}

		if current.Len()+len(sentWithSpace) > c.config.ChunkSize &&
			current.Len() > 0 {
			content := current.String()
			chunks = append(chunks, Chunk{
				Content: content,
				Start:   currentStart,
				End:     currentStart + len(content),
			})

			// Handle overlap by keeping trailing sentences
			if c.config.Overlap > 0 {
				overlapText := getOverlapText(
					content, c.config.Overlap,
				)
				current.Reset()
				current.WriteString(overlapText)
				currentStart = currentStart + len(content) - len(overlapText)
			} else {
				current.Reset()
				currentStart = startPos
			}
		}

		if current.Len() == 0 {
			currentStart = startPos
			current.WriteString(sent)
		} else {
			current.WriteString(" ")
			current.WriteString(sent)
		}
		startPos += len(sent) + 1 // +1 for the space between sentences
	}

	if current.Len() > 0 {
		content := current.String()
		chunks = append(chunks, Chunk{
			Content: content,
			Start:   currentStart,
			End:     currentStart + len(content),
		})
	}

	return chunks
}

// splitSentences splits text into sentences at common boundary characters.
func splitSentences(text string) []string {
	var sentences []string
	var current strings.Builder

	runes := []rune(text)
	for i, r := range runes {
		current.WriteRune(r)

		isSentenceEnd := r == '.' || r == '!' || r == '?'
		isLast := i == len(runes)-1
		nextIsSpace := !isLast && unicode.IsSpace(runes[i+1])

		if isSentenceEnd && (isLast || nextIsSpace) {
			sentence := strings.TrimSpace(current.String())
			if sentence != "" {
				sentences = append(sentences, sentence)
			}
			current.Reset()
		}
	}

	// Remaining text
	if remaining := strings.TrimSpace(current.String()); remaining != "" {
		sentences = append(sentences, remaining)
	}

	return sentences
}

// getOverlapText returns the trailing portion of text up to maxLen.
func getOverlapText(text string, maxLen int) string {
	if len(text) <= maxLen {
		return text
	}
	return text[len(text)-maxLen:]
}
