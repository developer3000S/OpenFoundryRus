import { useEffect, useState } from 'react';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';

import { auth } from '@stores/auth';
import { useTranslator } from '@/lib/i18n/store';

export function CallbackPage() {
  const t = useTranslator();
  const navigate = useNavigate();
  const [params] = useSearchParams();
  const [error, setError] = useState('');

  useEffect(() => {
    document.title = t('auth.callback.title');
  }, [t]);

  useEffect(() => {
    const code = params.get('code');
    const state = params.get('state');
    const samlResponse = params.get('SAMLResponse');
    const relayState = params.get('RelayState');

    if ((!code || !state) && (!samlResponse || !relayState)) {
      setError(t('auth.callback.missing'));
      return;
    }

    let cancelled = false;
    (async () => {
      try {
        const result = await auth.handleSsoCallback({
          code: code ?? undefined,
          state: state ?? undefined,
          saml_response: samlResponse ?? undefined,
          relay_state: relayState ?? undefined,
        });
        if (!cancelled) {
          navigate(result.status === 'mfa_required' ? '/auth/mfa' : '/', { replace: true });
        }
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : t('auth.callback.failed'));
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [params, navigate, t]);

  return (
    <div className="of-panel" style={{ width: '100%', maxWidth: 420, padding: 28, textAlign: 'center' }}>
      <p className="of-eyebrow">{t('auth.callback.badge')}</p>
      <h1 className="of-heading-lg" style={{ marginTop: 6 }}>
        {t('auth.callback.heading')}
      </h1>
      {error ? (
        <>
          <p
            className="of-status-danger"
            style={{ marginTop: 16, padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}
          >
            {error}
          </p>
          <Link
            to="/auth/login"
            className="of-link"
            style={{ display: 'inline-block', marginTop: 24, fontSize: 13 }}
          >
            {t('auth.callback.back')}
          </Link>
        </>
      ) : (
        <p className="of-text-muted" style={{ marginTop: 16, fontSize: 13 }}>
          {t('auth.callback.subtitle')}
        </p>
      )}
    </div>
  );
}
