// Pipeline Builder transform stack — block schema and SQL composition.
//
// The stack is the in-memory shape of a chain of transforms applied to
// a single source dataset. Each block has a config; serialising the
// stack down to a single SQL string yields the body of an SQL transform
// node which is what the rest of the pipeline DAG already understands.
//
// The editor lets the user reorder, edit, and apply the blocks, and the
// Apply-all action persists the stack to a downstream transform node.

export type CastTargetType = 'Timestamp' | 'Date' | 'String' | 'Integer' | 'Long' | 'Double' | 'Boolean';

export type FilterOperator =
  | 'is_null'
  | 'is_not_null'
  | 'equals'
  | 'not_equals'
  | 'greater_than'
  | 'less_than';

export interface CastBlock {
  id: string;
  kind: 'cast';
  applied: boolean;
  source_column: string;
  target_type: CastTargetType;
  target_column: string;
}

export interface FilterCondition {
  column: string;
  operator: FilterOperator;
  value: string;
  treat_empty_as_null: boolean;
}

export interface FilterBlock {
  id: string;
  kind: 'filter';
  applied: boolean;
  mode: 'keep' | 'drop';
  match: 'all' | 'any';
  conditions: FilterCondition[];
}

export interface DropColumnsBlock {
  id: string;
  kind: 'drop';
  applied: boolean;
  columns: string[];
}

export interface SelectColumnsBlock {
  id: string;
  kind: 'select';
  applied: boolean;
  columns: string[];
}

export interface RenameColumnsBlock {
  id: string;
  kind: 'rename';
  applied: boolean;
  renames: Array<{ from: string; to: string }>;
}

export interface NormalizeColumnsBlock {
  id: string;
  kind: 'normalize';
  applied: boolean;
  remove_special_characters: boolean;
}

export interface HaversineDistanceBlock {
  id: string;
  kind: 'haversine_distance';
  applied: boolean;
  start_lat_column: string;
  start_lon_column: string;
  end_lat_column: string;
  end_lon_column: string;
  unit: 'miles' | 'km' | 'meters';
  target_column: string;
}

export type TransformBlock =
  | CastBlock
  | FilterBlock
  | SelectColumnsBlock
  | DropColumnsBlock
  | RenameColumnsBlock
  | NormalizeColumnsBlock
  | HaversineDistanceBlock;

export interface TransformStack {
  source_dataset_id: string;
  source_dataset_name: string;
  display_name: string;
  blocks: TransformBlock[];
}

export const FILTER_OPERATORS: Array<{ id: FilterOperator; label: string }> = [
  { id: 'is_not_null', label: 'is not null' },
  { id: 'is_null', label: 'is null' },
  { id: 'equals', label: 'equals' },
  { id: 'not_equals', label: 'does not equal' },
  { id: 'greater_than', label: '>' },
  { id: 'less_than', label: '<' },
];

export const CAST_TYPES: CastTargetType[] = ['Timestamp', 'Date', 'String', 'Integer', 'Long', 'Double', 'Boolean'];

export function newBlockId(prefix: string): string {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) return crypto.randomUUID();
  return `${prefix}_${Date.now()}_${Math.floor(Math.random() * 1e6)}`;
}

export function newCastBlock(): CastBlock {
  return {
    id: newBlockId('cast'),
    kind: 'cast',
    applied: false,
    source_column: '',
    target_type: 'Timestamp',
    target_column: '',
  };
}

export function newFilterBlock(): FilterBlock {
  return {
    id: newBlockId('filter'),
    kind: 'filter',
    applied: false,
    mode: 'keep',
    match: 'all',
    conditions: [
      { column: '', operator: 'is_not_null', value: '', treat_empty_as_null: true },
    ],
  };
}

export function newDropColumnsBlock(): DropColumnsBlock {
  return { id: newBlockId('drop'), kind: 'drop', applied: false, columns: [] };
}

export function newSelectColumnsBlock(): SelectColumnsBlock {
  return { id: newBlockId('select'), kind: 'select', applied: false, columns: [] };
}

export function newRenameColumnsBlock(): RenameColumnsBlock {
  return {
    id: newBlockId('rename'),
    kind: 'rename',
    applied: false,
    renames: [{ from: '', to: '' }],
  };
}

export function newNormalizeColumnsBlock(): NormalizeColumnsBlock {
  return {
    id: newBlockId('normalize'),
    kind: 'normalize',
    applied: false,
    remove_special_characters: false,
  };
}

export function newHaversineDistanceBlock(): HaversineDistanceBlock {
  return {
    id: newBlockId('haversine_distance'),
    kind: 'haversine_distance',
    applied: false,
    start_lat_column: '',
    start_lon_column: '',
    end_lat_column: '',
    end_lon_column: '',
    unit: 'miles',
    target_column: 'distance_miles',
  };
}

export function blockTitle(block: TransformBlock): string {
  switch (block.kind) {
    case 'cast':
      return 'Cast to Timestamp';
    case 'filter':
      return 'Filter';
    case 'select':
      return 'Select columns';
    case 'drop':
      return 'Drop columns';
    case 'rename':
      return 'Rename columns';
    case 'normalize':
      return 'Normalize column names';
    case 'haversine_distance':
      return 'Haversine distance';
  }
}

export function blockSectionLabel(kind: TransformBlock['kind']): string {
  switch (kind) {
    case 'cast':
      return 'Cast to Timestamp';
    case 'filter':
      return 'Filter';
    case 'select':
      return 'Select columns';
    case 'drop':
      return 'Drop columns';
    case 'rename':
      return 'Rename columns';
    case 'normalize':
      return 'Normalize column names';
    case 'haversine_distance':
      return 'Haversine distance';
  }
}

function quoteIdent(name: string): string {
  return `"${name.replace(/"/g, '""')}"`;
}

function snakeCase(name: string, removeSpecial: boolean): string {
  let next = name
    .replace(/([a-z0-9])([A-Z])/g, '$1_$2')
    .replace(/[\s-]+/g, '_')
    .toLowerCase();
  if (removeSpecial) next = next.replace(/[^a-z0-9_]/g, '');
  return next;
}

function castSqlType(type: CastTargetType): string {
  switch (type) {
    case 'Timestamp':
      return 'TIMESTAMP';
    case 'Date':
      return 'DATE';
    case 'String':
      return 'VARCHAR';
    case 'Integer':
      return 'INTEGER';
    case 'Long':
      return 'BIGINT';
    case 'Double':
      return 'DOUBLE';
    case 'Boolean':
      return 'BOOLEAN';
  }
}

function filterConditionSql(condition: FilterCondition): string {
  const col = quoteIdent(condition.column);
  switch (condition.operator) {
    case 'is_null':
      return condition.treat_empty_as_null ? `(${col} IS NULL OR ${col} = '')` : `${col} IS NULL`;
    case 'is_not_null':
      return condition.treat_empty_as_null
        ? `(${col} IS NOT NULL AND ${col} <> '')`
        : `${col} IS NOT NULL`;
    case 'equals':
      return `${col} = '${condition.value.replace(/'/g, "''")}'`;
    case 'not_equals':
      return `${col} <> '${condition.value.replace(/'/g, "''")}'`;
    case 'greater_than':
      return `${col} > '${condition.value.replace(/'/g, "''")}'`;
    case 'less_than':
      return `${col} < '${condition.value.replace(/'/g, "''")}'`;
  }
}

// Compose the entire stack into a single SQL string. Each block becomes
// a CTE so users can read/audit the chain. The reference name for the
// source dataset table is `__source__` — the runtime injects it.
export function composeTransformStackSql(stack: TransformStack): string {
  const ctes: string[] = [];
  const blocks = stack.blocks.filter((block) => block.applied);

  let previous = '__source__';
  blocks.forEach((block, index) => {
    const cte = `t${index}`;
    let sql: string;
    switch (block.kind) {
      case 'cast': {
        if (!block.source_column) break;
        const target = block.target_column || block.source_column;
        const otherColumns = `* EXCEPT (${quoteIdent(block.source_column)})`;
        sql = `SELECT ${otherColumns}, CAST(${quoteIdent(block.source_column)} AS ${castSqlType(block.target_type)}) AS ${quoteIdent(target)} FROM ${previous}`;
        ctes.push(`${cte} AS (${sql})`);
        previous = cte;
        return;
      }
      case 'filter': {
        const conditions = block.conditions
          .filter((cond) => cond.column)
          .map(filterConditionSql);
        if (conditions.length === 0) break;
        const joiner = block.match === 'all' ? ' AND ' : ' OR ';
        const where = conditions.join(joiner);
        const negated = block.mode === 'drop' ? `NOT (${where})` : where;
        sql = `SELECT * FROM ${previous} WHERE ${negated}`;
        ctes.push(`${cte} AS (${sql})`);
        previous = cte;
        return;
      }
      case 'select': {
        if (block.columns.length === 0) break;
        const projection = block.columns.map(quoteIdent).join(', ');
        sql = `SELECT ${projection} FROM ${previous}`;
        ctes.push(`${cte} AS (${sql})`);
        previous = cte;
        return;
      }
      case 'drop': {
        if (block.columns.length === 0) break;
        const exceptList = block.columns.map(quoteIdent).join(', ');
        sql = `SELECT * EXCEPT (${exceptList}) FROM ${previous}`;
        ctes.push(`${cte} AS (${sql})`);
        previous = cte;
        return;
      }
      case 'rename': {
        const valid = block.renames.filter((entry) => entry.from && entry.to);
        if (valid.length === 0) break;
        const projections = valid
          .map((entry) => `${quoteIdent(entry.from)} AS ${quoteIdent(entry.to)}`)
          .join(', ');
        const removed = valid.map((entry) => quoteIdent(entry.from)).join(', ');
        sql = `SELECT * EXCEPT (${removed}), ${projections} FROM ${previous}`;
        ctes.push(`${cte} AS (${sql})`);
        previous = cte;
        return;
      }
      case 'normalize': {
        // Normalize is metadata-only at this layer — runtime renames columns
        // when materialising. Carry it as a comment for the audit trail.
        sql = `SELECT * FROM ${previous} /* normalize_column_names${block.remove_special_characters ? '_strict' : ''} */`;
        ctes.push(`${cte} AS (${sql})`);
        previous = cte;
        return;
      }
      case 'haversine_distance': {
        if (!block.start_lat_column || !block.start_lon_column || !block.end_lat_column || !block.end_lon_column || !block.target_column) break;
        sql = `SELECT *, haversine_distance(${quoteIdent(block.start_lat_column)}, ${quoteIdent(block.start_lon_column)}, ${quoteIdent(block.end_lat_column)}, ${quoteIdent(block.end_lon_column)}, '${block.unit}') AS ${quoteIdent(block.target_column)} FROM ${previous}`;
        ctes.push(`${cte} AS (${sql})`);
        previous = cte;
        return;
      }
    }
  });

  if (ctes.length === 0) return `SELECT * FROM __source__`;
  return `WITH ${ctes.join(',\n     ')}\nSELECT * FROM ${previous}`;
}

// Apply normalize-column-names to a list of column names. Used by the
// preview pane to show what the result schema looks like.
export function normaliseColumnList(columns: string[], removeSpecial: boolean): string[] {
  return columns.map((name) => snakeCase(name, removeSpecial));
}
