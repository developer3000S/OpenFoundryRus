import { useEffect, useState } from 'react';

import { ReportDesigner, type ReportDraft } from '@/lib/components/report/ReportDesigner';
import { ReportHistory } from '@/lib/components/report/ReportHistory';
import { ReportPreview } from '@/lib/components/report/ReportPreview';
import { ScheduleManager } from '@/lib/components/report/ScheduleManager';
import { TemplateLibrary } from '@/lib/components/report/TemplateLibrary';
import {
  createReport,
  generateReport,
  getCatalog,
  getDownload,
  getExecution,
  getOverview,
  getScheduleBoard,
  listHistory,
  listReports,
  updateReport,
  type DistributionRecipient,
  type DownloadPayload,
  type ReportCatalog,
  type ReportDefinition,
  type ReportExecution,
  type ReportOverview,
  type ReportSchedule,
  type ReportTemplate,
  type ScheduleBoard,
} from '@/lib/api/reports';
import { notifications } from '@stores/notifications';

function formatJson(value: unknown) {
  return JSON.stringify(value, null, 2);
}

function parseJson<T>(value: string): T {
  return JSON.parse(value) as T;
}

function parseCsv(value: string) {
  return value
    .split(',')
    .map((entry) => entry.trim())
    .filter(Boolean);
}

function createEmptyReportDraft(): ReportDraft {
  return {
    name: 'Executive Revenue Pulse',
    description: 'Weekly executive digest with KPI cards, regional split, and map hotspots.',
    owner: 'Revenue Operations',
    generator_kind: 'pdf',
    dataset_name: 'sales_fact_daily',
    active: true,
    tags_text: 'executive, weekly, revenue',
    schedule_text: formatJson({
      cadence: 'weekly',
      expression: '0 9 * * MON',
      timezone: 'UTC',
      anchor_time: '09:00',
      interval_minutes: 10080,
      enabled: true,
      next_run_at: null,
    }),
    template_text: formatJson({
      title: 'Executive Revenue Pulse',
      subtitle: 'Weekly commercial operating review',
      theme: 'copper',
      layout: 'briefing',
      sections: [
        {
          id: 'gross-margin',
          title: 'Gross Margin',
          kind: 'kpi',
          query: 'select margin from revenue_kpis',
          description: 'Margin headline for leadership',
          config: { unit: '%' },
        },
        {
          id: 'regional-split',
          title: 'Regional Revenue',
          kind: 'table',
          query: 'select region, revenue from regional_revenue',
          description: 'Regional split by operating market',
          config: { sortBy: 'value' },
        },
      ],
    }),
    recipients_text: formatJson([
      {
        id: 'exec-email',
        channel: 'email',
        target: 'exec-team@openfoundry.dev',
        label: 'Executive distribution',
        config: { subject: 'Weekly revenue pulse' },
      },
    ]),
  };
}

function reportToDraft(report: ReportDefinition): ReportDraft {
  return {
    id: report.id,
    name: report.name,
    description: report.description,
    owner: report.owner,
    generator_kind: report.generator_kind,
    dataset_name: report.dataset_name,
    active: report.active,
    tags_text: report.tags.join(', '),
    schedule_text: formatJson(report.schedule),
    template_text: formatJson(report.template),
    recipients_text: formatJson(report.recipients),
  };
}

export function ReportsPage() {
  const [overview, setOverview] = useState<ReportOverview | null>(null);
  const [catalog, setCatalog] = useState<ReportCatalog | null>(null);
  const [reports, setReports] = useState<ReportDefinition[]>([]);
  const [scheduleBoard, setScheduleBoard] = useState<ScheduleBoard | null>(null);
  const [history, setHistory] = useState<ReportExecution[]>([]);
  const [selectedReportId, setSelectedReportId] = useState('');
  const [latestExecution, setLatestExecution] = useState<ReportExecution | null>(null);
  const [downloadPayload, setDownloadPayload] = useState<DownloadPayload | null>(null);
  const [draft, setDraft] = useState<ReportDraft>(createEmptyReportDraft());
  const [loading, setLoading] = useState(true);
  const [busyAction, setBusyAction] = useState('');
  const [uiError, setUiError] = useState('');

  const busy = loading || busyAction.length > 0;

  async function loadHistory(reportId: string) {
    const response = await listHistory(reportId);
    setHistory(response.items);
    if (response.items.length > 0) {
      const newest = response.items[0];
      setLatestExecution(newest);
      setDownloadPayload(await getDownload(newest.id));
    } else {
      setLatestExecution(null);
      setDownloadPayload(null);
    }
  }

  async function selectReport(reportId: string, notify = true, source?: ReportDefinition[]) {
    setSelectedReportId(reportId);
    const pool = source ?? reports;
    const report = pool.find((entry) => entry.id === reportId);
    setDraft(report ? reportToDraft(report) : createEmptyReportDraft());
    if (reportId) {
      await loadHistory(reportId);
      if (notify) notifications.info(`Loaded ${report?.name ?? 'report'} context`);
    }
  }

  async function refreshAll(preferredReportId?: string) {
    setLoading(true);
    setUiError('');
    try {
      const [overviewResponse, catalogResponse, reportsResponse, boardResponse] = await Promise.all([
        getOverview(),
        getCatalog(),
        listReports(),
        getScheduleBoard(),
      ]);
      setOverview(overviewResponse);
      setCatalog(catalogResponse);
      setReports(reportsResponse.items);
      setScheduleBoard(boardResponse);

      const nextSelected = preferredReportId ?? selectedReportId ?? reportsResponse.items[0]?.id ?? '';
      if (nextSelected) {
        await selectReport(nextSelected, false, reportsResponse.items);
      } else {
        setSelectedReportId('');
        setHistory([]);
        setLatestExecution(null);
        setDownloadPayload(null);
        setDraft(createEmptyReportDraft());
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to load reporting surfaces';
      setUiError(message);
      notifications.error(message);
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void refreshAll();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  function updateDraft(patch: Partial<ReportDraft>) {
    setDraft((current) => ({ ...current, ...patch }));
  }

  function resetDraft() {
    setSelectedReportId('');
    setDraft(createEmptyReportDraft());
    setHistory([]);
    setLatestExecution(null);
    setDownloadPayload(null);
  }

  async function saveReportDraft() {
    setBusyAction('save-report');
    setUiError('');
    try {
      const payload = {
        name: draft.name,
        description: draft.description,
        owner: draft.owner,
        generator_kind: draft.generator_kind,
        dataset_name: draft.dataset_name,
        active: draft.active,
        tags: parseCsv(draft.tags_text),
        schedule: parseJson<ReportSchedule>(draft.schedule_text),
        template: parseJson<ReportTemplate>(draft.template_text),
        recipients: parseJson<DistributionRecipient[]>(draft.recipients_text),
        parameters: {} as Record<string, unknown>,
      };
      const report = draft.id ? await updateReport(draft.id, payload) : await createReport(payload);
      notifications.success(`${report.name} saved`);
      await refreshAll(report.id);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to save report';
      setUiError(message);
      notifications.error(message);
    } finally {
      setBusyAction('');
    }
  }

  async function runSelectedReport() {
    if (!selectedReportId) {
      notifications.warning('Select or create a report before generating it');
      return;
    }
    setBusyAction('run-report');
    setUiError('');
    try {
      const execution = await generateReport(selectedReportId);
      setLatestExecution(execution);
      setDownloadPayload(await getDownload(execution.id));
      notifications.success(`${execution.report_name} generated`);
      await refreshAll(selectedReportId);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to generate report';
      setUiError(message);
      notifications.error(message);
    } finally {
      setBusyAction('');
    }
  }

  async function selectExecution(executionId: string) {
    setBusyAction('load-execution');
    setUiError('');
    try {
      setLatestExecution(await getExecution(executionId));
      setDownloadPayload(await getDownload(executionId));
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to load execution';
      setUiError(message);
      notifications.error(message);
    } finally {
      setBusyAction('');
    }
  }

  return (
    <section className="of-page" style={{ display: 'grid', gap: 16 }}>
      <div className="of-panel" style={{ padding: 24 }}>
        <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 24 }}>
          <div style={{ maxWidth: 720 }}>
            <p className="of-eyebrow" style={{ color: '#b45309' }}>
              Reports
            </p>
            <h1 className="of-heading-xl" style={{ marginTop: 8 }}>
              Definitions, schedules, distributions, and PDF/PPTX generation
            </h1>
            <p className="of-text-muted" style={{ marginTop: 12, fontSize: 14, lineHeight: 1.7 }}>
              Compose report definitions, manage cadence and recipients, generate executions, and
              inspect download artifacts.
            </p>
          </div>
          <div style={{ display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr', minWidth: 360 }}>
            {[
              { label: 'Definitions', value: overview?.report_count ?? 0 },
              { label: 'Active schedules', value: overview?.active_schedules ?? 0 },
              { label: 'Executions 24h', value: overview?.executions_24h ?? 0 },
              {
                label: 'Generators',
                value: overview?.generator_mix.join(' • ') || 'No generators yet',
                small: true,
              },
            ].map((stat) => (
              <div key={stat.label} className="of-panel-muted" style={{ padding: 16 }}>
                <p className="of-eyebrow">{stat.label}</p>
                <p
                  style={{
                    marginTop: 8,
                    fontSize: stat.small ? 13 : 22,
                    fontWeight: stat.small ? 500 : 600,
                    color: 'var(--text-strong)',
                  }}
                >
                  {stat.value}
                </p>
              </div>
            ))}
          </div>
        </div>
      </div>

      {uiError && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {uiError}
        </div>
      )}

      <ReportDesigner
        reports={reports}
        selectedReportId={selectedReportId}
        draft={draft}
        busy={busy}
        onSelect={(id) => void selectReport(id)}
        onDraftChange={updateDraft}
        onSave={() => void saveReportDraft()}
        onReset={resetDraft}
      />

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 0.95fr) minmax(0, 1.05fr)' }}>
        <TemplateLibrary catalog={catalog} />
        <ScheduleManager
          board={scheduleBoard}
          selectedReportId={selectedReportId}
          busy={busy}
          onSelectReport={(id) => void selectReport(id)}
          onGenerate={() => void runSelectedReport()}
        />
      </div>

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 1.1fr) minmax(0, 0.9fr)' }}>
        <ReportPreview execution={latestExecution} download={downloadPayload} />
        <ReportHistory history={history} onSelectExecution={(id) => void selectExecution(id)} />
      </div>
    </section>
  );
}
