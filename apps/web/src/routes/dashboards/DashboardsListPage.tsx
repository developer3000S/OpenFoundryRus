import { useEffect, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';

import { ConfirmDialog } from '@components/ConfirmDialog';
import { dashboards, useDashboards } from '@/lib/stores/dashboards';
import {
  formatDashboardTimestamp,
  serializeDashboardSnapshot,
  type DashboardDefinition,
} from '@/lib/utils/dashboards';
import { useTranslator } from '@/lib/i18n/store';

export function DashboardsListPage() {
  const t = useTranslator();
  const navigate = useNavigate();
  const dashboardItems = useDashboards();

  const [feedback, setFeedback] = useState('');
  const [confirmId, setConfirmId] = useState<string | null>(null);

  useEffect(() => {
    dashboards.restore();
  }, []);

  function createDashboard() {
    const dashboard = dashboards.create(`New Dashboard ${dashboardItems.length + 1}`);
    navigate(`/dashboards/${dashboard.id}`);
  }

  function duplicateDashboard(id: string) {
    const copy = dashboards.duplicate(id);
    if (!copy) return;
    navigate(`/dashboards/${copy.id}`);
  }

  function confirmDelete() {
    if (!confirmId) return;
    dashboards.remove(confirmId);
    setConfirmId(null);
  }

  async function shareDashboard(dashboard: DashboardDefinition) {
    if (typeof window === 'undefined') return;
    const snapshot = serializeDashboardSnapshot(dashboard);
    const shareUrl = `${window.location.origin}/dashboards/${dashboard.id}?snapshot=${snapshot}`;
    try {
      await navigator.clipboard.writeText(shareUrl);
      setFeedback(t('pages.dashboards.copied'));
    } catch {
      setFeedback(shareUrl);
    }
  }

  return (
    <section className="of-page" style={{ display: 'grid', gap: 16 }}>
      <header className="of-hero-strip">
        <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 16 }}>
          <div style={{ maxWidth: 720 }}>
            <p className="of-eyebrow">{t('pages.dashboards.badge')}</p>
            <h1 className="of-heading-xl" style={{ marginTop: 8 }}>
              {t('pages.dashboards.heading')}
            </h1>
            <p className="of-text-muted" style={{ marginTop: 8 }}>
              {t('pages.dashboards.description')}
            </p>
          </div>
          <button type="button" className="of-btn of-btn-primary" onClick={createDashboard}>
            {t('pages.dashboards.create')}
          </button>
        </div>

        {feedback && (
          <div className="of-status-success" style={{ marginTop: 16, padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
            {feedback}
          </div>
        )}
      </header>

      <div
        className="of-card-grid"
        style={{ gridTemplateColumns: 'repeat(auto-fit, minmax(320px, 1fr))' }}
      >
        {dashboardItems.map((dashboard) => (
          <article key={dashboard.id} className="of-card">
            <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
              <div>
                <p className="of-eyebrow">
                  {t('pages.dashboards.widgets', { count: dashboard.widgets.length })}
                </p>
                <h2 className="of-heading-md" style={{ marginTop: 8 }}>
                  {dashboard.name}
                </h2>
              </div>
              <span className="of-chip">
                {t('pages.dashboards.updated', { value: formatDashboardTimestamp(dashboard.updatedAt) })}
              </span>
            </div>

            <p className="of-text-muted" style={{ minHeight: 48, fontSize: 13, lineHeight: 1.6 }}>
              {dashboard.description}
            </p>

            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, fontSize: 12 }}>
              {dashboard.widgets.map((widget) => (
                <span key={widget.id} className="of-chip">
                  {widget.type}
                </span>
              ))}
            </div>

            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8, marginTop: 8 }}>
              <Link to={`/dashboards/${dashboard.id}`} className="of-btn of-btn-primary">
                {t('pages.dashboards.open')}
              </Link>
              <button type="button" className="of-btn" onClick={() => duplicateDashboard(dashboard.id)}>
                {t('pages.dashboards.duplicate')}
              </button>
              <button type="button" className="of-btn" onClick={() => shareDashboard(dashboard)}>
                {t('pages.dashboards.share')}
              </button>
              <button type="button" className="of-btn of-btn-danger" onClick={() => setConfirmId(dashboard.id)}>
                {t('pages.dashboards.delete')}
              </button>
            </div>
          </article>
        ))}
      </div>

      <ConfirmDialog
        open={confirmId !== null}
        title={t('pages.dashboards.delete')}
        message={t('pages.dashboards.confirmDelete')}
        confirmLabel={t('pages.dashboards.delete')}
        danger
        onConfirm={confirmDelete}
        onCancel={() => setConfirmId(null)}
      />
    </section>
  );
}
