package rag

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/openfoundry/openfoundry-go/libs/ai-kernel-go/models"
)

// --- embedder ------------------------------------------------------------

func TestEmbedTextNormalised(t *testing.T) {
	t.Parallel()
	v := EmbedText("hello world")
	assert.Len(t, v, 12)
	var sumSq float32
	for _, x := range v {
		sumSq += x * x
	}
	assert.InDelta(t, 1.0, sumSq, 1e-4)
}

func TestEmbedTextDeterministic(t *testing.T) {
	t.Parallel()
	a := EmbedText("hello world")
	b := EmbedText("hello world")
	assert.Equal(t, a, b, "same input → same embedding")
}

// --- chunker -------------------------------------------------------------

func TestChunkTextSplitsOnParagraphs(t *testing.T) {
	t.Parallel()
	in := "First paragraph.\n\nSecond paragraph.\n\nThird paragraph."
	chunks := ChunkText(in, 30)
	require.NotEmpty(t, chunks)
	for _, c := range chunks {
		assert.LessOrEqual(t, len(c.Text), 60, "chunk should be near the limit")
	}
	// Positions sequential starting at 0.
	for i, c := range chunks {
		assert.Equal(t, int32(i), c.Position)
	}
}

func TestChunkTextSplitsLargeParagraphOnSentences(t *testing.T) {
	t.Parallel()
	long := "A. " + strings.Repeat("Sentence about something. ", 30)
	chunks := ChunkText(long, 100)
	assert.Greater(t, len(chunks), 1, "long content should split into multiple chunks")
}

// --- retriever -----------------------------------------------------------

func TestSearchScoresAndOrders(t *testing.T) {
	t.Parallel()
	docs := []models.KnowledgeDocument{
		{
			ID:              uuid.New(),
			KnowledgeBaseID: uuid.New(),
			Title:           "About foundry",
			Chunks: []models.KnowledgeChunk{
				{ID: "c1", Text: "foundry platform", Embedding: EmbedText("foundry platform")},
				{ID: "c2", Text: "unrelated content", Embedding: EmbedText("totally different")},
			},
		},
	}
	hits := Search("foundry", docs, 5, 0.0)
	require.NotEmpty(t, hits)
	// Highest score first.
	for i := 1; i < len(hits); i++ {
		assert.GreaterOrEqual(t, hits[i-1].Score, hits[i].Score)
	}
}

func TestSearchTopKAndMinScore(t *testing.T) {
	t.Parallel()
	docs := []models.KnowledgeDocument{
		{
			ID: uuid.New(), KnowledgeBaseID: uuid.New(),
			Chunks: []models.KnowledgeChunk{
				{ID: "c1", Text: "alpha beta", Embedding: EmbedText("alpha beta")},
				{ID: "c2", Text: "gamma delta", Embedding: EmbedText("gamma delta")},
				{ID: "c3", Text: "epsilon zeta", Embedding: EmbedText("epsilon zeta")},
			},
		},
	}
	hits := Search("alpha", docs, 1, 0.0)
	assert.Len(t, hits, 1, "top_k truncates to 1")

	hitsHigh := Search("alpha", docs, 5, 0.999)
	// Min score 0.999 is above similarity for unrelated chunks; only
	// near-perfect matches survive.
	for _, h := range hitsHigh {
		assert.GreaterOrEqual(t, h.Score, float32(0.999))
	}
}

// --- indexer -------------------------------------------------------------

func TestIndexDocumentFineVsDefault(t *testing.T) {
	t.Parallel()
	docID := uuid.New()
	long := strings.Repeat("paragraph content with some text.\n\n", 30)
	fine := IndexDocument(docID, long, "fine")
	def := IndexDocument(docID, long, "balanced")
	// Fine strategy → smaller max_chars (320) → strictly more chunks.
	assert.Greater(t, len(fine), len(def), "fine strategy produces more chunks than balanced")
	for _, c := range fine {
		assert.Contains(t, c.ID, docID.String())
		assert.Greater(t, len(c.Embedding), 0)
		assert.Contains(t, string(c.Metadata), "fine")
	}
}
