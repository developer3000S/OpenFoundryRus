import { useEffect, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';

import { getBootstrapStatus, register } from '@api/auth';
import { useTranslator } from '@/lib/i18n/store';

export function RegisterPage() {
  const t = useTranslator();
  const navigate = useNavigate();

  const [name, setName] = useState('');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const [requiresInitialAdmin, setRequiresInitialAdmin] = useState(false);

  useEffect(() => {
    document.title = t('auth.register.title');
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
      await register({ name, email, password });
      navigate('/auth/login?registered=true', { replace: true });
    } catch (err) {
      setError(err instanceof Error ? err.message : t('auth.register.failed'));
    } finally {
      setLoading(false);
    }
  }

  return (
    <div style={{ width: '100%', maxWidth: 360 }}>
      <div style={{ textAlign: 'center', marginBottom: 24 }}>
        <span style={{ fontSize: 36, color: 'var(--status-info)' }}>◆</span>
        <h1 className="of-heading-lg" style={{ marginTop: 8 }}>
          {t('auth.register.heading')}
        </h1>
      </div>

      <form onSubmit={handleSubmit} style={{ display: 'grid', gap: 12 }}>
        {requiresInitialAdmin && (
          <div className="of-status-info" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
            {t('auth.register.bootstrapNotice')}
          </div>
        )}

        {error && (
          <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
            {error}
          </div>
        )}

        <div>
          <label htmlFor="name" style={{ display: 'block', fontSize: 13, fontWeight: 500, marginBottom: 4 }}>
            {t('auth.register.name')}
          </label>
          <input
            id="name"
            type="text"
            className="of-input"
            value={name}
            onChange={(e) => setName(e.target.value)}
            required
            placeholder={t('auth.register.namePlaceholder')}
          />
        </div>

        <div>
          <label htmlFor="email" style={{ display: 'block', fontSize: 13, fontWeight: 500, marginBottom: 4 }}>
            {t('auth.register.email')}
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
            {t('auth.register.password')}
          </label>
          <input
            id="password"
            type="password"
            className="of-input"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            required
            minLength={8}
            placeholder={t('auth.register.passwordPlaceholder')}
          />
        </div>

        <button type="submit" className="of-btn of-btn-primary" disabled={loading} style={{ width: '100%' }}>
          {loading ? t('auth.register.creating') : t('auth.register.create')}
        </button>
      </form>

      <p className="of-text-muted" style={{ textAlign: 'center', fontSize: 13, marginTop: 20 }}>
        {t('auth.register.haveAccount')}{' '}
        <Link to="/auth/login" className="of-link">
          {t('auth.register.signIn')}
        </Link>
      </p>
    </div>
  );
}
