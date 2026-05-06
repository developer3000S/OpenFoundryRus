import { useEffect, useMemo, useState } from 'react';

import { previewView, type DatasetTransaction, type ViewPreviewResponse } from '@/lib/api/datasets';
import { VirtualizedPreviewTable, type ColumnDef } from './VirtualizedPreviewTable';

interface DataPreviewProps {
  datasetRid: string;
  viewId: string;
  transactions: DatasetTransaction[];
  selectedTransactionId?: string | null;
  onSelectTransaction?: (txId: string | null) => void;
  formatOverride?: 'auto' | 'parquet' | 'avro' | 'text';
  csvOverrides?: {
    delimiter?: string;
    quote?: string;
    escape?: string;
    header?: boolean;
    null_value?: string;
    charset?: string;
    date_format?: string;
    timestamp_format?: string;
    csv?: boolean;
  };
  rowHeight?: number;
  viewportHeight?: number;
  limit?: number;
}

export function DataPreview({
  datasetRid,
  viewId,
  transactions,
  selectedTransactionId = null,
  onSelectTransaction,
  formatOverride = 'auto',
  csvOverrides,
  rowHeight = 32,
  viewportHeight = 480,
  limit = 1000,
}: DataPreviewProps) {
  const [response, setResponse] = useState<ViewPreviewResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!datasetRid || !viewId) return;
    let cancelled = false;
    setLoading(true);
    setError(null);
    previewView(datasetRid, viewId, {
      limit,
      offset: 0,
      format: formatOverride,
      csv_delimiter: csvOverrides?.delimiter,
      csv_quote: csvOverrides?.quote,
      csv_escape: csvOverrides?.escape,
      csv_header: csvOverrides?.header,
      csv_null_value: csvOverrides?.null_value,
      csv_charset: csvOverrides?.charset,
      csv_date_format: csvOverrides?.date_format,
      csv_timestamp_format: csvOverrides?.timestamp_format,
      csv: csvOverrides?.csv,
    } as Parameters<typeof previewView>[2])
      .then((r) => { if (!cancelled) setResponse(r); })
      .catch((cause: unknown) => {
        if (!cancelled) {
          setError(cause instanceof Error ? cause.message : 'Preview failed.');
          setResponse(null);
        }
      })
      .finally(() => { if (!cancelled) setLoading(false); });
    return () => { cancelled = true; };
  }, [datasetRid, viewId, formatOverride, JSON.stringify(csvOverrides), limit]);

  const columns: ColumnDef[] = useMemo(
    () => (response?.columns ?? []).map((col) => ({ name: col.name, field_type: col.field_type })),
    [response],
  );
  const rows = response?.rows ?? [];

  if (loading) {
    return (
      <div role="status" className="of-panel" style={{ padding: 16, fontSize: 13 }}>
        Loading preview…
      </div>
    );
  }
  if (error) {
    return (
      <div role="alert" className="of-status-danger" style={{ padding: 16, fontSize: 13 }}>
        {error}
      </div>
    );
  }
  if (!response) return null;

  return (
    <VirtualizedPreviewTable
      columns={columns}
      rows={rows}
      transactions={transactions}
      selectedTransactionId={selectedTransactionId}
      onSelectTransaction={onSelectTransaction}
      rowHeight={rowHeight}
      viewportHeight={viewportHeight}
      fileFormat={response.file_format}
      textSubFormat={response.text_sub_format}
      schemaInferred={response.schema_inferred}
    />
  );
}
