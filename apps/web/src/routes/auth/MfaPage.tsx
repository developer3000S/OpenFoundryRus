import { useEffect, useRef, useState } from 'react';
import { useNavigate } from 'react-router-dom';

import { auth, usePendingMfaChallenge } from '@stores/auth';
import { useTranslator } from '@/lib/i18n/store';

export function MfaPage() {
  const t = useTranslator();
  const navigate = useNavigate();
  const pendingChallenge = usePendingMfaChallenge();

  const [code, setCode] = useState('');
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(false);
  const inputRef = useRef<HTMLInputElement | null>(null);

  useEffect(() => {
    document.title = t('auth.mfa.title');
  }, [t]);

  useEffect(() => {
    if (!pendingChallenge) {
      navigate('/auth/login', { replace: true });
      return;
    }
    inputRef.current?.focus();
  }, [pendingChallenge, navigate]);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    setError('');
    setLoading(true);
    try {
      await auth.completeMfa(code);
      navigate('/', { replace: true });
    } catch (err) {
      setError(err instanceof Error ? err.message : t('auth.mfa.failed'));
    } finally {
      setLoading(false);
    }
  }

  return (
    <div className="of-panel" style={{ width: '100%', maxWidth: 360, padding: 28 }}>
      <header style={{ marginBottom: 20 }}>
        <p className="of-eyebrow">{t('auth.mfa.badge')}</p>
        <h1 className="of-heading-lg" style={{ marginTop: 6 }}>
          {t('auth.mfa.heading')}
        </h1>
        <p className="of-text-muted" style={{ marginTop: 6, fontSize: 13 }}>
          {t('auth.mfa.subtitle')}
        </p>
      </header>

      <form onSubmit={handleSubmit} style={{ display: 'grid', gap: 12 }}>
        {error && (
          <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
            {error}
          </div>
        )}

        <div>
          <label htmlFor="mfa-code" style={{ display: 'block', fontSize: 13, fontWeight: 500, marginBottom: 4 }}>
            {t('auth.mfa.code')}
          </label>
          <input
            id="mfa-code"
            ref={inputRef}
            type="text"
            className="of-input"
            value={code}
            onChange={(e) => setCode(e.target.value)}
            required
            placeholder={t('auth.mfa.placeholder')}
            style={{ letterSpacing: '0.35em', textTransform: 'uppercase' }}
          />
        </div>

        <button type="submit" className="of-btn of-btn-primary" disabled={loading || !code} style={{ width: '100%' }}>
          {loading ? t('auth.mfa.verifying') : t('auth.mfa.verify')}
        </button>
      </form>
    </div>
  );
}
