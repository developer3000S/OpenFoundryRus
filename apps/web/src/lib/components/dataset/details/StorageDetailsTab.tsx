import { useEffect, useState } from 'react';

import { getDatasetStorageDetails, type DatasetStorageDetails } from '@/lib/api/datasets';

interface StorageDetailsTabProps {
  datasetRid: string;
}

function fmtBytes(value: number) {
  if (value < 1024) return `${value} B`;
  if (value < 1024 * 1024) return `${(value / 1024).toFixed(1)} KB`;
  if (value < 1024 * 1024 * 1024) return `${(value / (1024 * 1024)).toFixed(1)} MB`;
  return `${(value / (1024 * 1024 * 1024)).toFixed(2)} GB`;
}

function bucketFromFsId(fsId: string) {
  if (fsId.startsWith('s3:')) return fsId.slice(3);
  if (fsId.startsWith('hdfs:')) return fsId.slice(5);
  return '—';
}

function Card({ label, value, mono = false }: { label: string; value: string; mono?: boolean }) {
  return (
    <div className="of-panel" style={{ padding: 16 }}>
      <div style={{ fontSize: 11, textTransform: 'uppercase', color: 'var(--text-muted)', letterSpacing: '0.18em' }}>{label}</div>
      <div
        style={{
          marginTop: 8,
          ...(mono
            ? { fontFamily: 'var(--font-mono)', fontSize: 13, wordBreak: 'break-all' }
            : { fontSize: 24, fontWeight: 600 }),
        }}
      >
        {value}
      </div>
    </div>
  );
}

export function StorageDetailsTab({ datasetRid }: StorageDetailsTabProps) {
  const [details, setDetails] = useState<DatasetStorageDetails | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!datasetRid) return;
    let cancelled = false;
    setLoading(true);
    setError(null);
    getDatasetStorageDetails(datasetRid)
      .then((d) => { if (!cancelled) setDetails(d); })
      .catch((cause: unknown) => {
        if (!cancelled) {
          setError(cause instanceof Error ? cause.message : 'Storage details failed.');
          setDetails(null);
        }
      })
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [datasetRid]);

  return (
    <section style={{ display: 'grid', gap: 12 }}>
      <header className="of-panel" style={{ padding: 16 }}>
        <div style={{ fontSize: 11, textTransform: 'uppercase', color: 'var(--text-muted)', letterSpacing: '0.18em' }}>Storage</div>
        <h2 style={{ margin: '4px 0 0', fontSize: 18, fontWeight: 600 }}>Backing filesystem details</h2>
        <p className="of-text-muted" style={{ margin: '4px 0 0', fontSize: 13 }}>
          Foundry-style storage configuration. Visible only to users with dataset-manage permission.
        </p>
      </header>
      {loading && <div role="status" className="of-panel" style={{ padding: 16, fontSize: 13 }}>Loading storage details…</div>}
      {error && <div role="alert" className="of-status-danger" style={{ padding: 16, fontSize: 13 }}>{error}</div>}
      {details && (
        <>
          <div style={{ display: 'grid', gap: 12, gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))' }}>
            <Card label="Driver" value={details.driver.toUpperCase()} />
            <Card label="FS id" value={details.fs_id} mono />
            <Card label="Base directory" value={details.base_directory} mono />
            <Card label="Bucket / namenode" value={bucketFromFsId(details.fs_id)} mono />
          </div>
          <div style={{ display: 'grid', gap: 12, gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))' }}>
            <Card label="Active storage" value={fmtBytes(details.total_active_bytes)} />
            <Card label="Active files" value={details.total_active_files.toLocaleString()} />
            <Card label="Soft-deleted storage" value={fmtBytes(details.total_deleted_bytes)} />
          </div>
          <div className="of-panel" style={{ padding: 16, fontSize: 11, color: 'var(--text-muted)' }}>
            Presigned download URLs expire after <code>{details.presign_ttl_seconds}s</code>. Adjust via{' '}
            <code>OF_BACKING_FS_PRESIGN_TTL_SECONDS</code>.
          </div>
        </>
      )}
    </section>
  );
}
