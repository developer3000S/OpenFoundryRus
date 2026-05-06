import { useState } from 'react';

import { MonacoEditor } from '@components/MonacoEditor';

type Language = 'json' | 'typescript' | 'python' | 'sql' | 'markdown';

const SAMPLES: Record<Language, string> = {
  json: '{\n  "service": "openfoundry",\n  "tier": "platform",\n  "ready": true\n}\n',
  typescript: `interface Dataset {\n  id: string;\n  rows: number;\n}\n\nexport function summarise(d: Dataset) {\n  return \`\${d.id} has \${d.rows} rows\`;\n}\n`,
  python: `def summarise(dataset):\n    return f"{dataset['id']} has {dataset['rows']} rows"\n\nprint(summarise({"id": "events", "rows": 1234}))\n`,
  sql: 'SELECT id, count(*) AS rows\nFROM events\nWHERE created_at > now() - interval \'1 day\'\nGROUP BY id\nORDER BY rows DESC;\n',
  markdown:
    '# OpenFoundry\n\n- React shell at port 5174\n- Svelte shell at port 5173\n- Backend at 8080\n\n```sql\nSELECT 1;\n```\n',
};

export function MonacoDemoPage() {
  const [language, setLanguage] = useState<Language>('typescript');
  const [value, setValue] = useState(SAMPLES.typescript);

  function selectLanguage(next: Language) {
    setLanguage(next);
    setValue(SAMPLES[next]);
  }

  return (
    <section className="of-page" style={{ display: 'grid', gap: 16 }}>
      <header>
        <p className="of-eyebrow">Capability validator</p>
        <h1 className="of-heading-xl">Monaco wrapper demo</h1>
        <p className="of-text-muted" style={{ marginTop: 8, maxWidth: 720 }}>
          Validates <code>&lt;MonacoEditor&gt;</code>: lazy <code>monaco-editor/esm</code> import,
          per-language contribution loading, value sync without echo, runtime{' '}
          <code>setModelLanguage</code>, and dispose on unmount. No worker setup yet — language
          services run on the main thread, mirroring the SvelteKit app.
        </p>
      </header>

      <div className="of-panel" style={{ padding: 20 }}>
        <div className="of-toolbar" style={{ marginBottom: 16 }}>
          <label style={{ display: 'flex', alignItems: 'center', gap: 8, fontSize: 13 }}>
            Language
            <select
              className="of-select"
              value={language}
              onChange={(e) => selectLanguage(e.target.value as Language)}
              style={{ minWidth: 160 }}
            >
              <option value="json">JSON</option>
              <option value="typescript">TypeScript</option>
              <option value="python">Python</option>
              <option value="sql">SQL</option>
              <option value="markdown">Markdown</option>
            </select>
          </label>
          <span className="of-text-muted" style={{ fontSize: 12 }}>
            Editor changes flow into <code>onChange</code>; the panel below mirrors the value.
          </span>
        </div>

        <MonacoEditor value={value} language={language} minHeight={320} onChange={setValue} />
      </div>

      <div className="of-panel-muted" style={{ padding: 16 }}>
        <p className="of-eyebrow">Live value</p>
        <pre
          className="of-scrollbar"
          style={{
            margin: '8px 0 0',
            padding: 12,
            background: '#fff',
            border: '1px solid var(--border-subtle)',
            borderRadius: 'var(--radius-sm)',
            fontSize: 12,
            maxHeight: 240,
            overflow: 'auto',
          }}
        >
          {value}
        </pre>
      </div>
    </section>
  );
}
