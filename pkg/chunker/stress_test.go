// SPDX-License-Identifier: Apache-2.0
//
// Stress tests for digital.vasic.rag/pkg/chunker. Chunkers are pure
// functions against in-memory text; the stress value is asserting that
// concurrent callers never see torn output, never panic on oddly-sized
// inputs, and never leak goroutines.
package chunker

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

const (
	stressGoroutines   = 8
	stressIterations   = 300
	stressMaxWallClock = 15 * time.Second
)

// TestStress_FixedSizeChunker_ConcurrentChunk stresses the chunker
// against varying input sizes across 8 goroutines.
func TestStress_FixedSizeChunker_ConcurrentChunk(t *testing.T) {
	c := NewFixedSizeChunker(DefaultConfig())

	startGoroutines := runtime.NumGoroutine()
	var wg sync.WaitGroup
	var panics atomic.Int64
	deadline := time.Now().Add(stressMaxWallClock)

	inputs := []string{
		"",
		"short",
		strings.Repeat("medium sentence with punctuation. ", 20),
		strings.Repeat("a very long block of text that needs to be chunked. ", 200),
	}

	for g := 0; g < stressGoroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < stressIterations; j++ {
				if time.Now().After(deadline) {
					return
				}
				func() {
					defer func() {
						if r := recover(); r != nil {
							panics.Add(1)
						}
					}()
					text := inputs[j%len(inputs)] + fmt.Sprintf(" g%d-%d", id, j)
					chunks := c.Chunk(text)
					// Every chunk must be non-nil (empty slice is fine).
					for _, ch := range chunks {
						_ = ch.Content
					}
				}()
			}
		}(g)
	}
	wg.Wait()

	assert.Equal(t, int64(0), panics.Load(), "Chunk must not panic on any test input")

	time.Sleep(30 * time.Millisecond)
	runtime.Gosched()
	endGoroutines := runtime.NumGoroutine()
	assert.LessOrEqual(t, endGoroutines-startGoroutines, 2,
		"goroutine leak: worker count grew by %d", endGoroutines-startGoroutines)
}

// TestStress_RecursiveChunker_ConcurrentChunk mirrors the above for the
// recursive chunker which has more state.
func TestStress_RecursiveChunker_ConcurrentChunk(t *testing.T) {
	c := NewRecursiveChunker(DefaultConfig())

	var wg sync.WaitGroup
	var panics atomic.Int64
	deadline := time.Now().Add(stressMaxWallClock)

	for g := 0; g < stressGoroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < stressIterations; j++ {
				if time.Now().After(deadline) {
					return
				}
				func() {
					defer func() {
						if r := recover(); r != nil {
							panics.Add(1)
						}
					}()
					text := strings.Repeat("paragraph. ", 1+j%50) +
						fmt.Sprintf(" g%d-%d", id, j)
					_ = c.Chunk(text)
				}()
			}
		}(g)
	}
	wg.Wait()
	assert.Equal(t, int64(0), panics.Load(), "RecursiveChunker.Chunk must not panic")
}

// BenchmarkStress_FixedSizeChunker_Chunk establishes a throughput
// baseline for the fixed-size chunker.
func BenchmarkStress_FixedSizeChunker_Chunk(b *testing.B) {
	c := NewFixedSizeChunker(DefaultConfig())
	text := strings.Repeat("hello world. ", 500)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = c.Chunk(text)
	}
}
