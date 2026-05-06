package rag

import "strings"

// Chunk is the (position, text) pair returned by ChunkText.
type Chunk struct {
	Position int32
	Text     string
}

// ChunkText splits a document into chunks no larger than maxChars.
// Algorithm mirrors Rust src/domain/rag/chunker.rs:
//   - Split on "\n\n" paragraphs.
//   - When the next paragraph would push the buffer past maxChars
//     (and the buffer is non-empty), flush the buffer as a chunk
//     and start fresh.
//   - When a single paragraph itself exceeds maxChars, split on
//     "." sentences and apply the same buffer-flush rule.
//   - Trim each emitted chunk and assign sequential positions
//     starting at 0.
func ChunkText(content string, maxChars int) []Chunk {
	chunks := []Chunk{}
	var buffer strings.Builder
	position := int32(0)

	for _, paragraph := range strings.Split(content, "\n\n") {
		trimmed := strings.TrimSpace(paragraph)
		if trimmed == "" {
			continue
		}

		if buffer.Len()+len(trimmed)+2 > maxChars && buffer.Len() > 0 {
			chunks = append(chunks, Chunk{Position: position, Text: strings.TrimSpace(buffer.String())})
			position++
			buffer.Reset()
		}

		if len(trimmed) > maxChars {
			for _, sentence := range strings.Split(trimmed, ".") {
				sentence = strings.TrimSpace(sentence)
				if sentence == "" {
					continue
				}
				if buffer.Len()+len(sentence)+2 > maxChars && buffer.Len() > 0 {
					chunks = append(chunks, Chunk{Position: position, Text: strings.TrimSpace(buffer.String())})
					position++
					buffer.Reset()
				}
				buffer.WriteString(sentence)
				buffer.WriteString(". ")
			}
		} else {
			buffer.WriteString(trimmed)
			buffer.WriteString("\n\n")
		}
	}

	if strings.TrimSpace(buffer.String()) != "" {
		chunks = append(chunks, Chunk{Position: position, Text: strings.TrimSpace(buffer.String())})
	}
	return chunks
}
