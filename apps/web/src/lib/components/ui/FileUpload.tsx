import { useState } from 'react';

export interface AttachmentMetadata {
  attachment_rid: string;
  filename: string;
  content_type: string | null;
  size_bytes: number;
  storage_uri: string;
}

interface FileUploadProps {
  label?: string;
  accept?: string;
  onUploaded: (attachment: AttachmentMetadata) => void;
}

export function FileUpload({ label = 'Choose file', accept, onUploaded }: FileUploadProps) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  async function handleChange(e: React.ChangeEvent<HTMLInputElement>) {
    const input = e.currentTarget;
    const file = input.files?.[0];
    if (!file) return;
    setBusy(true);
    setError('');
    try {
      const buffer = await file.arrayBuffer();
      const base64 = btoa(String.fromCharCode(...new Uint8Array(buffer)));
      const response = await fetch('/api/v1/ontology/actions/uploads', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({
          filename: file.name,
          content_type: file.type || null,
          size_bytes: file.size,
          content_base64: base64,
        }),
      });
      if (!response.ok) {
        setError(`${response.status}: ${await response.text()}`);
        return;
      }
      const attachment = (await response.json()) as AttachmentMetadata;
      onUploaded(attachment);
      input.value = '';
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  }

  return (
    <div style={{ display: 'grid', gap: 4 }}>
      <label
        style={{
          display: 'inline-flex',
          alignItems: 'center',
          gap: 8,
          padding: '8px 12px',
          border: '1px solid var(--border-default)',
          borderRadius: 12,
          fontSize: 13,
          cursor: busy ? 'not-allowed' : 'pointer',
          width: 'fit-content',
        }}
      >
        {label}
        <input type="file" accept={accept} disabled={busy} onChange={handleChange} style={{ display: 'none' }} />
      </label>
      {busy && <p style={{ fontSize: 11, color: 'var(--text-muted)' }}>Uploading…</p>}
      {error && <p style={{ fontSize: 11, color: '#b91c1c' }}>{error}</p>}
    </div>
  );
}
