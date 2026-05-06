import { useEffect, useState } from 'react';

import { ContractManager, type ContractDraft } from '@/lib/components/nexus/ContractManager';
import { PeerList, type PeerDraft } from '@/lib/components/nexus/PeerList';
import { ShareWizard, type ShareDraft } from '@/lib/components/nexus/ShareWizard';
import { SharedDataBrowser, type QueryDraft } from '@/lib/components/nexus/SharedDataBrowser';
import { SharingDashboard } from '@/lib/components/nexus/SharingDashboard';
import { SpaceManager, type SpaceDraft } from '@/lib/components/nexus/SpaceManager';
import {
  authenticatePeer,
  createContract,
  createPeer,
  createShare,
  createSpace,
  getAuditBridge,
  getOverview,
  listContracts,
  listPeers,
  listReplicationPlans,
  listShares,
  listSpaces,
  runFederatedQuery,
  updateContract,
  type AuditBridgeSummary,
  type FederatedQueryResult,
  type NexusOverview,
  type NexusSpace,
  type PeerOrganization,
  type ReplicationPlan,
  type ShareDetail,
  type SharingContract,
} from '@/lib/api/nexus';
import { notifications } from '@stores/notifications';

function toLocalDateTime(date: Date) {
  const pad = (value: number) => String(value).padStart(2, '0');
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

function emptyPeerDraft(): PeerDraft {
  return {
    slug: 'partner-new',
    display_name: 'New Partner Org',
    organization_type: 'partner',
    region: 'eu-central-1',
    endpoint_url: 'https://partner.example.com/nexus',
    auth_mode: 'mtls+jwt',
    trust_level: 'partner',
    public_key_fingerprint: 'SHA256:NEW:PARTNER:FPR',
    shared_scopes_text: 'catalog, audit',
    admin_contacts_text: 'ops@example.com, security@example.com',
  };
}

function emptySpaceDraft(): SpaceDraft {
  return {
    slug: 'shared-space-new',
    display_name: 'Shared Partner Space',
    description: 'Shared data and operating context for cross-org delivery.',
    space_kind: 'shared',
    owner_peer_id: '',
    region: 'eu-west-1',
    member_peer_ids: [],
    governance_tags_text: 'partners, regulated',
    status: 'active',
  };
}

function emptyContractDraft(): ContractDraft {
  const expiresAt = new Date(Date.now() + 1000 * 60 * 60 * 24 * 180);
  return {
    peer_id: '',
    name: 'Cross-org Access Contract',
    description: 'Purpose-bound data sharing agreement with residency and encryption terms.',
    dataset_locator: 'partner://dataset/path',
    allowed_purposes_text: 'analytics, support',
    data_classes_text: 'confidential',
    residency_region: 'eu',
    query_template: 'SELECT * FROM shared_dataset LIMIT 100',
    max_rows_per_query: '1000',
    replication_mode: 'query_only',
    encryption_profile: 'mutual-tls+envelope',
    retention_days: '180',
    status: 'active',
    expires_at: toLocalDateTime(expiresAt),
  };
}

function emptyShareDraft(): ShareDraft {
  return {
    contract_id: '',
    provider_peer_id: '',
    consumer_peer_id: '',
    provider_space_id: '',
    consumer_space_id: '',
    dataset_name: 'shared_dataset_preview',
    selector_text: JSON.stringify({ partition: '2026-Q2' }),
    provider_schema_text: JSON.stringify({ id: 'string', metric: 'number', region: 'string' }, null, 2),
    consumer_schema_text: JSON.stringify({ id: 'string', metric: 'number', region: 'string' }, null, 2),
    sample_rows_text: JSON.stringify([{ id: 'row-1', metric: 42, region: 'eu' }], null, 2),
    replication_mode: 'incremental_replication',
  };
}

function emptyQueryDraft(): QueryDraft {
  return {
    share_id: '',
    sql: 'SELECT * FROM shared_dataset LIMIT 50',
    purpose: 'analytics',
    limit: '50',
  };
}

function parseCsv(value: string) {
  return value
    .split(',')
    .map((entry) => entry.trim())
    .filter(Boolean);
}

function parseJson<T>(value: string) {
  return JSON.parse(value) as T;
}

function contractToDraft(contract: SharingContract): ContractDraft {
  return {
    id: contract.id,
    peer_id: contract.peer_id,
    name: contract.name,
    description: contract.description,
    dataset_locator: contract.dataset_locator,
    allowed_purposes_text: contract.allowed_purposes.join(', '),
    data_classes_text: contract.data_classes.join(', '),
    residency_region: contract.residency_region,
    query_template: contract.query_template,
    max_rows_per_query: String(contract.max_rows_per_query),
    replication_mode: contract.replication_mode,
    encryption_profile: contract.encryption_profile,
    retention_days: String(contract.retention_days),
    status: contract.status,
    expires_at: toLocalDateTime(new Date(contract.expires_at)),
  };
}

function shareToQueryDraft(share: ShareDetail): QueryDraft {
  return {
    share_id: share.share.id,
    sql: share.access_grant?.query_template ?? `SELECT * FROM ${share.share.dataset_name} LIMIT 50`,
    purpose: share.access_grant?.allowed_purposes[0] ?? 'analytics',
    limit: String(share.access_grant?.max_rows_per_query ?? 50),
  };
}

export function NexusPage() {
  const [overview, setOverview] = useState<NexusOverview | null>(null);
  const [peers, setPeers] = useState<PeerOrganization[]>([]);
  const [spaces, setSpaces] = useState<NexusSpace[]>([]);
  const [contracts, setContracts] = useState<SharingContract[]>([]);
  const [shares, setShares] = useState<ShareDetail[]>([]);
  const [replicationPlans, setReplicationPlans] = useState<ReplicationPlan[]>([]);
  const [auditBridge, setAuditBridge] = useState<AuditBridgeSummary | null>(null);
  const [queryResult, setQueryResult] = useState<FederatedQueryResult | null>(null);

  const [selectedContractId, setSelectedContractId] = useState('');
  const [selectedShareId, setSelectedShareId] = useState('');

  const [loading, setLoading] = useState(true);
  const [busyAction, setBusyAction] = useState('');
  const [uiError, setUiError] = useState('');

  const [peerDraft, setPeerDraft] = useState<PeerDraft>(emptyPeerDraft);
  const [spaceDraft, setSpaceDraft] = useState<SpaceDraft>(emptySpaceDraft);
  const [contractDraft, setContractDraft] = useState<ContractDraft>(emptyContractDraft);
  const [shareDraft, setShareDraft] = useState<ShareDraft>(emptyShareDraft);
  const [queryDraft, setQueryDraft] = useState<QueryDraft>(emptyQueryDraft);

  const busy = loading || busyAction.length > 0;
  const selectedShare = shares.find((share) => share.share.id === selectedShareId) ?? null;

  async function refreshAll() {
    setLoading(true);
    setUiError('');
    try {
      const [overviewResponse, peersResponse, spacesResponse, contractsResponse, sharesResponse, replicationResponse, auditBridgeResponse] =
        await Promise.all([
          getOverview(),
          listPeers(),
          listSpaces(),
          listContracts(),
          listShares(),
          listReplicationPlans(),
          getAuditBridge(),
        ]);

      setOverview(overviewResponse);
      setPeers(peersResponse.items);
      setSpaces(spacesResponse.items);
      setContracts(contractsResponse.items);
      setShares(sharesResponse.items);
      setReplicationPlans(replicationResponse.items);
      setAuditBridge(auditBridgeResponse);

      setSelectedContractId((current) => {
        if (!current && contractsResponse.items[0]) {
          setContractDraft(contractToDraft(contractsResponse.items[0]));
          return contractsResponse.items[0].id;
        }
        if (current) {
          const found = contractsResponse.items.find((c) => c.id === current);
          if (found) setContractDraft(contractToDraft(found));
        }
        return current;
      });

      setSelectedShareId((current) => {
        if (!current && sharesResponse.items[0]) {
          setQueryDraft(shareToQueryDraft(sharesResponse.items[0]));
          return sharesResponse.items[0].share.id;
        }
        if (current) {
          const found = sharesResponse.items.find((s) => s.share.id === current);
          if (found) setQueryDraft(shareToQueryDraft(found));
        }
        return current;
      });

      setShareDraft((current) => ({
        ...current,
        contract_id: current.contract_id || contractsResponse.items[0]?.id || '',
        provider_peer_id: current.provider_peer_id || peersResponse.items[0]?.id || '',
        consumer_peer_id: current.consumer_peer_id || peersResponse.items[1]?.id || '',
        provider_space_id: current.provider_space_id || spacesResponse.items[0]?.id || '',
        consumer_space_id: current.consumer_space_id || spacesResponse.items[1]?.id || '',
      }));

      setContractDraft((current) => ({
        ...current,
        peer_id: current.peer_id || peersResponse.items[0]?.id || '',
      }));

      setSpaceDraft((current) => ({
        ...current,
        owner_peer_id: current.owner_peer_id || peersResponse.items[0]?.id || '',
      }));
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to load nexus surfaces';
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

  function selectContract(contractId: string) {
    setSelectedContractId(contractId);
    const contract = contracts.find((entry) => entry.id === contractId);
    setContractDraft(contract ? contractToDraft(contract) : emptyContractDraft());
  }

  function selectShare(shareId: string) {
    setSelectedShareId(shareId);
    const share = shares.find((entry) => entry.share.id === shareId);
    if (share) setQueryDraft(shareToQueryDraft(share));
  }

  async function createPeerAction() {
    setBusyAction('create-peer');
    try {
      const peer = await createPeer({
        slug: peerDraft.slug,
        display_name: peerDraft.display_name,
        organization_type: peerDraft.organization_type,
        region: peerDraft.region,
        endpoint_url: peerDraft.endpoint_url,
        auth_mode: peerDraft.auth_mode,
        trust_level: peerDraft.trust_level,
        public_key_fingerprint: peerDraft.public_key_fingerprint,
        shared_scopes: parseCsv(peerDraft.shared_scopes_text),
        admin_contacts: parseCsv(peerDraft.admin_contacts_text),
      });
      setPeerDraft(emptyPeerDraft());
      await refreshAll();
      notifications.success(`Registered ${peer.display_name}`);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to register peer';
      setUiError(message);
      notifications.error(message);
    } finally {
      setBusyAction('');
    }
  }

  async function createSpaceAction() {
    setBusyAction('create-space');
    try {
      const space = await createSpace({
        slug: spaceDraft.slug,
        display_name: spaceDraft.display_name,
        description: spaceDraft.description,
        space_kind: spaceDraft.space_kind,
        owner_peer_id: spaceDraft.owner_peer_id || null,
        region: spaceDraft.region,
        member_peer_ids: spaceDraft.member_peer_ids,
        governance_tags: parseCsv(spaceDraft.governance_tags_text),
        status: spaceDraft.status,
      });
      setSpaceDraft(emptySpaceDraft());
      await refreshAll();
      notifications.success(`Created space ${space.display_name}`);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to create space';
      setUiError(message);
      notifications.error(message);
    } finally {
      setBusyAction('');
    }
  }

  async function authenticatePeerAction(peerId: string) {
    setBusyAction('authenticate-peer');
    try {
      const peer = await authenticatePeer(peerId);
      await refreshAll();
      notifications.success(`Authenticated ${peer.display_name}`);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to authenticate peer';
      setUiError(message);
      notifications.error(message);
    } finally {
      setBusyAction('');
    }
  }

  async function saveContractAction() {
    setBusyAction('save-contract');
    try {
      const payload = {
        peer_id: contractDraft.peer_id,
        name: contractDraft.name,
        description: contractDraft.description,
        dataset_locator: contractDraft.dataset_locator,
        allowed_purposes: parseCsv(contractDraft.allowed_purposes_text),
        data_classes: parseCsv(contractDraft.data_classes_text),
        residency_region: contractDraft.residency_region,
        query_template: contractDraft.query_template,
        max_rows_per_query: Number(contractDraft.max_rows_per_query),
        replication_mode: contractDraft.replication_mode,
        encryption_profile: contractDraft.encryption_profile,
        retention_days: Number(contractDraft.retention_days),
        status: contractDraft.status,
        expires_at: new Date(contractDraft.expires_at).toISOString(),
      };
      const contract = contractDraft.id
        ? await updateContract(contractDraft.id, payload)
        : await createContract(payload);
      setSelectedContractId(contract.id);
      await refreshAll();
      notifications.success(`${contractDraft.id ? 'Updated' : 'Created'} ${contract.name}`);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to save contract';
      setUiError(message);
      notifications.error(message);
    } finally {
      setBusyAction('');
    }
  }

  async function createShareAction() {
    setBusyAction('create-share');
    try {
      const detail = await createShare({
        contract_id: shareDraft.contract_id,
        provider_peer_id: shareDraft.provider_peer_id,
        consumer_peer_id: shareDraft.consumer_peer_id,
        provider_space_id: shareDraft.provider_space_id || null,
        consumer_space_id: shareDraft.consumer_space_id || null,
        dataset_name: shareDraft.dataset_name,
        selector: parseJson<Record<string, unknown>>(shareDraft.selector_text),
        provider_schema: parseJson<Record<string, unknown>>(shareDraft.provider_schema_text),
        consumer_schema: parseJson<Record<string, unknown>>(shareDraft.consumer_schema_text),
        sample_rows: parseJson<Record<string, unknown>[]>(shareDraft.sample_rows_text),
        replication_mode: shareDraft.replication_mode,
      });
      setSelectedShareId(detail.share.id);
      setQueryDraft(shareToQueryDraft(detail));
      await refreshAll();
      notifications.success(`Created share ${detail.share.dataset_name}`);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to create share';
      setUiError(message);
      notifications.error(message);
    } finally {
      setBusyAction('');
    }
  }

  async function runQueryAction() {
    setBusyAction('run-query');
    try {
      const result = await runFederatedQuery({
        share_id: queryDraft.share_id,
        sql: queryDraft.sql,
        purpose: queryDraft.purpose,
        limit: Number(queryDraft.limit),
      });
      setQueryResult(result);
      notifications.success(`Loaded ${result.rows.length} federated rows`);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to run federated query';
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
          background: 'linear-gradient(135deg, #082f49 0%, #0c0a09 50%, #4a044e 100%)',
        }}
      >
        <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-end', justifyContent: 'space-between', gap: 24 }}>
          <div style={{ maxWidth: 720 }}>
            <p style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.28em', color: '#67e8f9' }}>
              Milestone 5.1
            </p>
            <h1 className="of-heading-xl" style={{ marginTop: 12, color: '#f8fafc' }}>
              Nexus for cross-org sharing, federated access, and trust-bound replication
            </h1>
            <p style={{ marginTop: 12, fontSize: 13, lineHeight: 1.6, color: 'rgba(248, 250, 252, 0.85)' }}>
              Operate partner onboarding, sharing contracts, schema checks, encrypted replication posture, and federated previews from one surface.
            </p>
          </div>
          <div style={{ borderRadius: 16, background: 'rgba(255,255,255,0.1)', padding: 16, minWidth: 260 }}>
            <p style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.18em', color: '#67e8f9' }}>
              Audit bridge
            </p>
            <p style={{ marginTop: 8, fontSize: 14, fontWeight: 600 }}>{overview?.audit_bridge_status ?? 'pending'}</p>
            <p style={{ marginTop: 4, fontSize: 12, color: '#a8a29e' }}>
              Latest sync {overview?.latest_sync_at ? new Date(overview.latest_sync_at).toLocaleString() : 'n/a'}
            </p>
          </div>
        </div>
      </section>

      {uiError && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {uiError}
        </div>
      )}

      <SharingDashboard overview={overview} auditBridge={auditBridge} replicationPlans={replicationPlans} />

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 0.95fr) minmax(0, 1.05fr)' }}>
        <PeerList
          peers={peers}
          draft={peerDraft}
          busy={busy}
          onDraftChange={(patch) => setPeerDraft((current) => ({ ...current, ...patch }))}
          onCreate={() => void createPeerAction()}
          onAuthenticate={(peerId) => void authenticatePeerAction(peerId)}
        />
        <ContractManager
          contracts={contracts}
          peers={peers}
          selectedContractId={selectedContractId}
          draft={contractDraft}
          busy={busy}
          onSelect={selectContract}
          onDraftChange={(patch) => setContractDraft((current) => ({ ...current, ...patch }))}
          onSave={() => void saveContractAction()}
          onReset={() => {
            setSelectedContractId('');
            setContractDraft(emptyContractDraft());
          }}
        />
      </div>

      <SpaceManager
        spaces={spaces}
        peers={peers}
        draft={spaceDraft}
        busy={busy}
        onDraftChange={(patch) => setSpaceDraft((current) => ({ ...current, ...patch }))}
        onCreate={() => void createSpaceAction()}
      />

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 0.98fr) minmax(0, 1.02fr)' }}>
        <ShareWizard
          shares={shares}
          peers={peers}
          spaces={spaces}
          contracts={contracts}
          draft={shareDraft}
          busy={busy}
          onDraftChange={(patch) => setShareDraft((current) => ({ ...current, ...patch }))}
          onCreate={() => void createShareAction()}
        />
        <SharedDataBrowser
          shares={shares}
          selectedShareId={selectedShareId}
          selectedShare={selectedShare}
          replicationPlans={replicationPlans}
          auditBridge={auditBridge}
          queryDraft={queryDraft}
          queryResult={queryResult}
          busy={busy}
          onSelectShare={selectShare}
          onQueryDraftChange={(patch) => setQueryDraft((current) => ({ ...current, ...patch }))}
          onRunQuery={() => void runQueryAction()}
        />
      </div>
    </section>
  );
}
