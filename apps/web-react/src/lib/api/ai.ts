import api from './client';

// ────────────────────────────────────────────────────────────────
// AI / knowledge-base slice — minimal surface used by /notepad
// (list available knowledge bases + create a knowledge document).
// Future AI routes will keep extending this module.
// ────────────────────────────────────────────────────────────────

export interface ListResponse<T> {
  data: T[];
}

export interface KnowledgeBase {
  id: string;
  name: string;
  description: string;
  status: string;
  embedding_provider: string;
  chunking_strategy: string;
  tags: string[];
  document_count: number;
  chunk_count: number;
  created_at: string;
  updated_at: string;
}

export interface KnowledgeDocument {
  id: string;
  knowledge_base_id: string;
  title: string;
  content: string;
  source_uri: string | null;
  metadata: Record<string, unknown>;
  status: string;
  chunk_count: number;
  created_at: string;
  updated_at: string;
}

export function listKnowledgeBases() {
  return api.get<ListResponse<KnowledgeBase>>('/ai/knowledge-bases');
}

export function createKnowledgeDocument(
  knowledgeBaseId: string,
  body: {
    title: string;
    content: string;
    source_uri?: string;
    metadata?: Record<string, unknown>;
  },
) {
  return api.post<KnowledgeDocument>(`/ai/knowledge-bases/${knowledgeBaseId}/documents`, body);
}
