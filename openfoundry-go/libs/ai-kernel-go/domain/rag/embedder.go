// Package rag hosts the pure-logic RAG helpers (embedder, chunker,
// indexer, retriever).
package rag

import (
	"math"
	"strings"
)

// EmbedText produces a 12-dim normalised vector. Deterministic and
// dependency-free — used as the dev/test signal until a real
// embedding model is wired. Mirrors Rust src/domain/rag/embedder.rs.
func EmbedText(content string) []float32 {
	vector := make([]float32, 12)
	vectorLen := len(vector)
	idx := 0
	for _, token := range strings.Fields(strings.ToLower(content)) {
		if token == "" {
			continue
		}
		var tokenValue uint32
		for _, b := range []byte(token) {
			tokenValue = tokenValue + uint32(b)
		}
		vector[idx%vectorLen] += float32(tokenValue%997) / 997.0
		idx++
	}
	var sumSq float32
	for _, v := range vector {
		sumSq += v * v
	}
	magnitude := float32(math.Sqrt(float64(sumSq)))
	if magnitude > 0 {
		for i := range vector {
			vector[i] /= magnitude
		}
	}
	return vector
}
