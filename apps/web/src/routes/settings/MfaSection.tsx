import { useState } from 'react';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { disableMfa, enrollMfa, verifyMfaSetup, type MfaEnrollmentResponse } from '@api/auth';
import { mfaQuery, settingsQueryKeys } from './queries';

interface MfaSectionProps {
  setNotice: (msg: string) => void;
  setError: (msg: string) => void;
}

export function MfaSection({ setNotice, setError }: MfaSectionProps) {
  const qc = useQueryClient();

  const status = useQuery(mfaQuery);
  const mfaStatus = status.data;

  const [enrollment, setEnrollment] = useState<MfaEnrollmentResponse | null>(null);
  const [verifyCode, setVerifyCode] = useState('');
  const [disableCode, setDisableCode] = useState('');

  const enrollMutation = useMutation({
    mutationFn: () => enrollMfa(),
    onSuccess: async (data) => {
      setEnrollment(data);
      await qc.invalidateQueries({ queryKey: settingsQueryKeys.mfa });
      setNotice('MFA enrollment secret generated.');
    },
    onError: (err) => setError(err instanceof Error ? err.message : 'Failed to start enrollment'),
  });

  const verifyMutation = useMutation({
    mutationFn: () => verifyMfaSetup({ code: verifyCode }),
    onSuccess: async () => {
      setVerifyCode('');
      await qc.invalidateQueries({ queryKey: settingsQueryKeys.mfa });
      setNotice('MFA enabled.');
    },
    onError: (err) => setError(err instanceof Error ? err.message : 'Failed to verify code'),
  });

  const disableMutation = useMutation({
    mutationFn: () => disableMfa({ code: disableCode }),
    onSuccess: async () => {
      setDisableCode('');
      setEnrollment(null);
      await qc.invalidateQueries({ queryKey: settingsQueryKeys.mfa });
      setNotice('MFA disabled.');
    },
    onError: (err) => setError(err instanceof Error ? err.message : 'Failed to disable MFA'),
  });

  const statusMessage = mfaStatus?.enabled
    ? 'MFA is active for your account.'
    : mfaStatus?.configured
      ? 'Enrollment is configured but still waiting for verification.'
      : 'Protect your account with a TOTP authenticator.';

  return (
    <section className="of-panel" style={{ padding: 24 }}>
      <p className="of-eyebrow">Self-service access</p>
      <h2 className="of-heading-lg">Multi-factor authentication</h2>

      <div className="of-panel-muted" style={{ padding: 20, marginTop: 16 }}>
        <header style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 16 }}>
          <div>
            <h3 className="of-heading-sm">TOTP authenticator</h3>
            <p className="of-text-muted" style={{ fontSize: 13, marginTop: 4 }}>
              {statusMessage}
            </p>
            {mfaStatus && (
              <p className="of-text-soft" style={{ fontSize: 12, marginTop: 4 }}>
                {mfaStatus.recovery_codes_remaining} recovery codes remaining
              </p>
            )}
          </div>
          <span className={`of-chip ${mfaStatus?.enabled ? 'of-status-success' : ''}`}>
            {mfaStatus?.enabled ? 'Enabled' : 'Disabled'}
          </span>
        </header>

        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8, marginTop: 16 }}>
          <button
            type="button"
            className="of-btn of-btn-primary"
            onClick={() => enrollMutation.mutate()}
            disabled={enrollMutation.isPending}
          >
            {enrollMutation.isPending ? 'Generating…' : 'Generate secret'}
          </button>
        </div>

        {enrollment && (
          <div
            style={{
              marginTop: 20,
              padding: 16,
              border: '1px dashed var(--status-info)',
              borderRadius: 'var(--radius-md)',
              fontSize: 13,
            }}
          >
            <div style={{ fontWeight: 500, color: 'var(--text-strong)' }}>Authenticator secret</div>
            <div
              style={{
                marginTop: 8,
                wordBreak: 'break-all',
                fontFamily: 'var(--font-mono)',
                color: 'var(--status-info)',
              }}
            >
              {enrollment.secret}
            </div>
            <div style={{ fontWeight: 500, color: 'var(--text-strong)', marginTop: 16 }}>
              Recovery codes
            </div>
            <div style={{ display: 'grid', gap: 6, gridTemplateColumns: '1fr 1fr', marginTop: 8 }}>
              {enrollment.recovery_codes.map((code) => (
                <div
                  key={code}
                  style={{
                    padding: '8px 12px',
                    background: '#fff',
                    border: '1px solid var(--border-subtle)',
                    borderRadius: 'var(--radius-sm)',
                    fontFamily: 'var(--font-mono)',
                    fontSize: 12,
                  }}
                >
                  {code}
                </div>
              ))}
            </div>
            <form
              onSubmit={(e) => {
                e.preventDefault();
                verifyMutation.mutate();
              }}
              style={{ display: 'flex', gap: 8, marginTop: 16 }}
            >
              <input
                className="of-input"
                value={verifyCode}
                onChange={(e) => setVerifyCode(e.target.value)}
                placeholder="Enter TOTP code"
                style={{ minWidth: 0, flex: 1 }}
              />
              <button
                type="submit"
                className="of-btn of-btn-primary"
                disabled={verifyMutation.isPending || !verifyCode}
              >
                {verifyMutation.isPending ? 'Verifying…' : 'Verify'}
              </button>
            </form>
          </div>
        )}

        {mfaStatus?.enabled && (
          <form
            onSubmit={(e) => {
              e.preventDefault();
              disableMutation.mutate();
            }}
            style={{ display: 'flex', gap: 8, marginTop: 20 }}
          >
            <input
              className="of-input"
              value={disableCode}
              onChange={(e) => setDisableCode(e.target.value)}
              placeholder="Code or recovery code to disable"
              style={{ minWidth: 0, flex: 1 }}
            />
            <button
              type="submit"
              className="of-btn of-btn-danger"
              disabled={disableMutation.isPending || !disableCode}
            >
              {disableMutation.isPending ? 'Disabling…' : 'Disable MFA'}
            </button>
          </form>
        )}
      </div>
    </section>
  );
}
