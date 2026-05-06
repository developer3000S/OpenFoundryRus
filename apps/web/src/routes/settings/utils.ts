export function toOptionalString(value: string) {
  const trimmed = value.trim();
  return trimmed ? trimmed : null;
}

export function toList(value: string) {
  return value
    .split(/[\n,]/)
    .map((entry) => entry.trim())
    .filter(Boolean);
}

export function parseJson(value: string): Record<string, unknown> {
  return value.trim() ? JSON.parse(value) : {};
}

export function toScopes(value: string) {
  return value
    .split(',')
    .map((entry) => entry.trim())
    .filter(Boolean);
}

export function toIsoDateTime(value: string) {
  return value ? new Date(value).toISOString() : null;
}
