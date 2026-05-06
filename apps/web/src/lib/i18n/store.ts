import { useSyncExternalStore } from 'react';

import { messages, type MessageKey } from './messages';

export const SUPPORTED_LOCALES = ['en', 'es'] as const;
export type AppLocale = (typeof SUPPORTED_LOCALES)[number];

export interface PlatformLocaleSettings {
  supported_locales: AppLocale[];
  default_locale: AppLocale;
}

export const DEFAULT_LOCALE: AppLocale = 'en';
export const LOCALE_STORAGE_KEY = 'of_locale';
export const LOCALE_COOKIE_KEY = 'of_locale';
export const PLATFORM_LOCALE_SETTINGS_KEY = 'of_platform_locale_settings';

interface I18nSnapshot {
  currentLocale: AppLocale;
  supportedLocales: AppLocale[];
  platformDefault: AppLocale;
}

let snapshot: I18nSnapshot = {
  currentLocale: DEFAULT_LOCALE,
  supportedLocales: [...SUPPORTED_LOCALES],
  platformDefault: DEFAULT_LOCALE,
};
const listeners = new Set<() => void>();

function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => listeners.delete(listener);
}

function setSnapshot(next: Partial<I18nSnapshot>) {
  snapshot = { ...snapshot, ...next };
  listeners.forEach((l) => l());
}

function isSupportedLocale(value: unknown): value is AppLocale {
  return typeof value === 'string' && SUPPORTED_LOCALES.includes(value as AppLocale);
}

export function normalizeLocale(value: unknown, fallback: AppLocale = DEFAULT_LOCALE): AppLocale {
  if (typeof value !== 'string') return fallback;
  const normalized = value.toLowerCase().trim();
  const base = normalized.split(/[-_]/)[0];
  return isSupportedLocale(base) ? base : fallback;
}

function applyDocumentLocale(locale: AppLocale) {
  if (typeof document === 'undefined') return;
  document.documentElement.lang = locale;
}

function writeLocaleCookie(locale: AppLocale) {
  if (typeof document === 'undefined') return;
  document.cookie = `${LOCALE_COOKIE_KEY}=${locale}; Path=/; Max-Age=31536000; SameSite=Lax`;
}

function setLocaleValue(locale: AppLocale, persist = true) {
  setSnapshot({ currentLocale: locale });
  applyDocumentLocale(locale);
  if (persist && typeof localStorage !== 'undefined') {
    try {
      localStorage.setItem(LOCALE_STORAGE_KEY, locale);
    } catch {
      // Ignore storage persistence failures.
    }
  }
  if (persist) {
    writeLocaleCookie(locale);
  }
}

function sanitizePlatformLocaleSettings(
  value?: Partial<PlatformLocaleSettings> | null,
): PlatformLocaleSettings {
  const supported = Array.from(
    new Set((value?.supported_locales ?? [...SUPPORTED_LOCALES]).map((locale) => normalizeLocale(locale))),
  ).filter(isSupportedLocale);
  const nextSupported = supported.length > 0 ? supported : [...SUPPORTED_LOCALES];
  const defaultLocale = normalizeLocale(value?.default_locale, nextSupported[0] ?? DEFAULT_LOCALE);
  return {
    supported_locales: nextSupported,
    default_locale: nextSupported.includes(defaultLocale) ? defaultLocale : (nextSupported[0] ?? DEFAULT_LOCALE),
  };
}

function loadPersistedPlatformLocaleSettings(): PlatformLocaleSettings | null {
  if (typeof localStorage === 'undefined') return null;
  let raw: string | null = null;
  try {
    raw = localStorage.getItem(PLATFORM_LOCALE_SETTINGS_KEY);
  } catch {
    return null;
  }
  if (!raw) return null;

  try {
    return sanitizePlatformLocaleSettings(JSON.parse(raw) as PlatformLocaleSettings);
  } catch {
    return null;
  }
}

function resolveBootstrapLocale(initialLocale?: string | null) {
  const persistedPlatform = loadPersistedPlatformLocaleSettings();
  if (persistedPlatform) {
    setSnapshot({
      supportedLocales: persistedPlatform.supported_locales,
      platformDefault: persistedPlatform.default_locale,
    });
  }

  const platformFallback = persistedPlatform?.default_locale ?? DEFAULT_LOCALE;
  const browserLocale =
    typeof navigator !== 'undefined' ? normalizeLocale(navigator.language, platformFallback) : platformFallback;
  let savedLocale: string | null = null;
  if (typeof localStorage !== 'undefined') {
    try {
      savedLocale = localStorage.getItem(LOCALE_STORAGE_KEY);
    } catch {
      savedLocale = null;
    }
  }

  return normalizeLocale(savedLocale ?? initialLocale ?? browserLocale, platformFallback);
}

export function initializeLocale(initialLocale?: string | null) {
  setLocaleValue(normalizeLocale(initialLocale, snapshot.currentLocale), false);
}

export function restoreLocale(initialLocale?: string | null) {
  setLocaleValue(resolveBootstrapLocale(initialLocale));
}

export function setLocale(locale: AppLocale) {
  setLocaleValue(normalizeLocale(locale, snapshot.platformDefault));
}

export function getUserLocalePreference(attributes: Record<string, unknown> | null | undefined) {
  const raw = attributes?.locale;
  if (typeof raw !== 'string') return null;
  const normalized = normalizeLocale(raw, DEFAULT_LOCALE);
  return raw.toLowerCase().startsWith(normalized) ? normalized : null;
}

export function applyUserLocalePreference(attributes: Record<string, unknown> | null | undefined) {
  const preferredLocale = getUserLocalePreference(attributes);
  if (preferredLocale) {
    setLocaleValue(preferredLocale);
  }
}

export function translate(
  key: MessageKey,
  params?: Record<string, string | number>,
  locale: AppLocale = snapshot.currentLocale,
) {
  const template = (messages[locale][key] ?? messages[DEFAULT_LOCALE][key] ?? key) as string;
  if (!params) return template;
  return Object.entries(params).reduce(
    (result, [name, value]) => result.replaceAll(`{${name}}`, String(value)),
    template,
  );
}

export function createTranslator(locale: AppLocale) {
  return (key: MessageKey, params?: Record<string, string | number>) => translate(key, params, locale);
}

export function getLocaleLabel(locale: AppLocale, displayLocale: AppLocale = snapshot.currentLocale) {
  return (displayLocale === 'es'
    ? locale === 'en'
      ? messages.es['locale.english']
      : messages.es['locale.spanish']
    : locale === 'en'
      ? messages.en['locale.english']
      : messages.en['locale.spanish']) as string;
}

export function useCurrentLocale(): AppLocale {
  return useSyncExternalStore(subscribe, () => snapshot.currentLocale, () => snapshot.currentLocale);
}

export function useSupportedLocales(): AppLocale[] {
  return useSyncExternalStore(subscribe, () => snapshot.supportedLocales, () => snapshot.supportedLocales);
}

export function useTranslator() {
  const locale = useCurrentLocale();
  return createTranslator(locale);
}
