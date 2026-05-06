import { useEffect, useMemo, useState } from 'react';

interface JsonEditorProps {
  value: string;
  onChange: (value: string) => void;
  label?: string;
  minHeight?: number;
  placeholder?: string;
  disabled?: boolean;
  /** When provided, validates parsed JSON against this predicate. */
  validate?: (parsed: unknown) => string | null;
}

export function JsonEditor({
  value,
  onChange,
  label,
  minHeight = 140,
  placeholder,
  disabled,
  validate,
}: JsonEditorProps) {
  const [touched, setTouched] = useState(false);

  const parseError = useMemo(() => {
    const trimmed = value.trim();
    if (!trimmed) return null;
    try {
      const parsed = JSON.parse(trimmed) as unknown;
      if (validate) return validate(parsed);
      return null;
    } catch (cause) {
      return cause instanceof Error ? cause.message : 'Invalid JSON';
    }
  }, [value, validate]);

  useEffect(() => {
    if (value && !touched) setTouched(true);
  }, [value, touched]);

  function format() {
    try {
      const parsed = JSON.parse(value);
      onChange(JSON.stringify(parsed, null, 2));
    } catch {
      // ignore — keep raw value if it's not valid JSON
    }
  }

  return (
    <div style={{ display: 'grid', gap: 4 }}>
      {label && (
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'baseline' }}>
          <label style={{ fontSize: 13 }}>{label}</label>
          <button
            type="button"
            onClick={format}
            disabled={disabled || !value.trim() || Boolean(parseError)}
            style={{
              fontSize: 11,
              padding: '2px 8px',
              border: 'none',
              background: 'transparent',
              color: 'var(--text-muted)',
              cursor: 'pointer',
            }}
            title="Reformat JSON"
          >
            format
          </button>
        </div>
      )}
      <textarea
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        disabled={disabled}
        className="of-input"
        style={{
          fontFamily: 'var(--font-mono)',
          fontSize: 11,
          minHeight,
          borderColor: parseError && touched ? '#fca5a5' : undefined,
        }}
      />
      {parseError && touched && (
        <p style={{ fontSize: 11, color: '#b91c1c', margin: 0 }}>{parseError}</p>
      )}
    </div>
  );
}

/** Helper: parse JSON string with a typed fallback. */
export function parseJsonOr<T>(text: string, fallback: T): T {
  const trimmed = text.trim();
  if (!trimmed) return fallback;
  try {
    return JSON.parse(trimmed) as T;
  } catch {
    return fallback;
  }
}
