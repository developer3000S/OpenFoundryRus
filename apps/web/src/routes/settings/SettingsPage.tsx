import { useEffect, useState } from 'react';

import { updateUser } from '@api/auth';
import { auth, useCurrentUser, useRequireAuth } from '@stores/auth';
import {
  getLocaleLabel,
  setLocale,
  useCurrentLocale,
  useSupportedLocales,
  useTranslator,
  type AppLocale,
} from '@/lib/i18n/store';

import { ApiKeysSection } from './ApiKeysSection';
import { GroupsSection } from './GroupsSection';
import { MfaSection } from './MfaSection';
import { PermissionsSection } from './PermissionsSection';
import { PoliciesSection } from './PoliciesSection';
import { RestrictedViewsSection } from './RestrictedViewsSection';
import { RolesSection } from './RolesSection';
import { SsoProvidersSection } from './SsoProvidersSection';
import { UsersSection } from './UsersSection';

export function SettingsPage() {
  const t = useTranslator();
  useRequireAuth();
  const currentUser = useCurrentUser();
  const currentLocale = useCurrentLocale();
  const supportedLocales = useSupportedLocales();

  const [selectedLocale, setSelectedLocale] = useState<AppLocale>(currentLocale);
  const [savingLanguage, setSavingLanguage] = useState(false);
  const [notice, setNotice] = useState('');
  const [error, setError] = useState('');

  useEffect(() => {
    setSelectedLocale(currentLocale);
  }, [currentLocale]);

  async function handleSaveLanguagePreference() {
    if (!currentUser) return;
    setSavingLanguage(true);
    setError('');
    setNotice('');
    try {
      const updated = await updateUser(currentUser.id, {
        attributes: { ...(currentUser.attributes ?? {}), locale: selectedLocale },
      });
      auth.updateCurrentUserProfile(updated);
      setLocale(selectedLocale);
      setNotice(t('settings.language.saved'));
    } catch (err) {
      setError(err instanceof Error ? err.message : t('settings.language.failed'));
    } finally {
      setSavingLanguage(false);
    }
  }

  return (
    <section className="of-page" style={{ display: 'grid', gap: 16 }}>
      <header>
        <p className="of-eyebrow">Settings</p>
        <h1 className="of-heading-xl">Workspace preferences</h1>
      </header>

      {error && <div className="of-inline-note">{error}</div>}
      {notice && (
        <div className="of-panel" style={{ padding: '10px 12px', color: 'var(--status-success)' }}>
          {notice}
        </div>
      )}

      <div className="of-panel" style={{ padding: 24 }}>
        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 16, alignItems: 'flex-end', justifyContent: 'space-between' }}>
          <div>
            <p className="of-eyebrow">{t('settings.language.badge')}</p>
            <h2 className="of-heading-lg">{t('settings.language.heading')}</h2>
            <p className="of-text-muted" style={{ marginTop: 8, maxWidth: 640 }}>
              {t('settings.language.description')}
            </p>
          </div>
          <button
            type="button"
            className="of-btn of-btn-primary"
            onClick={handleSaveLanguagePreference}
            disabled={savingLanguage || !currentUser}
          >
            {savingLanguage ? t('common.saving') : t('settings.language.save')}
          </button>
        </div>

        <div style={{ display: 'grid', gridTemplateColumns: 'minmax(0, 22rem) 1fr', gap: 16, marginTop: 24 }}>
          <label style={{ display: 'block', fontSize: 13 }}>
            <span style={{ display: 'block', fontWeight: 500, marginBottom: 8 }}>
              {t('settings.language.selectLabel')}
            </span>
            <select
              className="of-select"
              value={selectedLocale}
              onChange={(e) => setSelectedLocale(e.target.value as AppLocale)}
            >
              {supportedLocales.map((locale) => (
                <option key={locale} value={locale}>
                  {getLocaleLabel(locale, currentLocale)}
                </option>
              ))}
            </select>
          </label>
          <div className="of-panel-muted" style={{ padding: '10px 14px' }}>
            <span className="of-text-muted">{t('settings.language.help')}</span>
          </div>
        </div>

      </div>

      <UsersSection setNotice={setNotice} setError={setError} />

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'repeat(auto-fit, minmax(420px, 1fr))' }}>
        <RolesSection setNotice={setNotice} setError={setError} />
        <GroupsSection setNotice={setNotice} setError={setError} />
      </div>

      <PermissionsSection setNotice={setNotice} setError={setError} />

      <PoliciesSection setNotice={setNotice} setError={setError} />

      <RestrictedViewsSection setNotice={setNotice} setError={setError} />

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'repeat(auto-fit, minmax(420px, 1fr))' }}>
        <MfaSection setNotice={setNotice} setError={setError} />
        <ApiKeysSection setNotice={setNotice} setError={setError} />
      </div>

      <SsoProvidersSection setNotice={setNotice} setError={setError} />
    </section>
  );
}
