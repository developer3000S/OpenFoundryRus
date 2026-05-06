import { useEffect, useMemo, useState } from 'react';

import {
  listActionTypes,
  listInterfaces,
  listLinkTypes,
  listObjectTypes,
  listProjectMemberships,
  listProjects,
  listProperties,
  type ActionType,
  type LinkType,
  type ObjectType,
  type OntologyInterface,
  type OntologyProject,
  type OntologyProjectMembership,
  type Property,
} from '@/lib/api/ontology';

type ReviewTab = 'scorecard' | 'anti-patterns' | 'playbook' | 'review';
type Severity = 'critical' | 'warning' | 'info';
type CheckStatus = 'strong' | 'attention' | 'gap';

interface Finding {
  id: string;
  code: string;
  title: string;
  severity: Severity;
  affected: string;
  summary: string;
  recommendation: string;
  evidence: string[];
}

interface PracticeCheck {
  id: string;
  label: string;
  status: CheckStatus;
  score: number;
  detail: string;
}

interface ReviewState {
  dismissed: string[];
  notes: string;
}

const STORAGE_KEY = 'of.ontologyDesign.review';

const TABS: Array<{ id: ReviewTab; label: string }> = [
  { id: 'scorecard', label: 'Scorecard' },
  { id: 'anti-patterns', label: 'Anti-patterns' },
  { id: 'playbook', label: 'Playbook' },
  { id: 'review', label: 'Review' },
];

const SEVERITIES: Array<'all' | Severity> = ['all', 'critical', 'warning', 'info'];

const GENERIC_TYPE_NAMES = new Set(['item', 'items', 'object', 'objects', 'entity', 'entities', 'record', 'records', 'data']);
const GENERIC_PROPERTY_NAMES = new Set(['name', 'value', 'type', 'status', 'date', 'id']);
const GENERIC_LINK_NAMES = new Set(['related', 'related_to', 'link', 'association', 'connected_to']);
const SYSTEM_KEYWORDS = ['system', 'crm', 'erp', 'sap', 'hr', 'badge', 'project management', 'salesforce', 'workday', 'servicenow'];
const DEPARTMENT_KEYWORDS = ['sales', 'support', 'finance', 'billing', 'marketing', 'operations', 'ops', 'hr', 'legal'];
const METADATA_PATTERNS: RegExp[] = [
  /^_/,
  /etl/i,
  /extracted/i,
  /received/i,
  /batched/i,
  /sequence/i,
  /table_version/i,
  /internal_record_id/i,
  /debug/i,
  /metadata/i,
];

function normalizeName(value: string) {
  return value
    .trim()
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, ' ')
    .replace(/\s+/g, ' ')
    .trim();
}

function normalizeBaseName(value: string) {
  const normalized = normalizeName(value)
    .replace(/\b(system|crm|erp|sap|hr|badge|salesforce|support|finance|billing|marketing|operations|ops|legal)\b/g, '')
    .replace(/\s+/g, ' ')
    .trim();
  return normalized || normalizeName(value);
}

function severityWeight(severity: Severity) {
  if (severity === 'critical') return 3;
  if (severity === 'warning') return 2;
  return 1;
}

function readReviewState(): ReviewState {
  if (typeof window === 'undefined') return { dismissed: [], notes: '' };
  try {
    return JSON.parse(window.localStorage.getItem(STORAGE_KEY) ?? '{"dismissed":[],"notes":""}');
  } catch {
    return { dismissed: [], notes: '' };
  }
}

function persistReviewState(state: ReviewState) {
  if (typeof window === 'undefined') return;
  window.localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
}

export function OntologyDesignPage() {
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState('');
  const [activeTab, setActiveTab] = useState<ReviewTab>('scorecard');
  const [activeSeverity, setActiveSeverity] = useState<'all' | Severity>('all');

  const [objectTypes, setObjectTypes] = useState<ObjectType[]>([]);
  const [linkTypes, setLinkTypes] = useState<LinkType[]>([]);
  const [actionTypes, setActionTypes] = useState<ActionType[]>([]);
  const [interfaces, setInterfaces] = useState<OntologyInterface[]>([]);
  const [projects, setProjects] = useState<OntologyProject[]>([]);
  const [projectMemberships, setProjectMemberships] = useState<Record<string, OntologyProjectMembership[]>>({});
  const [propertiesByType, setPropertiesByType] = useState<Record<string, Property[]>>({});
  const [reviewState, setReviewState] = useState<ReviewState>({ dismissed: [], notes: '' });

  useEffect(() => {
    let cancelled = false;
    async function load() {
      setLoading(true);
      setLoadError('');
      try {
        const [typeResponse, linkResponse, actionResponse, interfaceResponse, projectResponse] = await Promise.all([
          listObjectTypes({ page: 1, per_page: 200 }),
          listLinkTypes({ page: 1, per_page: 200 }),
          listActionTypes({ page: 1, per_page: 200 }).catch(() => ({ data: [], total: 0, page: 1, per_page: 200 })),
          listInterfaces({ page: 1, per_page: 200 }).catch(() => ({ data: [], total: 0, page: 1, per_page: 200 })),
          listProjects({ page: 1, per_page: 200 }).catch(() => ({ data: [], total: 0, page: 1, per_page: 200 })),
        ]);
        if (cancelled) return;
        setObjectTypes(typeResponse.data);
        setLinkTypes(linkResponse.data);
        setActionTypes(actionResponse.data);
        setInterfaces(interfaceResponse.data);
        setProjects(projectResponse.data);

        const propertyEntries = await Promise.all(
          typeResponse.data.map(
            async (objectType) => [objectType.id, await listProperties(objectType.id).catch(() => [])] as const,
          ),
        );
        if (cancelled) return;
        setPropertiesByType(Object.fromEntries(propertyEntries));

        const membershipEntries = await Promise.all(
          projectResponse.data.map(
            async (project) => [project.id, await listProjectMemberships(project.id).catch(() => [])] as const,
          ),
        );
        if (cancelled) return;
        setProjectMemberships(Object.fromEntries(membershipEntries));

        setReviewState(readReviewState());
      } catch (error) {
        if (cancelled) return;
        setLoadError(error instanceof Error ? error.message : 'Failed to load ontology design review');
      } finally {
        if (!cancelled) setLoading(false);
      }
    }
    void load();
    return () => {
      cancelled = true;
    };
  }, []);

  function dismissFinding(id: string) {
    setReviewState((current) => {
      if (current.dismissed.includes(id)) return current;
      const next = { ...current, dismissed: [...current.dismissed, id] };
      persistReviewState(next);
      return next;
    });
  }

  function restoreDismissed() {
    setReviewState((current) => {
      const next = { ...current, dismissed: [] };
      persistReviewState(next);
      return next;
    });
  }

  function updateNotes(value: string) {
    setReviewState((current) => {
      const next = { ...current, notes: value };
      persistReviewState(next);
      return next;
    });
  }

  const findings = useMemo<Finding[]>(() => {
    function propertiesFor(typeId: string) {
      return propertiesByType[typeId] ?? [];
    }
    function actionTypesFor(typeId: string) {
      return actionTypes.filter((item) => item.object_type_id === typeId);
    }
    function labelForType(typeId: string) {
      return objectTypes.find((item) => item.id === typeId)?.display_name ?? typeId;
    }

    const findSystemSilos = (): Finding[] => {
      const grouped = new Map<string, ObjectType[]>();
      for (const objectType of objectTypes) {
        const normalized = normalizeBaseName(objectType.display_name || objectType.name);
        const entries = grouped.get(normalized) ?? [];
        entries.push(objectType);
        grouped.set(normalized, entries);
      }
      return [...grouped.entries()]
        .filter(([, items]) => items.length > 1)
        .filter(([, items]) =>
          items.some((item) => SYSTEM_KEYWORDS.some((keyword) => normalizeName(`${item.name} ${item.display_name}`).includes(keyword))),
        )
        .map(([base, items]) => ({
          id: `system-silos-${base}`,
          code: 'system_silos',
          title: 'System Silos',
          severity: 'critical' as const,
          affected: items.map((item) => item.display_name).join(', '),
          summary: `Multiple object types look like source-system variants of the same entity base: ${base}.`,
          recommendation: 'Merge system-specific entities into one unified object type and join data upstream in pipelines.',
          evidence: items.map((item) => `${item.display_name} (${item.name})`),
        }));
    };

    const findDepartmentSilos = (): Finding[] => {
      const grouped = new Map<string, ObjectType[]>();
      for (const objectType of objectTypes) {
        const normalized = normalizeBaseName(objectType.display_name || objectType.name);
        const entries = grouped.get(normalized) ?? [];
        entries.push(objectType);
        grouped.set(normalized, entries);
      }
      return [...grouped.entries()]
        .filter(([, items]) => items.length > 1)
        .filter(([, items]) =>
          items.some((item) => DEPARTMENT_KEYWORDS.some((keyword) => normalizeName(`${item.name} ${item.display_name}`).includes(keyword))),
        )
        .map(([base, items]) => ({
          id: `department-silos-${base}`,
          code: 'department_silos',
          title: 'Department Silos',
          severity: 'critical' as const,
          affected: items.map((item) => item.display_name).join(', '),
          summary: `The ontology appears to split a shared entity base across department-specific types: ${base}.`,
          recommendation: 'Use one shared object type, keep department-specific attributes as properties or links, and curate different object views by team.',
          evidence: items.map((item) => `${item.display_name} (${item.name})`),
        }));
    };

    const findKitchenSink = (): Finding[] =>
      objectTypes.flatMap((objectType) => {
        const flagged = propertiesFor(objectType.id).filter((property) =>
          METADATA_PATTERNS.some((pattern) => pattern.test(property.name) || pattern.test(property.display_name)),
        );
        if (flagged.length < 3) return [];
        return [
          {
            id: `kitchen-sink-${objectType.id}`,
            code: 'kitchen_sink',
            title: 'The Kitchen Sink',
            severity: flagged.length >= 6 ? ('critical' as const) : ('warning' as const),
            affected: objectType.display_name,
            summary: `${objectType.display_name} exposes several technical metadata fields that look better suited to backing datasets than to the ontology surface.`,
            recommendation: 'Remove ETL and system metadata from the ontology-facing schema, or hide it behind curated views instead of core properties.',
            evidence: flagged.slice(0, 8).map((property) => property.name),
          },
        ];
      });

    const findGodObjects = (): Finding[] =>
      objectTypes.flatMap((objectType) => {
        const properties = propertiesFor(objectType.id);
        const genericSignals = properties.filter((property) => GENERIC_PROPERTY_NAMES.has(normalizeName(property.name))).length;
        if (properties.length < 40 && !(properties.length >= 25 && genericSignals >= 3)) return [];
        return [
          {
            id: `god-object-${objectType.id}`,
            code: 'god_object',
            title: 'The God Object',
            severity: properties.length >= 60 ? ('critical' as const) : ('warning' as const),
            affected: objectType.display_name,
            summary: `${objectType.display_name} carries ${properties.length} properties, which suggests the object may represent multiple entities or overly broad use cases.`,
            recommendation: 'Split distinct entities into their own object types and use interfaces for genuinely shared characteristics.',
            evidence: [
              `${properties.length} total properties`,
              `${genericSignals} generic property names`,
              objectType.description || 'No description',
            ],
          },
        ];
      });

    const findGoldenHammer = (): Finding[] =>
      objectTypes.flatMap((objectType) => {
        const actions = actionTypesFor(objectType.id);
        const imperative = actions.filter((action) =>
          /^(set|update|calculate|assign|sync|refresh)\b/i.test(action.display_name || action.name),
        );
        if (actions.length < 8 || imperative.length < Math.ceil(actions.length * 0.6)) return [];
        return [
          {
            id: `golden-hammer-${objectType.id}`,
            code: 'golden_hammer',
            title: 'The Golden Hammer',
            severity: 'warning' as const,
            affected: objectType.display_name,
            summary: `${objectType.display_name} relies on a dense cluster of imperative actions, which can indicate that actions are standing in for pipelines, automations, or cleaner workflow design.`,
            recommendation: 'Re-check whether these changes should be precomputed in pipelines, event-driven in automations, or bundled into fewer meaningful operations.',
            evidence: imperative.slice(0, 8).map((action) => action.display_name || action.name),
          },
        ];
      });

    const findActionSprawl = (): Finding[] =>
      objectTypes.flatMap((objectType) => {
        const actions = actionTypesFor(objectType.id);
        const singleField = actions.filter((action) => /^(set|update)\b/i.test(action.display_name || action.name));
        if (actions.length < 10 && singleField.length < 5) return [];
        return [
          {
            id: `action-sprawl-${objectType.id}`,
            code: 'action_sprawl',
            title: 'Action Sprawl',
            severity: 'warning' as const,
            affected: objectType.display_name,
            summary: `${objectType.display_name} has ${actions.length} actions, many of which read like single-field updates rather than business operations.`,
            recommendation: 'Bundle related edits into cohesive business actions such as transfer, onboard, approve, or escalate.',
            evidence: singleField.slice(0, 10).map((action) => action.display_name || action.name),
          },
        ];
      });

    const findTimeMachine = (): Finding[] =>
      objectTypes.flatMap((objectType) => {
        const properties = propertiesFor(objectType.id);
        const temporalSignals = properties.filter((property) => /(version|revision|is_current|effective_date|amended_at)/i.test(property.name));
        const yearlyNaming = /(?:19|20)\d{2}\b/.test(objectType.display_name) || /(?:19|20)\d{2}\b/.test(objectType.name);
        if (!yearlyNaming && temporalSignals.length < 2) return [];
        return [
          {
            id: `time-machine-${objectType.id}`,
            code: 'time_machine',
            title: 'The Time Machine',
            severity: 'warning' as const,
            affected: objectType.display_name,
            summary: `${objectType.display_name} shows versioning signals that may indicate historical copies are being modeled as first-class objects instead of current state plus history.`,
            recommendation: 'Keep one current object per entity, then model amendments or history in linked objects or time-aware properties.',
            evidence: [
              yearlyNaming ? 'Object type name contains a year/version signal.' : '',
              ...temporalSignals.slice(0, 6).map((property) => property.name),
            ].filter(Boolean),
          },
        ];
      });

    const findMisnomers = (): Finding[] => {
      const typeFindings: Finding[] = objectTypes
        .filter(
          (objectType) =>
            GENERIC_TYPE_NAMES.has(normalizeName(objectType.name)) ||
            GENERIC_TYPE_NAMES.has(normalizeName(objectType.display_name)),
        )
        .map((objectType) => ({
          id: `misnomer-type-${objectType.id}`,
          code: 'misnomer',
          title: 'The Misnomer',
          severity: 'warning' as const,
          affected: objectType.display_name,
          summary: `${objectType.display_name} uses a generic name that is hard to interpret without extra context.`,
          recommendation: 'Rename object types and properties so they explain the business entity directly, not a vague placeholder.',
          evidence: [objectType.name, objectType.display_name],
        }));
      const propertyFindings: Finding[] = objectTypes.flatMap((objectType) => {
        const flagged = propertiesFor(objectType.id).filter((property) => GENERIC_PROPERTY_NAMES.has(normalizeName(property.name)));
        if (flagged.length === 0) return [];
        return [
          {
            id: `misnomer-property-${objectType.id}`,
            code: 'misnomer',
            title: 'The Misnomer',
            severity: 'info' as const,
            affected: objectType.display_name,
            summary: `${objectType.display_name} contains generic property names that may need qualification.`,
            recommendation: 'Prefer names like monetary_value, due_date, product_category, or risk_score over generic labels like value, date, or type.',
            evidence: flagged.slice(0, 8).map((property) => property.name),
          },
        ];
      });
      const linkFindings: Finding[] = linkTypes
        .filter(
          (linkType) =>
            GENERIC_LINK_NAMES.has(normalizeName(linkType.name)) ||
            GENERIC_LINK_NAMES.has(normalizeName(linkType.display_name)),
        )
        .map((linkType) => ({
          id: `misnomer-link-${linkType.id}`,
          code: 'misnomer',
          title: 'The Misnomer',
          severity: 'info' as const,
          affected: linkType.display_name,
          summary: `${linkType.display_name} does not clearly describe the relationship between the two sides of the link.`,
          recommendation: 'Name links after the relationship, such as Supervisor, Manufacturing Facility, or Purchasing Customer.',
          evidence: [linkType.name, `${labelForType(linkType.source_type_id)} -> ${labelForType(linkType.target_type_id)}`],
        }));
      return [...typeFindings, ...propertyFindings, ...linkFindings];
    };

    return [
      ...findSystemSilos(),
      ...findDepartmentSilos(),
      ...findKitchenSink(),
      ...findGodObjects(),
      ...findGoldenHammer(),
      ...findActionSprawl(),
      ...findTimeMachine(),
      ...findMisnomers(),
    ].sort((left, right) => severityWeight(right.severity) - severityWeight(left.severity));
  }, [objectTypes, linkTypes, actionTypes, propertiesByType]);

  const visibleFindings = useMemo(
    () =>
      findings.filter(
        (finding) =>
          !reviewState.dismissed.includes(finding.id) && (activeSeverity === 'all' || finding.severity === activeSeverity),
      ),
    [findings, reviewState.dismissed, activeSeverity],
  );

  const designScore = useMemo(() => {
    let score = 100;
    for (const finding of findings) {
      score -= finding.severity === 'critical' ? 12 : finding.severity === 'warning' ? 6 : 2;
    }
    return Math.max(0, score);
  }, [findings]);

  const practiceChecks = useMemo<PracticeCheck[]>(() => {
    const totalProperties = Object.values(propertiesByType).flat();
    const describedTypes = objectTypes.filter((item) => item.description.trim().length > 0).length;
    const describedProperties = totalProperties.filter((item) => item.description.trim().length > 0).length;
    const descriptionCoverage = totalProperties.length === 0 ? 1 : describedProperties / totalProperties.length;
    const multiMemberProjects = projects.filter((project) => (projectMemberships[project.id] ?? []).length > 1).length;
    const siloCount = findings.filter((item) => item.code === 'system_silos' || item.code === 'department_silos').length;
    const godObjectCount = findings.filter((item) => item.code === 'god_object').length;
    const actionRiskCount = findings.filter((item) => item.code === 'golden_hammer' || item.code === 'action_sprawl').length;
    const misnomerCount = findings.filter((item) => item.code === 'misnomer').length;

    return [
      {
        id: 'model-reality',
        label: 'Model reality, not systems',
        status: siloCount === 0 ? 'strong' : siloCount === 1 ? 'attention' : 'gap',
        score: Math.max(0, 100 - siloCount * 30),
        detail:
          siloCount === 0
            ? 'No major system or department silos detected from naming patterns.'
            : `${siloCount} silo pattern groups detected across object types.`,
      },
      {
        id: 'curate-intentionally',
        label: 'Curate intentionally',
        status: findings.some((item) => item.code === 'kitchen_sink') ? 'attention' : 'strong',
        score: findings.some((item) => item.code === 'kitchen_sink') ? 62 : 91,
        detail: findings.some((item) => item.code === 'kitchen_sink')
          ? 'Technical metadata appears exposed as ontology properties on at least one object type.'
          : 'Property curation looks reasonably intentional from current schema signals.',
      },
      {
        id: 'collaborate-across-teams',
        label: 'Collaborate across teams',
        status: multiMemberProjects > 0 ? 'strong' : 'attention',
        score: multiMemberProjects > 0 ? 88 : 58,
        detail:
          multiMemberProjects > 0
            ? `${multiMemberProjects} ontology spaces already have multiple memberships, which supports cross-team ownership.`
            : 'No multi-member ontology space was detected from current project memberships.',
      },
      {
        id: 'keep-types-focused',
        label: 'Keep object types focused',
        status: godObjectCount === 0 ? 'strong' : godObjectCount === 1 ? 'attention' : 'gap',
        score: Math.max(0, 100 - godObjectCount * 25),
        detail:
          godObjectCount === 0
            ? 'No likely god-object surface detected from property counts.'
            : `${godObjectCount} object types look overloaded.`,
      },
      {
        id: 'choose-right-tool',
        label: 'Choose the right tool',
        status: actionRiskCount === 0 ? 'strong' : 'attention',
        score: Math.max(0, 90 - actionRiskCount * 12),
        detail:
          actionRiskCount === 0
            ? 'Action surface does not currently show obvious overuse patterns.'
            : `${actionRiskCount} action-design findings suggest some workflows may belong in pipelines, automations, or broader business actions.`,
      },
      {
        id: 'use-interfaces',
        label: 'Use interfaces for abstraction',
        status: interfaces.length > 0 ? 'strong' : 'attention',
        score: interfaces.length > 0 ? 90 : 52,
        detail:
          interfaces.length > 0
            ? `${interfaces.length} interfaces are available for shared abstractions.`
            : 'No interfaces are currently modeled, which may push too much reuse into wide object types.',
      },
      {
        id: 'document-decisions',
        label: 'Document your decisions',
        status:
          describedTypes === objectTypes.length && descriptionCoverage >= 0.75
            ? 'strong'
            : descriptionCoverage >= 0.45
              ? 'attention'
              : 'gap',
        score: Math.round((describedTypes / Math.max(objectTypes.length, 1)) * 40 + descriptionCoverage * 60),
        detail: `${describedTypes}/${objectTypes.length} object types and ${(descriptionCoverage * 100).toFixed(0)}% of properties have descriptions. ${
          misnomerCount > 0 ? 'Generic naming still increases documentation pressure.' : ''
        }`,
      },
    ];
  }, [findings, objectTypes, propertiesByType, projects, projectMemberships, interfaces]);

  return (
    <section className="of-page" style={{ display: 'grid', gap: 16 }}>
      <div className="of-panel" style={{ padding: 24 }}>
        <div style={{ display: 'grid', gap: 24, gridTemplateColumns: '1.45fr 1fr' }}>
          <div>
            <p className="of-eyebrow" style={{ color: '#0369a1' }}>
              Ontology design
            </p>
            <h1 className="of-heading-xl" style={{ marginTop: 8 }}>
              Best practices and anti-patterns
            </h1>
            <p className="of-text-muted" style={{ marginTop: 12, fontSize: 14, lineHeight: 1.7 }}>
              Review ontology quality against real design guidance: scorecard the current model, detect
              anti-patterns across object types, properties, links, actions, and interfaces, then route
              remediation back into the right OpenFoundry products.
            </p>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8, marginTop: 14, fontSize: 12 }}>
              {[
                { href: '/ontology-manager', label: 'Ontology Manager' },
                { href: '/object-link-types', label: 'Object and Link Types' },
                { href: '/action-types', label: 'Action Types' },
                { href: '/interfaces', label: 'Interfaces' },
              ].map((link) => (
                <a key={link.href} href={link.href} className="of-chip">
                  {link.label}
                </a>
              ))}
            </div>
          </div>

          <div style={{ display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr' }}>
            {[
              { label: 'Design score', value: designScore, sub: 'Composite ontology design posture.' },
              { label: 'Findings', value: findings.length, sub: 'Detected anti-pattern signals.' },
              { label: 'Object types', value: objectTypes.length, sub: 'Schema surfaces reviewed.' },
              { label: 'Interfaces', value: interfaces.length, sub: 'Shared abstraction contracts discovered.' },
            ].map((card) => (
              <div key={card.label} className="of-panel-muted" style={{ padding: 16 }}>
                <p className="of-eyebrow">{card.label}</p>
                <p style={{ marginTop: 4, fontSize: 28, fontWeight: 600, color: 'var(--text-strong)' }}>{card.value}</p>
                <p className="of-text-muted" style={{ marginTop: 4, fontSize: 12 }}>
                  {card.sub}
                </p>
              </div>
            ))}
          </div>
        </div>
      </div>

      {loadError && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {loadError}
        </div>
      )}

      {loading ? (
        <div className="of-panel" style={{ padding: 56, textAlign: 'center', fontSize: 13, color: 'var(--text-muted)' }}>
          Loading ontology design review…
        </div>
      ) : (
        <>
          <section className="of-panel" style={{ padding: 12 }}>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
              {TABS.map((tab) => {
                const active = activeTab === tab.id;
                return (
                  <button
                    key={tab.id}
                    type="button"
                    onClick={() => setActiveTab(tab.id)}
                    className={active ? 'of-button of-button--primary' : 'of-button'}
                    style={{ fontSize: 13 }}
                  >
                    {tab.label}
                  </button>
                );
              })}
            </div>
          </section>

          {activeTab === 'scorecard' && (
            <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 1fr) 340px' }}>
              <section className="of-panel" style={{ padding: 20, display: 'grid', gap: 10 }}>
                {practiceChecks.map((check) => (
                  <div key={check.id} className="of-panel-muted" style={{ padding: 14 }}>
                    <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
                      <div>
                        <p style={{ fontWeight: 600, color: 'var(--text-strong)' }}>{check.label}</p>
                        <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
                          {check.detail}
                        </p>
                      </div>
                      <div style={{ textAlign: 'right' }}>
                        <span
                          style={{
                            display: 'inline-block',
                            padding: '4px 10px',
                            borderRadius: 999,
                            fontSize: 11,
                            textTransform: 'uppercase',
                            letterSpacing: '0.16em',
                            border: '1px solid',
                            ...(check.status === 'strong'
                              ? { background: '#ecfdf5', color: '#047857', borderColor: '#a7f3d0' }
                              : check.status === 'attention'
                                ? { background: '#fffbeb', color: '#b45309', borderColor: '#fde68a' }
                                : { background: '#fef2f2', color: '#b91c1c', borderColor: '#fecaca' }),
                          }}
                        >
                          {check.status}
                        </span>
                        <div style={{ marginTop: 6, fontSize: 18, fontWeight: 600, color: 'var(--text-strong)' }}>{check.score}</div>
                      </div>
                    </div>
                  </div>
                ))}
              </section>

              <section className="of-panel" style={{ padding: 20 }}>
                <p className="of-eyebrow">What to improve first</p>
                <div style={{ display: 'grid', gap: 10, marginTop: 12 }}>
                  {practiceChecks
                    .filter((check) => check.status !== 'strong')
                    .slice(0, 4)
                    .map((check) => (
                      <div key={check.id} className="of-panel-muted" style={{ padding: 14 }}>
                        <p style={{ fontWeight: 600, color: 'var(--text-strong)' }}>{check.label}</p>
                        <p className="of-text-muted" style={{ marginTop: 6, fontSize: 13 }}>
                          {check.detail}
                        </p>
                      </div>
                    ))}
                </div>
              </section>
            </div>
          )}

          {activeTab === 'anti-patterns' && (
            <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 1fr) 320px' }}>
              <section className="of-panel" style={{ padding: 20 }}>
                <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12, flexWrap: 'wrap' }}>
                  <div>
                    <p className="of-eyebrow">Detected anti-patterns</p>
                    <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
                      Signals are derived from current schema, action surface, naming, and collaboration structure.
                    </p>
                  </div>
                  <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
                    {SEVERITIES.map((severity) => {
                      const active = activeSeverity === severity;
                      return (
                        <button
                          key={severity}
                          type="button"
                          onClick={() => setActiveSeverity(severity)}
                          className={active ? 'of-button of-button--primary' : 'of-button'}
                          style={{ fontSize: 12 }}
                        >
                          {severity}
                        </button>
                      );
                    })}
                  </div>
                </div>

                <div style={{ display: 'grid', gap: 10, marginTop: 16 }}>
                  {visibleFindings.map((finding) => (
                    <div key={finding.id} className="of-panel-muted" style={{ padding: 14 }}>
                      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
                        <div>
                          <p style={{ fontWeight: 600, color: 'var(--text-strong)' }}>{finding.title}</p>
                          <p className="of-text-muted" style={{ marginTop: 4, fontSize: 11, textTransform: 'uppercase', letterSpacing: '0.16em' }}>
                            {finding.affected}
                          </p>
                        </div>
                        <span
                          style={{
                            padding: '4px 10px',
                            borderRadius: 999,
                            fontSize: 11,
                            textTransform: 'uppercase',
                            letterSpacing: '0.16em',
                            border: '1px solid',
                            ...(finding.severity === 'critical'
                              ? { background: '#fef2f2', color: '#b91c1c', borderColor: '#fecaca' }
                              : finding.severity === 'warning'
                                ? { background: '#fffbeb', color: '#b45309', borderColor: '#fde68a' }
                                : { background: 'var(--bg-subtle)', color: 'var(--text-muted)', borderColor: 'var(--border-default)' }),
                          }}
                        >
                          {finding.severity}
                        </span>
                      </div>
                      <p className="of-text-muted" style={{ marginTop: 10, fontSize: 13 }}>
                        {finding.summary}
                      </p>
                      <div className="of-panel" style={{ padding: 10, marginTop: 10 }}>
                        <p style={{ fontWeight: 600, color: 'var(--text-strong)' }}>Recommendation</p>
                        <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
                          {finding.recommendation}
                        </p>
                      </div>
                      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6, marginTop: 10 }}>
                        {finding.evidence.map((entry, index) => (
                          <span key={`${finding.id}-${index}`} className="of-chip">
                            {entry}
                          </span>
                        ))}
                      </div>
                      <div style={{ marginTop: 10 }}>
                        <button type="button" onClick={() => dismissFinding(finding.id)} className="of-button" style={{ fontSize: 12 }}>
                          Dismiss
                        </button>
                      </div>
                    </div>
                  ))}
                  {visibleFindings.length === 0 && (
                    <div
                      style={{
                        border: '1px dashed var(--border-default)',
                        borderRadius: 'var(--radius-md)',
                        padding: 32,
                        textAlign: 'center',
                        fontSize: 13,
                        color: 'var(--text-muted)',
                      }}
                    >
                      No active findings for the current severity filter.
                    </div>
                  )}
                </div>
              </section>

              <section className="of-panel" style={{ padding: 20 }}>
                <p className="of-eyebrow">Finding inventory</p>
                <div style={{ display: 'grid', gap: 10, marginTop: 12 }}>
                  {(['critical', 'warning', 'info'] as const).map((sev) => (
                    <div key={sev} className="of-panel-muted" style={{ padding: 14 }}>
                      <p style={{ fontWeight: 600, color: 'var(--text-strong)', textTransform: 'capitalize' }}>{sev}</p>
                      <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
                        {findings.filter((item) => item.severity === sev).length}
                      </p>
                    </div>
                  ))}
                  <button type="button" onClick={restoreDismissed} className="of-button" style={{ width: '100%', fontSize: 13 }}>
                    Restore dismissed
                  </button>
                </div>
              </section>
            </div>
          )}

          {activeTab === 'playbook' && (
            <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'repeat(auto-fit, minmax(360px, 1fr))' }}>
              <section className="of-panel" style={{ padding: 20 }}>
                <p className="of-eyebrow">Best practices</p>
                <div style={{ display: 'grid', gap: 10, marginTop: 12 }}>
                  {[
                    {
                      title: 'Model reality, not systems',
                      detail: 'Unify source-system variants into business entities and merge upstream in pipelines.',
                    },
                    {
                      title: 'Curate intentionally',
                      detail: 'Expose only properties that matter to workflows, search, views, and decisions.',
                    },
                    {
                      title: 'Collaborate across teams',
                      detail: 'Use shared ontologies and project memberships to keep one canonical model across departments.',
                    },
                    {
                      title: 'Keep object types focused',
                      detail: 'Split overloaded types and re-use interfaces when behavior is truly shared.',
                    },
                    {
                      title: 'Choose the right tool',
                      detail: 'Actions for decisions, pipelines for transforms, automations for reactions, functions for live complex logic.',
                    },
                    {
                      title: 'Document your decisions',
                      detail: 'Descriptions and precise naming reduce onboarding friction and prevent silent divergence.',
                    },
                  ].map((entry) => (
                    <div key={entry.title} className="of-panel-muted" style={{ padding: 14 }}>
                      <p style={{ fontWeight: 600, color: 'var(--text-strong)' }}>{entry.title}</p>
                      <p className="of-text-muted" style={{ marginTop: 6, fontSize: 13 }}>
                        {entry.detail}
                      </p>
                    </div>
                  ))}
                </div>
              </section>

              <section className="of-panel" style={{ padding: 20 }}>
                <p className="of-eyebrow">Fix path in OpenFoundry</p>
                <div style={{ display: 'grid', gap: 10, marginTop: 12 }}>
                  {[
                    {
                      href: '/object-link-types',
                      title: 'Use Object and Link Types',
                      detail: 'Split system silos, rename ambiguous types, curate properties, and reshape link semantics.',
                    },
                    {
                      href: '/interfaces',
                      title: 'Use Interfaces',
                      detail: 'Extract shared traits from wide object types instead of overloading one god object.',
                    },
                    {
                      href: '/action-types',
                      title: 'Use Action Types',
                      detail: 'Replace action sprawl with cohesive business operations and cleaner parameter flows.',
                    },
                    {
                      href: '/ontologies',
                      title: 'Use Ontologies',
                      detail: 'Run branch reviews, cross-team proposals, and shared-space governance when the model changes affect many teams.',
                    },
                    {
                      href: '/ontology-manager',
                      title: 'Use Ontology Manager',
                      detail: 'Save staged changes, inspect warnings, and govern import or export flows during remediations.',
                    },
                  ].map((entry) => (
                    <a
                      key={entry.href}
                      href={entry.href}
                      className="of-panel-muted"
                      style={{ padding: 14, display: 'block', textDecoration: 'none', color: 'inherit' }}
                    >
                      <p style={{ fontWeight: 600, color: 'var(--text-strong)' }}>{entry.title}</p>
                      <p className="of-text-muted" style={{ marginTop: 6, fontSize: 13 }}>
                        {entry.detail}
                      </p>
                    </a>
                  ))}
                </div>
              </section>
            </div>
          )}

          {activeTab === 'review' && (
            <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 1fr) 360px' }}>
              <section className="of-panel" style={{ padding: 20 }}>
                <p className="of-eyebrow">Design review notes</p>
                <label style={{ display: 'block', marginTop: 12, fontSize: 13 }}>
                  <span style={{ fontWeight: 600 }}>Capture remediation decisions, ownership, or open questions</span>
                  <textarea
                    rows={16}
                    value={reviewState.notes}
                    onChange={(e) => updateNotes(e.target.value)}
                    placeholder="Example: Merge Sales Customer and Support Customer into Customer, then move team-specific metadata into links and object views."
                    className="of-input"
                    style={{ marginTop: 8, fontSize: 13 }}
                  />
                </label>
              </section>

              <section className="of-panel" style={{ padding: 20 }}>
                <p className="of-eyebrow">Review summary</p>
                <div style={{ display: 'grid', gap: 10, marginTop: 12 }}>
                  {[
                    { label: 'Design score', value: designScore },
                    { label: 'Outstanding findings', value: visibleFindings.length },
                    { label: 'Dismissed findings', value: reviewState.dismissed.length },
                    {
                      label: 'Projects with multiple members',
                      value: projects.filter((project) => (projectMemberships[project.id] ?? []).length > 1).length,
                    },
                  ].map((row) => (
                    <div key={row.label} className="of-panel-muted" style={{ padding: 14 }}>
                      <p style={{ fontWeight: 600, color: 'var(--text-strong)' }}>{row.label}</p>
                      <p style={{ marginTop: 4, fontSize: 22, fontWeight: 600 }}>{row.value}</p>
                    </div>
                  ))}
                </div>
              </section>
            </div>
          )}
        </>
      )}
    </section>
  );
}
