import { useEffect, useState } from 'react';

import { AuditLogViewer, type EventDraft, type EventFilterDraft } from '@/lib/components/audit/AuditLogViewer';
import { AuditTimeline } from '@/lib/components/audit/AuditTimeline';
import { ComplianceDashboard } from '@/lib/components/audit/ComplianceDashboard';
import { ExportWizard, type GdprDraft, type ReportDraft } from '@/lib/components/audit/ExportWizard';
import { GovernanceStudio, type GovernanceTemplateDraft } from '@/lib/components/audit/GovernanceStudio';
import { PolicyManager, type PolicyDraft } from '@/lib/components/audit/PolicyManager';
import {
  appendEvent,
  applyGovernanceTemplate,
  createPolicy,
  eraseSubjectData,
  exportSubjectData,
  generateReport,
  getCompliancePosture,
  getOverview,
  listAnomalies,
  listClassifications,
  listCollectors,
  listEvents,
  listGovernanceApplications,
  listGovernanceTemplates,
  listPolicies,
  listReports,
  scanSensitiveData,
  updatePolicy,
  type AnomalyAlert,
  type AuditEvent,
  type AuditOverview,
  type AuditPolicy,
  type ClassificationCatalogEntry,
  type CollectorStatus,
  type CompliancePostureOverview,
  type ComplianceReport,
  type GdprEraseResponse,
  type GdprExportPayload,
  type GovernanceTemplate,
  type GovernanceTemplateApplication,
  type SensitiveDataScanResponse,
} from '@/lib/api/audit';
import { notifications } from '@stores/notifications';

function toLocalDateTime(date: Date) {
  const pad = (value: number) => String(value).padStart(2, '0');
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

function emptyFilters(): EventFilterDraft {
  return { source_service: '', subject_id: '', classification: '' };
}

function emptyEventDraft(): EventDraft {
  return {
    source_service: 'gateway',
    channel: 'http',
    actor: 'system:gateway',
    action: 'request.forwarded',
    resource_type: 'http_request',
    resource_id: '/api/v1/apps',
    status: 'success',
    severity: 'low',
    classification: 'confidential',
    subject_id: 'subject-demo-3',
    ip_address: '10.0.0.14',
    location: 'Madrid',
    labels_text: 'manual-probe, phase4',
    metadata_text: JSON.stringify({ method: 'GET', route: '/api/v1/apps', origin: 'audit-console' }, null, 2),
    retention_days: '365',
  };
}

function emptyPolicyDraft(): PolicyDraft {
  return {
    name: 'Access and export retention',
    description: 'Retain sensitive access and export audit events with legal hold support.',
    scope: 'production-platform',
    classification: 'pii',
    retention_days: '730',
    legal_hold: true,
    purge_mode: 'redact-then-retain-hash',
    active: true,
    rules_text: 'mask subject payloads on erasure\npreserve hash chain\nweekly legal hold review',
    updated_by: 'Security Governance',
  };
}

function emptyReportDraft(): ReportDraft {
  const now = new Date();
  const start = new Date(now.getTime() - 1000 * 60 * 60 * 24 * 30);
  return {
    standard: 'soc2',
    title: 'SOC2 Monthly Evidence Pack',
    scope: 'production-platform',
    window_start: toLocalDateTime(start),
    window_end: toLocalDateTime(now),
  };
}

function emptyGdprDraft(): GdprDraft {
  return {
    subject_id: 'subject-demo-1',
    portable_format: 'json',
    hard_delete: false,
    legal_hold: false,
  };
}

function emptyGovernanceTemplateDraft(): GovernanceTemplateDraft {
  return {
    scope: 'production-platform',
    updated_by: 'Security Governance',
    scan_text: 'Customer SSN 123-45-6789 exported to analytics bucket',
  };
}

function parseCsv(value: string) {
  return value
    .split(',')
    .map((entry) => entry.trim())
    .filter(Boolean);
}

function parseLines(value: string) {
  return value
    .split('\n')
    .map((entry) => entry.trim())
    .filter(Boolean);
}

function parseJson<T>(value: string) {
  return JSON.parse(value) as T;
}

function policyToDraft(policy: AuditPolicy): PolicyDraft {
  return {
    id: policy.id,
    name: policy.name,
    description: policy.description,
    scope: policy.scope,
    classification: policy.classification,
    retention_days: String(policy.retention_days),
    legal_hold: policy.legal_hold,
    purge_mode: policy.purge_mode,
    active: policy.active,
    rules_text: policy.rules.join('\n'),
    updated_by: policy.updated_by,
  };
}

export function AuditPage() {
  const [overview, setOverview] = useState<AuditOverview | null>(null);
  const [events, setEvents] = useState<AuditEvent[]>([]);
  const [collectors, setCollectors] = useState<CollectorStatus[]>([]);
  const [anomalies, setAnomalies] = useState<AnomalyAlert[]>([]);
  const [policies, setPolicies] = useState<AuditPolicy[]>([]);
  const [reports, setReports] = useState<ComplianceReport[]>([]);
  const [classifications, setClassifications] = useState<ClassificationCatalogEntry[]>([]);
  const [governanceTemplates, setGovernanceTemplates] = useState<GovernanceTemplate[]>([]);
  const [governanceApplications, setGovernanceApplications] = useState<GovernanceTemplateApplication[]>([]);
  const [compliancePosture, setCompliancePosture] = useState<CompliancePostureOverview | null>(null);
  const [exportPayload, setExportPayload] = useState<GdprExportPayload | null>(null);
  const [eraseResponse, setEraseResponse] = useState<GdprEraseResponse | null>(null);
  const [scanResult, setScanResult] = useState<SensitiveDataScanResponse | null>(null);

  const [selectedPolicyId, setSelectedPolicyId] = useState('');
  const [loading, setLoading] = useState(true);
  const [busyAction, setBusyAction] = useState('');
  const [uiError, setUiError] = useState('');

  const [filters, setFilters] = useState<EventFilterDraft>(emptyFilters);
  const [eventDraft, setEventDraft] = useState<EventDraft>(emptyEventDraft);
  const [policyDraft, setPolicyDraft] = useState<PolicyDraft>(emptyPolicyDraft);
  const [reportDraft, setReportDraft] = useState<ReportDraft>(emptyReportDraft);
  const [gdprDraft, setGdprDraft] = useState<GdprDraft>(emptyGdprDraft);
  const [governanceTemplateDraft, setGovernanceTemplateDraft] = useState<GovernanceTemplateDraft>(emptyGovernanceTemplateDraft);

  const busy = loading || busyAction.length > 0;

  async function refreshAll(currentFilters = filters) {
    setLoading(true);
    setUiError('');
    try {
      const [
        overviewResponse,
        eventResponse,
        collectorsResponse,
        anomaliesResponse,
        policiesResponse,
        reportsResponse,
        classificationsResponse,
        governanceTemplateResponse,
        governanceApplicationResponse,
        compliancePostureResponse,
      ] = await Promise.all([
        getOverview(),
        listEvents(currentFilters),
        listCollectors(),
        listAnomalies(),
        listPolicies(),
        listReports(),
        listClassifications(),
        listGovernanceTemplates(),
        listGovernanceApplications(),
        getCompliancePosture(),
      ]);

      setOverview(overviewResponse);
      setEvents(eventResponse.items);
      setCollectors(collectorsResponse);
      setAnomalies(anomaliesResponse);
      setPolicies(policiesResponse.items);
      setReports(reportsResponse.items);
      setClassifications(classificationsResponse);
      setGovernanceTemplates(governanceTemplateResponse);
      setGovernanceApplications(governanceApplicationResponse.items);
      setCompliancePosture(compliancePostureResponse);

      if (selectedPolicyId) {
        const selected = policiesResponse.items.find((policy) => policy.id === selectedPolicyId);
        if (selected) setPolicyDraft(policyToDraft(selected));
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to load audit surfaces';
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

  async function applyFilters() {
    setBusyAction('filters');
    try {
      const response = await listEvents(filters);
      setEvents(response.items);
      notifications.success(`Loaded ${response.items.length} audit events`);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to apply audit filters';
      setUiError(message);
      notifications.error(message);
    } finally {
      setBusyAction('');
    }
  }

  async function appendEventAction() {
    setBusyAction('append-event');
    try {
      await appendEvent({
        source_service: eventDraft.source_service,
        channel: eventDraft.channel,
        actor: eventDraft.actor,
        action: eventDraft.action,
        resource_type: eventDraft.resource_type,
        resource_id: eventDraft.resource_id,
        status: eventDraft.status,
        severity: eventDraft.severity,
        classification: eventDraft.classification,
        subject_id: eventDraft.subject_id || null,
        ip_address: eventDraft.ip_address || null,
        location: eventDraft.location || null,
        metadata: parseJson<Record<string, unknown>>(eventDraft.metadata_text),
        labels: parseCsv(eventDraft.labels_text),
        retention_days: Number(eventDraft.retention_days),
      });
      await refreshAll();
      notifications.success('Appended audit event to immutable log');
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to append audit event';
      setUiError(message);
      notifications.error(message);
    } finally {
      setBusyAction('');
    }
  }

  function selectPolicy(policyId: string) {
    setSelectedPolicyId(policyId);
    const policy = policies.find((entry) => entry.id === policyId);
    setPolicyDraft(policy ? policyToDraft(policy) : emptyPolicyDraft());
  }

  async function savePolicy() {
    setBusyAction('policy');
    try {
      const payload = {
        name: policyDraft.name,
        description: policyDraft.description,
        scope: policyDraft.scope,
        classification: policyDraft.classification,
        retention_days: Number(policyDraft.retention_days),
        legal_hold: policyDraft.legal_hold,
        purge_mode: policyDraft.purge_mode,
        active: policyDraft.active,
        rules: parseLines(policyDraft.rules_text),
        updated_by: policyDraft.updated_by,
      };
      const policy = policyDraft.id ? await updatePolicy(policyDraft.id, payload) : await createPolicy(payload);
      setSelectedPolicyId(policy.id);
      await refreshAll();
      notifications.success(`${policyDraft.id ? 'Updated' : 'Created'} ${policy.name}`);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to save audit policy';
      setUiError(message);
      notifications.error(message);
    } finally {
      setBusyAction('');
    }
  }

  async function generateReportAction() {
    setBusyAction('report');
    try {
      await generateReport({
        standard: reportDraft.standard,
        title: reportDraft.title,
        scope: reportDraft.scope,
        window_start: new Date(reportDraft.window_start).toISOString(),
        window_end: new Date(reportDraft.window_end).toISOString(),
      });
      await refreshAll();
      notifications.success(`Generated ${reportDraft.standard} evidence pack`);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to generate compliance report';
      setUiError(message);
      notifications.error(message);
    } finally {
      setBusyAction('');
    }
  }

  async function exportSubjectAction() {
    setBusyAction('gdpr-export');
    try {
      const payload = await exportSubjectData({
        subject_id: gdprDraft.subject_id,
        portable_format: gdprDraft.portable_format,
      });
      setExportPayload(payload);
      notifications.success(`Exported data for ${gdprDraft.subject_id}`);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to export subject data';
      setUiError(message);
      notifications.error(message);
    } finally {
      setBusyAction('');
    }
  }

  async function eraseSubjectAction() {
    setBusyAction('gdpr-erase');
    try {
      const response = await eraseSubjectData({
        subject_id: gdprDraft.subject_id,
        hard_delete: gdprDraft.hard_delete,
        legal_hold: gdprDraft.legal_hold,
      });
      setEraseResponse(response);
      await refreshAll();
      notifications.success(`Processed erasure workflow for ${gdprDraft.subject_id}`);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to process subject erasure';
      setUiError(message);
      notifications.error(message);
    } finally {
      setBusyAction('');
    }
  }

  async function applyGovernanceTemplateAction(slug: string) {
    setBusyAction(`template-${slug}`);
    try {
      await applyGovernanceTemplate(slug, {
        scope: governanceTemplateDraft.scope,
        updated_by: governanceTemplateDraft.updated_by,
      });
      await refreshAll();
      notifications.success(`Applied governance template ${slug}`);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to apply governance template';
      setUiError(message);
      notifications.error(message);
    } finally {
      setBusyAction('');
    }
  }

  async function runSensitiveDataScanAction() {
    setBusyAction('sds-scan');
    try {
      const result = await scanSensitiveData({
        content: governanceTemplateDraft.scan_text,
        redact: true,
      });
      setScanResult(result);
      notifications.success('Sensitive data scan completed');
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to run sensitive data scan';
      setUiError(message);
      notifications.error(message);
    } finally {
      setBusyAction('');
    }
  }

  return (
    <section className="of-page" style={{ display: 'grid', gap: 16 }}>
      <section
        style={{
          overflow: 'hidden',
          borderRadius: 32,
          padding: 24,
          color: '#f8fafc',
          background: 'linear-gradient(135deg, #042f2e 0%, #0c0a09 50%, #4a044e 100%)',
        }}
      >
        <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-end', justifyContent: 'space-between', gap: 24 }}>
          <div style={{ maxWidth: 720 }}>
            <p style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.28em', color: '#5eead4' }}>
              Milestone 4.5
            </p>
            <h1 className="of-heading-xl" style={{ marginTop: 12, color: '#f8fafc' }}>
              Immutable audit, compliance evidence, GDPR workflows, and anomaly response
            </h1>
            <p style={{ marginTop: 12, fontSize: 13, lineHeight: 1.6, color: 'rgba(248, 250, 252, 0.85)' }}>
              Operate the audit chain, collector health, retention policies, evidence pack generation, and subject-centric export/erasure from one workspace.
            </p>
          </div>
          <div style={{ borderRadius: 16, background: 'rgba(255,255,255,0.1)', padding: 16, minWidth: 260 }}>
            <p style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.18em', color: '#5eead4' }}>
              Latest event
            </p>
            <p style={{ marginTop: 8, fontSize: 14, fontWeight: 600 }}>{overview?.latest_event?.action ?? 'No events yet'}</p>
            <p style={{ marginTop: 4, fontSize: 12, color: '#a8a29e' }}>{overview?.latest_event?.source_service ?? 'n/a'}</p>
          </div>
        </div>
      </section>

      {uiError && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {uiError}
        </div>
      )}

      <ComplianceDashboard overview={overview} collectors={collectors} anomalies={anomalies} reports={reports} />

      <GovernanceStudio
        templates={governanceTemplates}
        applications={governanceApplications}
        posture={compliancePosture}
        scanResult={scanResult}
        draft={governanceTemplateDraft}
        busy={busy}
        onDraftChange={(patch) => setGovernanceTemplateDraft((current) => ({ ...current, ...patch }))}
        onApplyTemplate={(slug) => void applyGovernanceTemplateAction(slug)}
        onScan={() => void runSensitiveDataScanAction()}
      />

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 1.05fr) minmax(0, 0.95fr)' }}>
        <AuditLogViewer
          events={events}
          classifications={classifications}
          filters={filters}
          draft={eventDraft}
          busy={busy}
          onFilterChange={(patch) => setFilters((current) => ({ ...current, ...patch }))}
          onApplyFilters={() => void applyFilters()}
          onDraftChange={(patch) => setEventDraft((current) => ({ ...current, ...patch }))}
          onAppendEvent={() => void appendEventAction()}
        />
        <AuditTimeline events={events} />
      </div>

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 0.98fr) minmax(0, 1.02fr)' }}>
        <PolicyManager
          policies={policies}
          classifications={classifications}
          selectedPolicyId={selectedPolicyId}
          draft={policyDraft}
          busy={busy}
          onSelectPolicy={selectPolicy}
          onDraftChange={(patch) => setPolicyDraft((current) => ({ ...current, ...patch }))}
          onSave={() => void savePolicy()}
          onReset={() => {
            setSelectedPolicyId('');
            setPolicyDraft(emptyPolicyDraft());
          }}
        />
        <ExportWizard
          reports={reports}
          reportDraft={reportDraft}
          gdprDraft={gdprDraft}
          exportPayload={exportPayload}
          eraseResponse={eraseResponse}
          busy={busy}
          onReportDraftChange={(patch) => setReportDraft((current) => ({ ...current, ...patch }))}
          onGdprDraftChange={(patch) => setGdprDraft((current) => ({ ...current, ...patch }))}
          onGenerateReport={() => void generateReportAction()}
          onExportSubject={() => void exportSubjectAction()}
          onEraseSubject={() => void eraseSubjectAction()}
        />
      </div>
    </section>
  );
}
