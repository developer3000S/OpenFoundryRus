import type { MediaSetSchema } from '@/lib/api/mediaSets';

interface Props {
  schema: MediaSetSchema;
  size?: number;
  strokeWidth?: number;
}

const TONE: Record<MediaSetSchema, string> = {
  IMAGE: '#7c3aed',
  AUDIO: '#0ea5e9',
  VIDEO: '#ef4444',
  DOCUMENT: '#0891b2',
  SPREADSHEET: '#16a34a',
  EMAIL: '#f59e0b',
};

const PATHS: Record<MediaSetSchema, string[]> = {
  IMAGE: [
    'M4.5 5.5h15v13h-15z',
    'M4.5 15.5l4-4 4 4 3-3 4 4',
    'M9 9.5a1.4 1.4 0 1 1 0-2.8 1.4 1.4 0 0 1 0 2.8z',
  ],
  AUDIO: [
    'M9 18V8l8-3v10',
    'M9 18a2 2 0 1 1-4 0 2 2 0 0 1 4 0z',
    'M17 15a2 2 0 1 1-4 0 2 2 0 0 1 4 0z',
  ],
  VIDEO: ['M4.5 6.5h11v11h-11z', 'M15.5 9.5l4-2v9l-4-2z'],
  DOCUMENT: ['M7 4.5h7l4 4v11H7z', 'M14 4.5v4h4', 'M9 13h6', 'M9 16h6', 'M9 10h3'],
  SPREADSHEET: ['M4.5 5.5h15v13h-15z', 'M4.5 9.5h15', 'M4.5 13.5h15', 'M9.5 5.5v13', 'M14.5 5.5v13'],
  EMAIL: ['M4.5 6.5h15v11h-15z', 'M4.5 7l7.5 6 7.5-6'],
};

export function MediaSchemaIcon({ schema, size = 18, strokeWidth = 1.8 }: Props) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" xmlns="http://www.w3.org/2000/svg" aria-label={`${schema} schema icon`} data-schema={schema}>
      {PATHS[schema].map((d) => (
        <path key={d} d={d} stroke={TONE[schema]} strokeWidth={strokeWidth} strokeLinecap="round" strokeLinejoin="round" />
      ))}
    </svg>
  );
}
