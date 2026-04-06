package snapshot

import (
	"fmt"
	"strings"
)

// TextChunk represents a segment of text with context window metadata.
type TextChunk struct {
	Content       string // actual content
	ChunkIndex    int    // 0-based chunk number
	TotalChunks   int    // total number of chunks
	Tokens        int    // estimated token count for this chunk
	StartLine     int    // original line number in source
	EndLine       int    // original line number in source
	Contextual    string // full text with headers/footers for injection
}

// SplitOptions configures text splitting behavior.
type SplitOptions struct {
	ModelContextWindow int64
	ReserveTokens     int64  // tokens reserved for system prompt + user query (default 2000)
	ChunkOverlap      int    // lines to overlap between chunks for context (default 10)
	PreferParagraphs  bool   // try to split on paragraph boundaries (default true)
	Language          string // hint for language-specific splitting (optional)
}

// DefaultSplitOptions returns sensible defaults.
func DefaultSplitOptions(contextWindow int64) SplitOptions {
	return SplitOptions{
		ModelContextWindow: contextWindow,
		ReserveTokens:      2000,
		ChunkOverlap:       10,
		PreferParagraphs:   true,
	}
}

// SplitText splits large text into chunks that fit within a context window.
// Returns chunks with contextualisation (headers/footers indicating position in file).
func SplitText(content string, opts SplitOptions) []TextChunk {
	if content == "" {
		return nil
	}

	// Calculate available tokens per chunk
	availableTokens := opts.ModelContextWindow - opts.ReserveTokens
	if availableTokens < 1000 {
		availableTokens = 1000 // minimum viable chunk
	}

	// Estimate bytes per chunk (rough: 4 bytes per token)
	bytesPerChunk := int(availableTokens) * 4

	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return nil
	}

	// Build chunks
	var chunks []TextChunk
	var currentChunkLines []string
	currentChunkBytes := 0
	startLine := 0

	for i, line := range lines {
		lineBytes := len(line) + 1 // +1 for newline

		// Check if adding this line exceeds budget
		if currentChunkBytes+lineBytes > bytesPerChunk && len(currentChunkLines) > 0 {
			// Save current chunk
			chunk := buildChunk(currentChunkLines, startLine, i-1, content)
			chunks = append(chunks, chunk)

			// Start new chunk with overlap
			overlapStart := max(0, len(currentChunkLines)-int(opts.ChunkOverlap))
			currentChunkLines = currentChunkLines[overlapStart:]
			currentChunkBytes = 0
			for _, l := range currentChunkLines {
				currentChunkBytes += len(l) + 1
			}
			startLine = i - len(currentChunkLines)
		}

		currentChunkLines = append(currentChunkLines, line)
		currentChunkBytes += lineBytes
	}

	// Add final chunk
	if len(currentChunkLines) > 0 {
		chunk := buildChunk(currentChunkLines, startLine, len(lines)-1, content)
		chunks = append(chunks, chunk)
	}

	// Update chunk indices and contextualize
	for i := range chunks {
		chunks[i].ChunkIndex = i
		chunks[i].TotalChunks = len(chunks)
		chunks[i].Contextual = contextualizeChunk(&chunks[i], len(lines))
	}

	return chunks
}

// buildChunk creates a TextChunk from a slice of lines.
func buildChunk(lines []string, startLine, endLine int, fullContent string) TextChunk {
	content := strings.Join(lines, "\n")
	return TextChunk{
		Content:    content,
		StartLine:  startLine,
		EndLine:    endLine,
		Tokens:     EstimateTokens(content),
	}
}

// contextualizeChunk wraps a chunk with contextual headers and footers.
func contextualizeChunk(chunk *TextChunk, totalLines int) string {
	var sb strings.Builder

	// Header indicating position
	sb.WriteString(fmt.Sprintf("=== CHUNK %d of %d ===\n", chunk.ChunkIndex+1, chunk.TotalChunks))
	sb.WriteString(fmt.Sprintf("Lines %d-%d (of ~%d)\n", chunk.StartLine+1, chunk.EndLine+1, totalLines))

	if chunk.ChunkIndex > 0 {
		sb.WriteString(fmt.Sprintf("[Context continues from chunk %d]\n", chunk.ChunkIndex))
	}

	sb.WriteString("\n")
	sb.WriteString(chunk.Content)
	sb.WriteString("\n\n")

	// Footer
	if chunk.ChunkIndex < chunk.TotalChunks-1 {
		sb.WriteString(fmt.Sprintf("[Context continues in chunk %d]\n", chunk.ChunkIndex+2))
	} else {
		sb.WriteString("[END OF FILE]\n")
	}

	return sb.String()
}

// ChunkReport generates a human-readable report of how text will be split.
type ChunkReport struct {
	TotalTokens   int
	ChunkCount    int
	AvgTokens     int
	MaxTokens     int
	MinTokens     int
	Chunks        []ChunkInfo
}

// ChunkInfo holds summary information for one chunk.
type ChunkInfo struct {
	Index      int
	Tokens     int
	Lines      string // "startLine-endLine"
	Percentage int    // percent of total tokens
}

// GenerateReport creates a summary report of how text will be split.
func GenerateReport(chunks []TextChunk) ChunkReport {
	report := ChunkReport{
		ChunkCount: len(chunks),
	}

	if len(chunks) == 0 {
		return report
	}

	totalTokens := 0
	maxTokens := 0
	minTokens := chunks[0].Tokens

	for i, chunk := range chunks {
		totalTokens += chunk.Tokens
		if chunk.Tokens > maxTokens {
			maxTokens = chunk.Tokens
		}
		if chunk.Tokens < minTokens {
			minTokens = chunk.Tokens
		}

		percentage := 0
		if totalTokens > 0 {
			percentage = (chunk.Tokens * 100) / totalTokens
		}

		report.Chunks = append(report.Chunks, ChunkInfo{
			Index:      i + 1,
			Tokens:     chunk.Tokens,
			Lines:      fmt.Sprintf("%d-%d", chunk.StartLine+1, chunk.EndLine+1),
			Percentage: percentage,
		})
	}

	report.TotalTokens = totalTokens
	report.AvgTokens = totalTokens / len(chunks)
	report.MaxTokens = maxTokens
	report.MinTokens = minTokens

	return report
}

// FormatReport returns a human-readable report string.
func FormatReport(report ChunkReport) string {
	if report.ChunkCount == 0 {
		return ""
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("📋 Splitting into %d chunks (~%d tokens each):\n", report.ChunkCount, report.AvgTokens))

	for _, info := range report.Chunks {
		bar := strings.Repeat("█", max(1, info.Percentage/5))
		sb.WriteString(fmt.Sprintf("  [%d] %s tokens | %s | %s\n",
			info.Index, padInt(info.Tokens, 5), info.Lines, bar))
	}

	sb.WriteString(fmt.Sprintf("Total: %d tokens across %d chunks\n", report.TotalTokens, report.ChunkCount))
	return sb.String()
}

func padInt(n, width int) string {
	s := fmt.Sprintf("%d", n)
	for len(s) < width {
		s = " " + s
	}
	return s
}
