package rag

import (
	"sort"

	"github.com/openfoundry/openfoundry-go/libs/ai-kernel-go/domain/llm"
	"github.com/openfoundry/openfoundry-go/libs/ai-kernel-go/models"
)

// Search retrieves the top-K knowledge hits matching the query.
// Mirrors Rust src/domain/rag/retriever.rs::search — embeds the
// query then delegates to SearchWithEmbedding.
func Search(query string, documents []models.KnowledgeDocument, topK uint32, minScore float32) []models.KnowledgeSearchResult {
	q := EmbedText(query)
	return SearchWithEmbedding(q, documents, topK, minScore)
}

// SearchWithEmbedding scores every chunk against the precomputed
// embedding via cosine similarity, drops everything below minScore,
// sorts by score desc, and truncates to max(topK, 1).
func SearchWithEmbedding(queryEmbedding []float32, documents []models.KnowledgeDocument, topK uint32, minScore float32) []models.KnowledgeSearchResult {
	hits := []models.KnowledgeSearchResult{}
	for _, doc := range documents {
		for _, chunk := range doc.Chunks {
			score := llm.CosineSimilarity(queryEmbedding, chunk.Embedding)
			if score < minScore {
				continue
			}
			hits = append(hits, models.KnowledgeSearchResult{
				KnowledgeBaseID: doc.KnowledgeBaseID,
				DocumentID:      doc.ID,
				DocumentTitle:   doc.Title,
				ChunkID:         chunk.ID,
				Score:           score,
				Excerpt:         chunk.Text,
				SourceURI:       doc.SourceURI,
				Metadata:        chunk.Metadata,
			})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].Score > hits[j].Score })
	limit := int(topK)
	if limit < 1 {
		limit = 1
	}
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}
