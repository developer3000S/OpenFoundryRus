import type { CellOutput as NotebookCellOutput } from '@/lib/api/notebooks';

interface TableContent {
  columns: Array<{ name: string; data_type: string }>;
  rows: string[][];
  total_rows: number;
  execution_time_ms: number;
}

interface LlmContent {
  reply: string;
  provider_name?: string;
  conversation_id?: string;
  citations?: Array<{ document_title?: string; excerpt?: string; source_uri?: string | null }>;
  usage?: {
    total_tokens?: number;
    latency_ms?: number;
    estimated_cost_usd?: number;
  };
}

function getTableContent(content: unknown): TableContent | null {
  if (!content || typeof content !== 'object') return null;
  const candidate = content as Partial<TableContent>;
  if (!Array.isArray(candidate.columns) || !Array.isArray(candidate.rows)) return null;
  return candidate as TableContent;
}

function getLlmContent(content: unknown): LlmContent | null {
  if (!content || typeof content !== 'object') return null;
  const candidate = content as Partial<LlmContent>;
  return typeof candidate.reply === 'string' ? (candidate as LlmContent) : null;
}

function formatContent(content: unknown): string {
  return typeof content === 'string' ? content : JSON.stringify(content, null, 2);
}

const PRE_BASE: React.CSSProperties = {
  margin: 0,
  padding: 16,
  fontFamily: 'var(--font-mono)',
  fontSize: 12,
  overflow: 'auto',
};

interface CellOutputProps {
  output: NotebookCellOutput | null | undefined;
}

export function CellOutput({ output }: CellOutputProps) {
  if (!output) return null;

  const wrapperStyle: React.CSSProperties = {
    borderTop: '1px solid #1e293b',
    background: '#0f172a',
    color: '#f1f5f9',
  };

  if (output.output_type === 'table') {
    const table = getTableContent(output.content);
    if (!table) {
      return (
        <div style={wrapperStyle}>
          <pre style={{ ...PRE_BASE, color: '#e2e8f0' }}>{formatContent(output.content)}</pre>
        </div>
      );
    }
    return (
      <div style={wrapperStyle}>
        <div style={{ borderBottom: '1px solid #1e293b', padding: '8px 16px', fontSize: 11, color: '#94a3b8' }}>
          {table.total_rows} rows in {table.execution_time_ms}ms
        </div>
        <div className="of-scrollbar" style={{ maxHeight: 288, overflow: 'auto' }}>
          <table style={{ minWidth: '100%', textAlign: 'left', fontSize: 13, borderCollapse: 'collapse' }}>
            <thead style={{ position: 'sticky', top: 0, background: '#1e293b', textTransform: 'uppercase', fontSize: 11, color: '#94a3b8', letterSpacing: '0.04em' }}>
              <tr>
                {table.columns.map((column) => (
                  <th key={column.name} style={{ borderBottom: '1px solid #1e293b', padding: '8px 16px', fontWeight: 500 }}>
                    {column.name}
                    <span style={{ marginLeft: 8, fontSize: 10, textTransform: 'none', color: '#64748b' }}>
                      {column.data_type}
                    </span>
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {table.rows.map((row, index) => (
                <tr key={index} style={{ background: index % 2 === 0 ? 'rgba(15, 23, 42, 0.4)' : undefined }}>
                  {row.map((cell, cellIndex) => (
                    <td
                      key={cellIndex}
                      style={{
                        borderBottom: '1px solid #0f172a',
                        padding: '8px 16px',
                        fontFamily: 'var(--font-mono)',
                        fontSize: 11,
                        color: '#e2e8f0',
                      }}
                    >
                      {cell}
                    </td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </div>
    );
  }

  if (output.output_type === 'llm') {
    const llm = getLlmContent(output.content);
    if (!llm) {
      return (
        <div style={wrapperStyle}>
          <pre style={{ ...PRE_BASE, color: '#e2e8f0' }}>{formatContent(output.content)}</pre>
        </div>
      );
    }
    return (
      <div style={wrapperStyle}>
        <div style={{ display: 'grid', gap: 16, padding: 16 }}>
          <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 8, fontSize: 11, color: '#94a3b8' }}>
            <span
              style={{
                padding: '4px 8px',
                border: '1px solid rgba(56, 189, 248, 0.4)',
                background: 'rgba(8, 47, 73, 0.4)',
                color: '#7dd3fc',
                fontWeight: 600,
                textTransform: 'uppercase',
                letterSpacing: '0.2em',
                borderRadius: 999,
              }}
            >
              LLM
            </span>
            {llm.provider_name && <span>{llm.provider_name}</span>}
            {llm.usage?.total_tokens !== undefined && <span>· {llm.usage.total_tokens} tokens</span>}
            {llm.usage?.latency_ms !== undefined && <span>· {llm.usage.latency_ms}ms</span>}
            {llm.usage?.estimated_cost_usd !== undefined && (
              <span>· ${llm.usage.estimated_cost_usd.toFixed(4)}</span>
            )}
          </div>
          <div style={{ whiteSpace: 'pre-wrap', fontSize: 13, lineHeight: 1.7, color: '#f1f5f9' }}>
            {llm.reply}
          </div>
          {llm.citations && llm.citations.length > 0 && (
            <div
              style={{
                border: '1px solid #1e293b',
                background: 'rgba(15, 23, 42, 0.7)',
                borderRadius: 16,
                padding: 12,
              }}
            >
              <div className="of-eyebrow" style={{ color: '#94a3b8' }}>
                Citations
              </div>
              <div style={{ marginTop: 12, display: 'grid', gap: 8 }}>
                {llm.citations.map((citation, index) => (
                  <div
                    key={index}
                    style={{
                      border: '1px solid #1e293b',
                      background: 'rgba(2, 6, 23, 0.8)',
                      borderRadius: 12,
                      padding: '8px 12px',
                    }}
                  >
                    <div style={{ fontSize: 13, fontWeight: 500, color: '#f1f5f9' }}>
                      {citation.document_title ?? 'Knowledge document'}
                    </div>
                    <div style={{ marginTop: 4, fontSize: 11, lineHeight: 1.6, color: '#94a3b8' }}>
                      {citation.excerpt ?? 'No excerpt available.'}
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}
        </div>
      </div>
    );
  }

  if (output.output_type === 'error') {
    return (
      <div style={wrapperStyle}>
        <pre style={{ ...PRE_BASE, color: '#fda4af' }}>{formatContent(output.content)}</pre>
      </div>
    );
  }

  return (
    <div style={wrapperStyle}>
      <pre style={{ ...PRE_BASE, color: '#6ee7b7' }}>{formatContent(output.content)}</pre>
    </div>
  );
}
