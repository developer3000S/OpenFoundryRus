type Operation = 'append' | 'overwrite' | 'delete' | 'replace';

interface Props {
  operation: Operation;
  foundry_equivalent?: 'APPEND' | 'UPDATE' | 'SNAPSHOT' | 'DELETE' | 'INTERNAL_NOOP';
  overwrite_kind?: 'full' | 'partial';
}

const TONES: Record<Operation, string> = {
  append: 'bg-blue-100 text-blue-800 border-blue-200 dark:bg-blue-950 dark:text-blue-300 dark:border-blue-900',
  overwrite: 'bg-orange-100 text-orange-800 border-orange-200 dark:bg-orange-950 dark:text-orange-300 dark:border-orange-900',
  delete: 'bg-red-100 text-red-800 border-red-200 dark:bg-red-950 dark:text-red-300 dark:border-red-900',
  replace: 'bg-gray-100 text-gray-800 border-gray-200 dark:bg-gray-800 dark:text-gray-300 dark:border-gray-700',
};

export function SnapshotBadge({ operation, foundry_equivalent, overwrite_kind }: Props) {
  const foundry = foundry_equivalent ?? (
    operation === 'append' ? 'APPEND' :
    operation === 'delete' ? 'DELETE' :
    operation === 'overwrite' ? (overwrite_kind === 'full' ? 'SNAPSHOT' : 'UPDATE') :
    'INTERNAL_NOOP'
  );
  const tooltip = `Iceberg ${operation} corresponds to Foundry ${foundry === 'INTERNAL_NOOP' ? '(maintenance — no equivalent)' : foundry}`;

  return (
    <span className={`inline-flex items-center rounded border px-1.5 py-0.5 text-xs font-semibold ${TONES[operation]}`} title={tooltip}>
      {operation}
      <span className="ml-1 text-[0.65rem] opacity-70">→ {foundry}</span>
    </span>
  );
}
