import { useEffect, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';

import { getBootstrapStatus, listPublicSsoProviders, type PublicSsoProvider } from '@api/auth';
import { auth } from '@stores/auth';
import { useTranslator } from '@/lib/i18n/store';

export function LoginPage() {
  const t = useTranslator();
  const navigate = useNavigate();

  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const [providers, setProviders] = useState<PublicSsoProvider[]>([]);
  const [requiresInitialAdmin, setRequiresInitialAdmin] = useState(false);

  useEffect(() => {
    document.title = t('auth.login.title');
  }, [t]);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        const status = await getBootstrapStatus();
        if (!cancelled) setRequiresInitialAdmin(status.requires_initial_admin);
      } catch {
        if (!cancelled) setRequiresInitialAdmin(false);
      }
      try {
        const list = await listPublicSsoProviders();
        if (!cancelled) setProviders(list);
      } catch {
        if (!cancelled) setProviders([]);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError('');
    setLoading(true);
    try {
      const result = await auth.login(email, password);
      navigate(result.status === 'mfa_required' ? '/auth/mfa' : '/', { replace: true });
    } catch (err) {
      setError(err instanceof Error ? err.message : t('auth.login.failed'));
    } finally {
      setLoading(false);
    }
  }

  async function handleSsoLogin(slug: string) {
    setError('');
    try {
      await auth.startSsoLogin(slug);
    } catch (err) {
      setError(err instanceof Error ? err.message : t('auth.login.ssoFailed'));
    }
  }

  return (
    <div style={{ width: '100%', maxWidth: 360 }}>
      <div style={{ textAlign: 'center', marginBottom: 24 }}>
        <span style={{ fontSize: 36, color: 'var(--status-info)' }}>◆</span>
        <h1 className="of-heading-lg" style={{ marginTop: 8 }}>
          {t('auth.login.heading')}
        </h1>
      </div>

      <form onSubmit={handleSubmit} style={{ display: 'grid', gap: 12 }}>
        {requiresInitialAdmin && (
          <div className="of-status-info" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
            {t('auth.login.bootstrapNotice')}
          </div>
        )}

        {error && (
          <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
            {error}
          </div>
        )}

        <div>
          <label htmlFor="email" style={{ display: 'block', fontSize: 13, fontWeight: 500, marginBottom: 4 }}>
            {t('auth.login.email')}
          </label>
          <input
            id="email"
            type="email"
            className="of-input"
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            required
            placeholder={t('auth.login.emailPlaceholder')}
          />
        </div>

        <div>
          <label htmlFor="password" style={{ display: 'block', fontSize: 13, fontWeight: 500, marginBottom: 4 }}>
            {t('auth.login.password')}
          </label>
          <input
            id="password"
            type="password"
            className="of-input"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
            placeholder={t('auth.login.passwordPlaceholder')}
          />
        </div>

        <button type="submit" className="of-btn of-btn-primary" disabled={loading} style={{ width: '100%' }}>
          {loading ? t('auth.login.signingIn') : t('auth.login.signIn')}
        </button>

        {providers.length > 0 && (
          <div style={{ display: 'grid', gap: 8, paddingTop: 8 }}>
            <div className="of-eyebrow">{t('auth.login.sso')}</div>
            {providers.map((provider) => (
              <button
                key={provider.id}
                type="button"
                className="of-btn"
                style={{ width: '100%' }}
                onClick={() => handleSsoLogin(provider.slug)}
              >
                {t('auth.login.continueWith', { provider: provider.name })}
              </button>
            ))}
          </div>
        )}
      </form>

      <p className="of-text-muted" style={{ textAlign: 'center', fontSize: 13, marginTop: 20 }}>
        {requiresInitialAdmin ? t('auth.login.bootstrapCta') : t('auth.login.noAccount')}{' '}
        <Link to="/auth/register" className="of-link">
          {t('auth.login.register')}
        </Link>
      </p>
    </div>
  );
}
