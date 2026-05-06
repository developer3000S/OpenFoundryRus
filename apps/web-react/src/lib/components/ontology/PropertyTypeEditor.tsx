import type { MediaSet } from '@/lib/api/mediaSets';

export type PropertyType =
  | 'string' | 'integer' | 'float' | 'boolean' | 'date' | 'timestamp' | 'json'
  | 'array' | 'vector' | 'reference' | 'geo_point' | 'media_reference' | 'struct' | 'attachment';

interface PropertyTypeEditorProps {
  propertyType: PropertyType;
  onChange: (next: PropertyType) => void;
  backingMediaSetRid?: string | null;
  onBackingChange?: (rid: string | null) => void;
  mediaSetCatalog?: MediaSet[];
}

const OPTIONS: { value: PropertyType; label: string }[] = [
  { value: 'string', label: 'String' },
  { value: 'integer', label: 'Integer' },
  { value: 'float', label: 'Float' },
  { value: 'boolean', label: 'Boolean' },
  { value: 'date', label: 'Date' },
  { value: 'timestamp', label: 'Timestamp' },
  { value: 'json', label: 'JSON' },
  { value: 'array', label: 'Array' },
  { value: 'vector', label: 'Vector (embedding)' },
  { value: 'reference', label: 'Object reference' },
  { value: 'geo_point', label: 'Geo point' },
  { value: 'media_reference', label: 'Media reference' },
  { value: 'struct', label: 'Struct' },
  { value: 'attachment', label: 'Attachment' },
];

export function PropertyTypeEditor({
  propertyType,
  onChange,
  backingMediaSetRid = null,
  onBackingChange = () => {},
  mediaSetCatalog = [],
}: PropertyTypeEditorProps) {
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
      <label style={{ fontSize: 12 }}>
        Property type
        <select
          value={propertyType}
          onChange={(e) => {
            const next = e.target.value as PropertyType;
            onChange(next);
            if (next !== 'media_reference') onBackingChange(null);
          }}
          className="of-input"
          style={{ marginTop: 4 }}
        >
          {OPTIONS.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
        </select>
      </label>
      {propertyType === 'media_reference' && (
        <label style={{ fontSize: 12 }}>
          Backing media set
          <select
            value={backingMediaSetRid ?? ''}
            onChange={(e) => onBackingChange(e.target.value || null)}
            className="of-input"
            style={{ marginTop: 4, fontFamily: 'var(--font-mono)' }}
          >
            <option value="">— pick —</option>
            {mediaSetCatalog.map((ms) => <option key={ms.rid} value={ms.rid}>{ms.name} ({ms.rid.slice(-12)})</option>)}
          </select>
        </label>
      )}
    </div>
  );
}
