import { useEffect, useMemo, useState } from 'react';

import {
  getMachineryInsights,
  getMachineryQueue,
  listObjectTypes,
  listRules,
  updateMachineryQueueItem,
  type MachineryInsight,
  type MachineryQueueItem,
  type MachineryQueueResponse,
  type ObjectType,
  type OntologyRule,
} from '@/lib/api/ontology';
import { notifications } from '@stores/notifications';

type ViewMode = 'week' | 'day' | 'agenda';

interface ScenarioPlacement {
  scheduled_for: string;
  required_capability: string;
}

interface TimelineSegment {
  key: string;
  start: Date;
  end: Date;
  label: string;
  secondaryLabel: string;
}

interface SchedulingSuggestion {
  itemId: string;
  capability: string;
  start: Date;
  end: Date;
  score: number;
  reason: string;
}

function startOfLocalDay(value: Date) {
  return new Date(value.getFullYear(), value.getMonth(), value.getDate(), 0, 0, 0, 0);
}

function toDateInput(value: Date) {
  const year = value.getFullYear();
  const month = String(value.getMonth() + 1).padStart(2, '0');
  const day = String(value.getDate()).padStart(2, '0');
  return `${year}-${month}-${day}`;
}

function parseDateInput(value: string) {
  if (!value) return startOfLocalDay(new Date());
  const parsed = new Date(`${value}T00:00:00`);
  return Number.isNaN(parsed.getTime()) ? startOfLocalDay(new Date()) : parsed;
}

function addMinutes(value: Date, minutes: number) {
  return new Date(value.getTime() + minutes * 60_000);
}

function addHours(value: Date, hours: number) {
  return addMinutes(value, hours * 60);
}

function addDays(value: Date, days: number) {
  return addHours(value, days * 24);
}

function formatTimestamp(value: string | Date | null | undefined) {
  if (!value) return 'n/a';
  const date = value instanceof Date ? value : new Date(value);
  return new Intl.DateTimeFormat('en', {
    month: 'short',
    day: 'numeric',
    hour: 'numeric',
    minute: '2-digit',
  }).format(date);
}

function formatDuration(minutes: number) {
  if (minutes >= 60) {
    const hours = Math.floor(minutes / 60);
    const remainder = minutes % 60;
    return remainder === 0 ? `${hours}h` : `${hours}h ${remainder}m`;
  }
  return `${minutes}m`;
}

function capabilityLabel(capability: string | null | undefined) {
  const normalized = (capability || 'general').replaceAll('_', ' ');
  return normalized.charAt(0).toUpperCase() + normalized.slice(1);
}

function intervalsOverlap(leftStart: Date, leftEnd: Date, rightStart: Date, rightEnd: Date) {
  return leftStart < rightEnd && rightStart < leftEnd;
}

function statusTone(status: string): React.CSSProperties {
  if (status === 'pending') return { background: '#fffbeb', color: '#b45309' };
  if (status === 'in_progress') return { background: '#eff6ff', color: '#1d4ed8' };
  return { background: '#ecfdf5', color: '#047857' };
}

export function DynamicSchedulingPage() {
  const [objectTypes, setObjectTypes] = useState<ObjectType[]>([]);
  const [rules, setRules] = useState<OntologyRule[]>([]);
  const [insights, setInsights] = useState<MachineryInsight[]>([]);
  const [queue, setQueue] = useState<MachineryQueueResponse | null>(null);
  const [selectedObjectTypeId, setSelectedObjectTypeId] = useState('');
  const [viewMode, setViewMode] = useState<ViewMode>('week');
  const [horizonStartInput, setHorizonStartInput] = useState(toDateInput(new Date()));
  const [selectedItemId, setSelectedItemId] = useState('');
  const [draggingItemId, setDraggingItemId] = useState('');
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');
  const [scenarioEdits, setScenarioEdits] = useState<Record<string, ScenarioPlacement>>({});

  const horizonStart = useMemo(() => parseDateInput(horizonStartInput), [horizonStartInput]);
  const horizonEnd = useMemo(
    () => (viewMode === 'day' ? addHours(horizonStart, 24) : addDays(horizonStart, 7)),
    [horizonStart, viewMode],
  );

  const segments = useMemo<TimelineSegment[]>(() => {
    if (viewMode === 'agenda') return [];
    const stepHours = viewMode === 'day' ? 2 : 6;
    const segmentCount = viewMode === 'day' ? 12 : 28;
    return Array.from({ length: segmentCount }, (_, index) => {
      const segmentStart = addHours(horizonStart, stepHours * index);
      const segmentEnd = addHours(segmentStart, stepHours);
      const major = index === 0 || segmentStart.getHours() === 0;
      return {
        key: `${segmentStart.toISOString()}-${index}`,
        start: segmentStart,
        end: segmentEnd,
        label: major
          ? new Intl.DateTimeFormat('en', { weekday: 'short', month: 'short', day: 'numeric' }).format(segmentStart)
          : new Intl.DateTimeFormat('en', { hour: 'numeric' }).format(segmentStart),
        secondaryLabel: new Intl.DateTimeFormat('en', { hour: 'numeric' }).format(segmentStart),
      };
    });
  }, [viewMode, horizonStart]);

  function effectiveCapability(item: MachineryQueueItem) {
    return scenarioEdits[item.id]?.required_capability || item.required_capability || 'general';
  }
  function effectiveStart(item: MachineryQueueItem) {
    return new Date(scenarioEdits[item.id]?.scheduled_for || item.scheduled_for);
  }
  function effectiveEnd(item: MachineryQueueItem) {
    return addMinutes(effectiveStart(item), Math.max(item.estimated_duration_minutes, 30));
  }
  function isPendingLike(item: MachineryQueueItem) {
    return item.status === 'pending' || item.status === 'in_progress';
  }

  const timelineItems = useMemo(
    () =>
      (queue?.data ?? [])
        .filter(isPendingLike)
        .sort((left, right) => effectiveStart(left).getTime() - effectiveStart(right).getTime()),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [queue, scenarioEdits],
  );

  const agendaItems = useMemo(
    () =>
      [...(queue?.data ?? [])].sort((left, right) => effectiveStart(left).getTime() - effectiveStart(right).getTime()),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [queue, scenarioEdits],
  );

  const capabilities = useMemo(() => {
    const values = new Set<string>();
    for (const item of timelineItems) values.add(effectiveCapability(item));
    for (const cap of queue?.recommendation.capability_load ?? []) values.add(cap.capability);
    if (values.size === 0) values.add('general');
    return [...values].sort((left, right) => left.localeCompare(right));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [timelineItems, queue, scenarioEdits]);

  function rowItems(capability: string) {
    return timelineItems.filter((item) => effectiveCapability(item) === capability);
  }

  function itemConflicts(item: MachineryQueueItem) {
    const itemStart = effectiveStart(item);
    const itemEnd = effectiveEnd(item);
    return timelineItems.filter((candidate) => {
      if (candidate.id === item.id) return false;
      if (effectiveCapability(candidate) !== effectiveCapability(item)) return false;
      return intervalsOverlap(itemStart, itemEnd, effectiveStart(candidate), effectiveEnd(candidate));
    });
  }

  function recommendationRank(itemId: string) {
    const rank = queue?.recommendation.recommended_order.indexOf(itemId) ?? -1;
    return rank >= 0 ? rank + 1 : null;
  }

  function itemStyle(item: MachineryQueueItem): React.CSSProperties {
    const start = effectiveStart(item);
    const end = effectiveEnd(item);
    const horizonMs = horizonEnd.getTime() - horizonStart.getTime();
    const clampedStart = Math.max(start.getTime(), horizonStart.getTime());
    const clampedEnd = Math.min(end.getTime(), horizonEnd.getTime());
    const leftPct = ((clampedStart - horizonStart.getTime()) / horizonMs) * 100;
    const widthPct = Math.max(((clampedEnd - clampedStart) / horizonMs) * 100, 4);
    return { left: `${leftPct}%`, width: `${widthPct}%` };
  }

  function rowUtilization(capability: string) {
    const totalMinutes = viewMode === 'day' ? 24 * 60 : 7 * 24 * 60;
    const usedMinutes = rowItems(capability).reduce(
      (sum, item) => sum + Math.max(item.estimated_duration_minutes, 30),
      0,
    );
    return Math.min(100, Math.round((usedMinutes / totalMinutes) * 100));
  }

  function stageItemPlacement(itemId: string, capability: string, start: Date) {
    setScenarioEdits((current) => ({
      ...current,
      [itemId]: { required_capability: capability, scheduled_for: start.toISOString() },
    }));
    setSelectedItemId(itemId);
  }

  function clearScenario() {
    setScenarioEdits({});
  }

  function clearItemScenario(itemId: string) {
    setScenarioEdits((current) => {
      const next = { ...current };
      delete next[itemId];
      return next;
    });
  }

  const selectedItem = agendaItems.find((item) => item.id === selectedItemId) ?? null;

  const selectedSuggestions = useMemo<SchedulingSuggestion[]>(() => {
    if (!selectedItem || viewMode === 'agenda') return [];
    const durationMinutes = Math.max(selectedItem.estimated_duration_minutes, 30);
    const items = timelineItems;
    const suggestions: SchedulingSuggestion[] = [];

    for (const capability of capabilities) {
      for (const segment of segments) {
        const candidateStart = segment.start;
        const candidateEnd = addMinutes(candidateStart, durationMinutes);
        if (candidateEnd > horizonEnd) continue;

        const overlapping = items.some((candidate) => {
          if (candidate.id === selectedItem.id) return false;
          if (effectiveCapability(candidate) !== capability) return false;
          return intervalsOverlap(candidateStart, candidateEnd, effectiveStart(candidate), effectiveEnd(candidate));
        });
        if (overlapping) continue;

        const sameCapabilityBonus = capability === (selectedItem.required_capability || 'general') ? 40 : 0;
        const overduePenalty = selectedItem.status === 'pending' && candidateStart < new Date() ? -15 : 0;
        const score = sameCapabilityBonus - overduePenalty - suggestions.length;

        suggestions.push({
          itemId: selectedItem.id,
          capability,
          start: candidateStart,
          end: candidateEnd,
          score,
          reason:
            capability === (selectedItem.required_capability || 'general')
              ? 'Matches required capability and avoids overlaps.'
              : 'Alternative row with no detected overlap in the visible horizon.',
        });
      }
    }
    return suggestions
      .sort((left, right) => right.score - left.score || left.start.getTime() - right.start.getTime())
      .slice(0, 3);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [selectedItem, viewMode, segments, capabilities, timelineItems, scenarioEdits, horizonEnd]);

  function validationSummary(item: MachineryQueueItem) {
    const conflicts = itemConflicts(item);
    const validations: Array<{ label: string; tone: 'danger' | 'success' | 'warning' | 'info'; detail: string }> = [];
    if (conflicts.length > 0) {
      validations.push({
        label: 'Overlap conflict',
        tone: 'danger',
        detail: `${conflicts.length} conflicting puck(s) on ${capabilityLabel(effectiveCapability(item))}.`,
      });
    } else {
      validations.push({
        label: 'No overlap',
        tone: 'success',
        detail: 'No conflicting assignment detected for the selected resource lane.',
      });
    }
    if (item.status === 'pending' && effectiveStart(item) < new Date()) {
      validations.push({
        label: 'Overdue',
        tone: 'warning',
        detail: 'The item is still pending even though its scheduled window has already started.',
      });
    }
    if (Object.keys(item.constraint_snapshot ?? {}).length > 0) {
      validations.push({
        label: 'Constraint snapshot',
        tone: 'info',
        detail: 'Constraint metadata is attached and visible to the scheduling surface.',
      });
    }
    return validations;
  }

  function timelineHighlight(capability: string, segment: TimelineSegment) {
    return selectedSuggestions.some(
      (suggestion) => suggestion.capability === capability && suggestion.start.getTime() === segment.start.getTime(),
    );
  }

  async function refreshSurface(typeId = selectedObjectTypeId, preferredItemId = selectedItemId) {
    if (!typeId) return;
    setLoading(true);
    setError('');
    try {
      const [queueResponse, insightResponse, ruleResponse] = await Promise.all([
        getMachineryQueue({ object_type_id: typeId }),
        getMachineryInsights({ object_type_id: typeId }),
        listRules({ object_type_id: typeId, page: 1, per_page: 100 }),
      ]);
      setQueue(queueResponse);
      setInsights(insightResponse.data);
      setRules(ruleResponse.data);
      if (preferredItemId && queueResponse.data.some((item) => item.id === preferredItemId)) {
        setSelectedItemId(preferredItemId);
      } else {
        setSelectedItemId(queueResponse.data[0]?.id ?? '');
      }
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'Failed to load dynamic scheduling surface');
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    let cancelled = false;
    async function load() {
      setLoading(true);
      setError('');
      try {
        const response = await listObjectTypes({ page: 1, per_page: 100 });
        if (cancelled) return;
        setObjectTypes(response.data);
        const nextId = response.data[0]?.id ?? '';
        setSelectedObjectTypeId(nextId);
        await refreshSurface(nextId, '');
      } catch (cause) {
        if (cancelled) return;
        setError(cause instanceof Error ? cause.message : 'Failed to load ontology object types');
        setLoading(false);
      }
    }
    void load();
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function transitionItemStatus(itemId: string, status: string) {
    setBusy(true);
    try {
      await updateMachineryQueueItem(itemId, { status });
      notifications.success(`Queue item moved to ${status.replaceAll('_', ' ')}.`);
      await refreshSurface(selectedObjectTypeId, itemId);
    } catch (cause) {
      const message = cause instanceof Error ? cause.message : 'Failed to update queue item';
      notifications.error(message);
    } finally {
      setBusy(false);
    }
  }

  function moveHorizon(step: number) {
    const next = addDays(horizonStart, step);
    setHorizonStartInput(toDateInput(next));
  }

  function onDropPlacement(capability: string, segment: TimelineSegment) {
    if (!draggingItemId) return;
    stageItemPlacement(draggingItemId, capability, segment.start);
    setDraggingItemId('');
  }

  const scenarioCount = Object.keys(scenarioEdits).length;

  return (
    <section className="of-page" style={{ display: 'grid', gap: 16 }}>
      <div className="of-panel" style={{ padding: 24 }}>
        <div style={{ display: 'grid', gap: 24, gridTemplateColumns: 'minmax(0, 1.2fr) 360px' }}>
          <div>
            <p className="of-eyebrow">Dynamic scheduling</p>
            <h1 className="of-heading-xl" style={{ marginTop: 8 }}>
              Schedule ontology work across resource lanes, scenario staging, and constraint-aware
              queue operations.
            </h1>
            <p className="of-text-muted" style={{ marginTop: 12, fontSize: 14, lineHeight: 1.7 }}>
              This surface turns the machinery queue into a dedicated scheduling application. Teams
              can visualize schedules and constraints, stage drag-and-drop changes locally, inspect
              validation pressure, and dispatch operational queue transitions from one place.
            </p>
            <div style={{ marginTop: 16, display: 'flex', flexWrap: 'wrap', gap: 8 }}>
              <a href="/ontology" className="of-button">
                Back to ontology
              </a>
              <a href="/ontology/graph" className="of-button">
                Open graph
              </a>
            </div>
          </div>

          <div className="of-panel-muted" style={{ padding: 16 }}>
            <p className="of-eyebrow">Core concepts</p>
            <div style={{ display: 'grid', gap: 8, marginTop: 12 }}>
              {[
                {
                  title: 'Schedule objects',
                  detail:
                    'Queue items behave like schedule pucks with start time, duration, priority, and resource affinity.',
                },
                {
                  title: 'Resource rows',
                  detail: 'Required capabilities become the operational resource lanes used for load and conflict analysis.',
                },
                {
                  title: 'Scenario staging',
                  detail: 'Dragging a puck stages a local what-if plan without mutating the backend queue until you promote an action.',
                },
              ].map((card) => (
                <article key={card.title} className="of-panel" style={{ padding: 12 }}>
                  <div style={{ fontWeight: 600, color: 'var(--text-strong)' }}>{card.title}</div>
                  <div className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
                    {card.detail}
                  </div>
                </article>
              ))}
            </div>
          </div>
        </div>
      </div>

      <section className="of-panel" style={{ padding: 20 }}>
        <div style={{ display: 'grid', gap: 12, gridTemplateColumns: 'minmax(0, 1fr) auto', alignItems: 'flex-end' }}>
          <div style={{ display: 'grid', gap: 12, gridTemplateColumns: 'repeat(auto-fit, minmax(180px, 1fr))' }}>
            <label style={{ fontSize: 13, display: 'grid', gap: 4 }}>
              <span style={{ fontWeight: 600 }}>Schedule object type</span>
              <select
                value={selectedObjectTypeId}
                onChange={(e) => {
                  setSelectedObjectTypeId(e.target.value);
                  void refreshSurface(e.target.value);
                }}
                className="of-input"
              >
                {objectTypes.map((objectType) => (
                  <option key={objectType.id} value={objectType.id}>
                    {objectType.display_name}
                  </option>
                ))}
              </select>
            </label>
            <label style={{ fontSize: 13, display: 'grid', gap: 4 }}>
              <span style={{ fontWeight: 600 }}>View</span>
              <div style={{ display: 'flex', gap: 4 }}>
                {(['week', 'day', 'agenda'] as ViewMode[]).map((mode) => (
                  <button
                    key={mode}
                    type="button"
                    onClick={() => setViewMode(mode)}
                    className={viewMode === mode ? 'of-button of-button--primary' : 'of-button'}
                    style={{ fontSize: 12, textTransform: 'capitalize', flex: 1 }}
                  >
                    {mode}
                  </button>
                ))}
              </div>
            </label>
            <label style={{ fontSize: 13, display: 'grid', gap: 4 }}>
              <span style={{ fontWeight: 600 }}>Horizon start</span>
              <input
                type="date"
                value={horizonStartInput}
                onChange={(e) => setHorizonStartInput(e.target.value)}
                className="of-input"
              />
            </label>
            <div style={{ display: 'grid', gap: 4 }}>
              <span style={{ fontWeight: 600, fontSize: 13 }}>Scenario</span>
              <div style={{ display: 'flex', gap: 6, alignItems: 'center' }}>
                <span className="of-chip">{scenarioCount} staged</span>
                <button type="button" onClick={clearScenario} disabled={scenarioCount === 0} className="of-button" style={{ fontSize: 12 }}>
                  Clear
                </button>
              </div>
            </div>
          </div>

          <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
            <button type="button" onClick={() => moveHorizon(viewMode === 'day' ? -1 : -7)} className="of-button">
              Previous
            </button>
            <button type="button" onClick={() => setHorizonStartInput(toDateInput(new Date()))} className="of-button">
              Today
            </button>
            <button type="button" onClick={() => moveHorizon(viewMode === 'day' ? 1 : 7)} className="of-button">
              Next
            </button>
            <button type="button" onClick={() => void refreshSurface()} disabled={busy} className="of-button of-button--primary">
              Refresh queue
            </button>
          </div>
        </div>
      </section>

      {error && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {error}
        </div>
      )}

      {queue && (
        <section style={{ display: 'grid', gap: 12, gridTemplateColumns: 'repeat(auto-fit, minmax(220px, 1fr))' }}>
          {[
            {
              label: 'Queue depth',
              value: queue.recommendation.queue_depth,
              detail: 'Pending or in-progress pucks in the visible machinery queue.',
            },
            {
              label: 'Overdue',
              value: queue.recommendation.overdue_count,
              detail: 'Items whose scheduled window has already started without completion.',
            },
            {
              label: 'Capacity load',
              value: `${queue.recommendation.total_estimated_minutes}m`,
              detail: 'Estimated runtime minutes stacked in the recommendation engine.',
            },
            {
              label: 'Next due',
              value: formatTimestamp(queue.recommendation.next_due_at),
              detail: queue.recommendation.strategy,
            },
          ].map((card) => (
            <article key={card.label} className="of-panel" style={{ padding: 16 }}>
              <p className="of-eyebrow">{card.label}</p>
              <p style={{ marginTop: 6, fontSize: 22, fontWeight: 600, color: 'var(--text-strong)' }}>{card.value}</p>
              <p className="of-text-muted" style={{ marginTop: 4, fontSize: 12 }}>
                {card.detail}
              </p>
            </article>
          ))}
        </section>
      )}

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 1fr) 340px' }}>
        <section style={{ display: 'grid', gap: 16 }}>
          <section className="of-panel" style={{ overflow: 'hidden' }}>
            <div style={{ borderBottom: '1px solid var(--border-default)', background: 'var(--bg-subtle)', padding: '14px 20px' }}>
              <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
                <div>
                  <div className="of-heading-md">Scheduling board</div>
                  <div className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
                    Drag a puck onto another resource lane or time segment to stage a scenario move.
                  </div>
                </div>
                {queue && (
                  <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
                    {queue.recommendation.capability_load.map((capability) => (
                      <span key={capability.capability} className="of-chip">
                        {capabilityLabel(capability.capability)} {capability.pending_count}
                      </span>
                    ))}
                  </div>
                )}
              </div>
            </div>

            <div style={{ padding: 20 }}>
              {loading ? (
                <div
                  style={{
                    border: '1px dashed var(--border-default)',
                    borderRadius: 'var(--radius-md)',
                    padding: 48,
                    textAlign: 'center',
                    fontSize: 13,
                    color: 'var(--text-muted)',
                  }}
                >
                  Loading dynamic scheduling workspace…
                </div>
              ) : !queue || queue.data.length === 0 ? (
                <div
                  style={{
                    border: '1px dashed var(--border-default)',
                    borderRadius: 'var(--radius-md)',
                    padding: 48,
                    textAlign: 'center',
                    fontSize: 13,
                    color: 'var(--text-muted)',
                  }}
                >
                  No scheduling queue items are available yet for this ontology object type.
                </div>
              ) : viewMode === 'agenda' ? (
                <div style={{ display: 'grid', gap: 10 }}>
                  {agendaItems.map((item) => (
                    <article
                      key={item.id}
                      style={{
                        padding: 14,
                        border: '1px solid',
                        borderColor: selectedItemId === item.id ? '#9fb9ec' : 'var(--border-default)',
                        background: selectedItemId === item.id ? '#f8fbff' : 'var(--bg-elevated)',
                        borderRadius: 'var(--radius-md)',
                      }}
                    >
                      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 8 }}>
                        <div>
                          <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 6 }}>
                            <button
                              type="button"
                              onClick={() => setSelectedItemId(item.id)}
                              style={{ background: 'none', border: 'none', padding: 0, fontSize: 15, fontWeight: 600, color: 'var(--text-strong)', cursor: 'pointer', textAlign: 'left' }}
                            >
                              {item.rule_display_name}
                            </button>
                            <span className="of-chip" style={statusTone(item.status)}>
                              {item.status.replaceAll('_', ' ')}
                            </span>
                            {recommendationRank(item.id) && (
                              <span className="of-chip">Rank {recommendationRank(item.id)}</span>
                            )}
                            {scenarioEdits[item.id] && <span className="of-chip">Staged</span>}
                          </div>
                          <div className="of-text-muted" style={{ marginTop: 6, fontSize: 13 }}>
                            {formatTimestamp(effectiveStart(item))} to {formatTimestamp(effectiveEnd(item))}
                          </div>
                        </div>
                        <div style={{ textAlign: 'right', fontSize: 13, color: 'var(--text-muted)' }}>
                          <div>{capabilityLabel(effectiveCapability(item))}</div>
                          <div style={{ marginTop: 4 }}>{formatDuration(Math.max(item.estimated_duration_minutes, 30))}</div>
                        </div>
                      </div>
                    </article>
                  ))}
                </div>
              ) : (
                <div style={{ overflowX: 'auto', paddingBottom: 8 }}>
                  <div style={{ minWidth: 980, display: 'grid', gridTemplateColumns: '220px minmax(0, 1fr)', gap: 0 }}>
                    <div
                      style={{
                        borderBottom: '1px solid var(--border-default)',
                        background: 'var(--bg-subtle)',
                        padding: '10px 16px',
                        fontSize: 11,
                        fontWeight: 600,
                        textTransform: 'uppercase',
                        letterSpacing: '0.16em',
                      }}
                    >
                      Resource rows
                    </div>
                    <div style={{ borderBottom: '1px solid var(--border-default)', display: 'grid', gridTemplateColumns: `repeat(${segments.length}, minmax(0, 1fr))` }}>
                      {segments.map((segment, index) => (
                        <div
                          key={segment.key}
                          style={{
                            borderLeft: index === 0 ? 'none' : '1px solid var(--border-default)',
                            padding: '10px 6px',
                            textAlign: 'center',
                          }}
                        >
                          <div style={{ fontSize: 12, fontWeight: 600 }}>{segment.label}</div>
                          <div style={{ marginTop: 4, fontSize: 11, color: 'var(--text-muted)' }}>{segment.secondaryLabel}</div>
                        </div>
                      ))}
                    </div>

                    {capabilities.map((capability) => (
                      <div key={`row-${capability}`} style={{ display: 'contents' }}>
                        <div
                          style={{
                            borderBottom: '1px solid var(--border-default)',
                            background: 'var(--bg-subtle)',
                            padding: 14,
                          }}
                        >
                          <div style={{ fontSize: 13, fontWeight: 600, color: 'var(--text-strong)' }}>
                            {capabilityLabel(capability)}
                          </div>
                          <div className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
                            {rowItems(capability).length} scheduled item(s)
                          </div>
                          <div style={{ marginTop: 10 }}>
                            <div style={{ height: 6, borderRadius: 999, background: '#eef2f7' }}>
                              <div style={{ height: '100%', borderRadius: 999, background: '#7aa2e8', width: `${rowUtilization(capability)}%` }} />
                            </div>
                            <div className="of-text-muted" style={{ marginTop: 6, fontSize: 11 }}>
                              {rowUtilization(capability)}% visible load
                            </div>
                          </div>
                        </div>

                        <div style={{ borderBottom: '1px solid var(--border-default)', background: 'var(--bg-elevated)', padding: 0 }}>
                          <div style={{ position: 'relative', height: 92 }}>
                            <div
                              style={{
                                display: 'grid',
                                height: '100%',
                                gridTemplateColumns: `repeat(${segments.length}, minmax(0, 1fr))`,
                              }}
                            >
                              {segments.map((segment, index) => (
                                <div
                                  key={segment.key}
                                  role="button"
                                  tabIndex={-1}
                                  aria-label={`Drop ${capabilityLabel(capability)} puck at ${formatTimestamp(segment.start)}`}
                                  onDragOver={(event) => event.preventDefault()}
                                  onDrop={() => onDropPlacement(capability, segment)}
                                  style={{
                                    borderLeft: index === 0 ? 'none' : '1px solid var(--border-default)',
                                    background: timelineHighlight(capability, segment) ? '#eef6ff' : undefined,
                                  }}
                                />
                              ))}
                            </div>

                            {rowItems(capability).map((item) => (
                              <button
                                key={item.id}
                                type="button"
                                draggable
                                onClick={() => setSelectedItemId(item.id)}
                                onDragStart={() => {
                                  setDraggingItemId(item.id);
                                  setSelectedItemId(item.id);
                                }}
                                onDragEnd={() => setDraggingItemId('')}
                                style={{
                                  position: 'absolute',
                                  top: 12,
                                  height: 54,
                                  overflow: 'hidden',
                                  borderRadius: 16,
                                  padding: '8px 12px',
                                  textAlign: 'left',
                                  cursor: 'pointer',
                                  zIndex: selectedItemId === item.id ? 20 : 10,
                                  border: `1px solid ${selectedItemId === item.id ? '#5c8fe3' : item.status === 'in_progress' ? '#8ad3c6' : '#dfd8c8'}`,
                                  background: selectedItemId === item.id ? '#eaf2ff' : item.status === 'in_progress' ? '#f2fcfa' : '#fff7e7',
                                  boxShadow: itemConflicts(item).length > 0 ? '0 0 0 2px #ef9b9b' : 'none',
                                  ...itemStyle(item),
                                }}
                              >
                                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
                                  <span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', fontSize: 13, fontWeight: 600, color: 'var(--text-strong)' }}>
                                    {item.rule_display_name}
                                  </span>
                                  {recommendationRank(item.id) && (
                                    <span style={{ background: 'rgba(255,255,255,0.85)', padding: '2px 6px', borderRadius: 999, fontSize: 10, fontWeight: 600, color: 'var(--text-muted)' }}>
                                      #{recommendationRank(item.id)}
                                    </span>
                                  )}
                                </div>
                                <div className="of-text-muted" style={{ marginTop: 4, fontSize: 11, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                                  {formatDuration(Math.max(item.estimated_duration_minutes, 30))} · {formatTimestamp(effectiveStart(item))}
                                </div>
                                {scenarioEdits[item.id] && (
                                  <div style={{ marginTop: 6, fontSize: 10, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.14em', color: '#2b5bb7' }}>
                                    Scenario staged
                                  </div>
                                )}
                              </button>
                            ))}
                          </div>
                        </div>
                      </div>
                    ))}
                  </div>
                </div>
              )}
            </div>
          </section>

          <section className="of-panel" style={{ overflow: 'hidden' }}>
            <div style={{ borderBottom: '1px solid var(--border-default)', background: 'var(--bg-subtle)', padding: '14px 20px' }}>
              <div className="of-heading-md">Validation rules and queue pressure</div>
              <div className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
                Constraint signals that shape scheduling recommendations and overload visibility.
              </div>
            </div>
            <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 1fr) 320px', padding: 20 }}>
              <div style={{ display: 'grid', gap: 10 }}>
                {insights.length === 0 ? (
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
                    No dynamic scheduling insights have been generated yet.
                  </div>
                ) : (
                  insights.map((insight) => (
                    <article key={insight.rule_id} className="of-panel-muted" style={{ padding: 14 }}>
                      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 8 }}>
                        <div>
                          <div style={{ fontSize: 13, fontWeight: 600, color: 'var(--text-strong)' }}>{insight.display_name}</div>
                          <div className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
                            {insight.pending_schedules} pending · {insight.overdue_schedules} overdue ·{' '}
                            {insight.matched_runs}/{insight.total_runs} matched runs
                          </div>
                        </div>
                        <span
                          className="of-chip"
                          style={
                            insight.dynamic_pressure === 'elevated' || insight.dynamic_pressure === 'critical'
                              ? { background: '#fffbeb', color: '#b45309' }
                              : { background: '#ecfdf5', color: '#047857' }
                          }
                        >
                          {insight.dynamic_pressure}
                        </span>
                      </div>
                    </article>
                  ))
                )}
              </div>

              <div className="of-panel-muted" style={{ padding: 14 }}>
                <div style={{ fontSize: 13, fontWeight: 600, color: 'var(--text-strong)' }}>Builder guidance</div>
                <ul className="of-text-muted" style={{ marginTop: 10, paddingLeft: 18, fontSize: 13, lineHeight: 1.7 }}>
                  <li>Use resource lanes to model the rows of the scheduling board.</li>
                  <li>Use queue items as schedule pucks with duration and priority.</li>
                  <li>Use rule pressure and conflict detection as validation rules.</li>
                  <li>Use scenario staging before promoting queue changes into runtime operations.</li>
                </ul>
                <div
                  style={{
                    marginTop: 12,
                    border: '1px dashed var(--border-default)',
                    borderRadius: 'var(--radius-md)',
                    padding: '10px 12px',
                    fontSize: 13,
                    color: 'var(--text-muted)',
                  }}
                >
                  Rules configured for this object type: <span style={{ fontWeight: 600, color: 'var(--text-strong)' }}>{rules.length}</span>
                  .
                </div>
              </div>
            </div>
          </section>
        </section>

        <aside style={{ display: 'grid', gap: 16 }}>
          <section className="of-panel" style={{ overflow: 'hidden' }}>
            <div style={{ borderBottom: '1px solid var(--border-default)', background: 'var(--bg-subtle)', padding: '12px 16px' }}>
              <div className="of-heading-md">Selected puck</div>
              <div className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
                Inspect validations, recommendations, and queue actions.
              </div>
            </div>

            <div style={{ display: 'grid', gap: 12, padding: 16 }}>
              {selectedItem ? (
                <>
                  <div>
                    <div style={{ fontSize: 16, fontWeight: 600, color: 'var(--text-strong)' }}>{selectedItem.rule_display_name}</div>
                    <div className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
                      {capabilityLabel(effectiveCapability(selectedItem))}
                    </div>
                    <div style={{ marginTop: 10, display: 'flex', flexWrap: 'wrap', gap: 6 }}>
                      <span className="of-chip" style={statusTone(selectedItem.status)}>
                        {selectedItem.status.replaceAll('_', ' ')}
                      </span>
                      {scenarioEdits[selectedItem.id] && <span className="of-chip">Scenario staged</span>}
                      {recommendationRank(selectedItem.id) && (
                        <span className="of-chip">Recommendation #{recommendationRank(selectedItem.id)}</span>
                      )}
                    </div>
                  </div>

                  <div className="of-panel-muted" style={{ padding: 14 }}>
                    <div style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.16em', color: 'var(--text-muted)' }}>
                      Schedule
                    </div>
                    <div style={{ marginTop: 6, fontSize: 13 }}>{formatTimestamp(effectiveStart(selectedItem))}</div>
                    <div className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
                      {formatDuration(Math.max(selectedItem.estimated_duration_minutes, 30))} · ends{' '}
                      {formatTimestamp(effectiveEnd(selectedItem))}
                    </div>
                  </div>

                  <div style={{ display: 'grid', gap: 8 }}>
                    <div style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.16em', color: 'var(--text-muted)' }}>
                      Validation rules
                    </div>
                    {validationSummary(selectedItem).map((validation, index) => (
                      <article key={index} className="of-panel-muted" style={{ padding: 12 }}>
                        <span
                          className="of-chip"
                          style={
                            validation.tone === 'danger'
                              ? { background: '#fef2f2', color: '#b91c1c' }
                              : validation.tone === 'warning'
                                ? { background: '#fffbeb', color: '#b45309' }
                                : validation.tone === 'success'
                                  ? { background: '#ecfdf5', color: '#047857' }
                                  : { background: '#eff6ff', color: '#1d4ed8' }
                          }
                        >
                          {validation.label}
                        </span>
                        <div className="of-text-muted" style={{ marginTop: 6, fontSize: 13 }}>
                          {validation.detail}
                        </div>
                      </article>
                    ))}
                  </div>

                  <div style={{ display: 'grid', gap: 8 }}>
                    <div style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.16em', color: 'var(--text-muted)' }}>
                      Suggestion function
                    </div>
                    {selectedSuggestions.length === 0 ? (
                      <div
                        style={{
                          border: '1px dashed var(--border-default)',
                          borderRadius: 'var(--radius-md)',
                          padding: 14,
                          fontSize: 13,
                          color: 'var(--text-muted)',
                        }}
                      >
                        No visible slot suggestions are available in the current horizon.
                      </div>
                    ) : (
                      selectedSuggestions.map((suggestion, index) => (
                        <button
                          key={`${suggestion.capability}-${suggestion.start.toISOString()}-${index}`}
                          type="button"
                          onClick={() => stageItemPlacement(selectedItem.id, suggestion.capability, suggestion.start)}
                          className="of-panel-muted"
                          style={{ padding: 12, textAlign: 'left', cursor: 'pointer' }}
                        >
                          <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
                            <div style={{ fontSize: 13, fontWeight: 600, color: 'var(--text-strong)' }}>
                              {capabilityLabel(suggestion.capability)}
                            </div>
                            <span className="of-chip">Score {suggestion.score}</span>
                          </div>
                          <div className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
                            {formatTimestamp(suggestion.start)} to {formatTimestamp(suggestion.end)}
                          </div>
                          <div className="of-text-muted" style={{ marginTop: 6, fontSize: 13 }}>
                            {suggestion.reason}
                          </div>
                        </button>
                      ))
                    )}
                  </div>

                  <div style={{ display: 'grid', gap: 8 }}>
                    <div style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.16em', color: 'var(--text-muted)' }}>
                      Operational actions
                    </div>
                    <div style={{ display: 'grid', gap: 6, gridTemplateColumns: '1fr 1fr' }}>
                      <button
                        type="button"
                        onClick={() => void transitionItemStatus(selectedItem.id, 'in_progress')}
                        disabled={busy}
                        className="of-button of-button--primary"
                      >
                        Start
                      </button>
                      <button
                        type="button"
                        onClick={() => void transitionItemStatus(selectedItem.id, 'completed')}
                        disabled={busy}
                        className="of-button"
                      >
                        Complete
                      </button>
                      <button
                        type="button"
                        onClick={() => void transitionItemStatus(selectedItem.id, 'pending')}
                        disabled={busy}
                        className="of-button"
                      >
                        Reset
                      </button>
                      <button
                        type="button"
                        onClick={() => void transitionItemStatus(selectedItem.id, 'cancelled')}
                        disabled={busy}
                        className="of-button"
                      >
                        Cancel
                      </button>
                    </div>
                    <button
                      type="button"
                      onClick={() => clearItemScenario(selectedItem.id)}
                      disabled={!scenarioEdits[selectedItem.id]}
                      className="of-button"
                      style={{ width: '100%' }}
                    >
                      Clear staged move
                    </button>
                  </div>
                </>
              ) : (
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
                  Select a queue puck to inspect validation rules, suggestions, and actions.
                </div>
              )}
            </div>
          </section>
        </aside>
      </div>
    </section>
  );
}
