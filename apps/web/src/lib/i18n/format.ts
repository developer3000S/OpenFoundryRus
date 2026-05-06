import type { AppLocale } from './store';

export function formatDateTime(value: string | number | Date, locale: AppLocale) {
  return new Intl.DateTimeFormat(locale, { dateStyle: 'medium', timeStyle: 'short' }).format(new Date(value));
}

export function formatNumber(value: number, locale: AppLocale) {
  return new Intl.NumberFormat(locale).format(value);
}
