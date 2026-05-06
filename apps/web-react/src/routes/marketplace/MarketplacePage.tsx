import { useEffect, useState } from 'react';

import { DeliveryStudio, type BranchDraft, type FleetDraft } from '@/lib/components/marketplace/DeliveryStudio';
import { ListingDetail, type InstallDraft, type ReviewDraft } from '@/lib/components/marketplace/ListingDetail';
import { MarketplaceBrowser } from '@/lib/components/marketplace/MarketplaceBrowser';
import { MyPackages } from '@/lib/components/marketplace/MyPackages';
import { PublishWizard, type ListingDraft, type VersionDraft } from '@/lib/components/marketplace/PublishWizard';
import {
  createEnrollmentBranch,
  createFleet,
  createInstall,
  createListing,
  createReview,
  getListing,
  getOverview,
  listCategories,
  listEnrollmentBranches,
  listFleets,
  listInstalls,
  listListings,
  publishVersion,
  searchListings,
  syncFleet,
  updateListing,
  type CategoryDefinition,
  type DependencyRequirement,
  type EnrollmentBranchRecord,
  type InstallRecord,
  type ListingDefinition,
  type ListingDetail as ListingDetailModel,
  type MaintenanceWindow,
  type MarketplaceOverview,
  type PackagedResource,
  type ProductFleetRecord,
} from '@/lib/api/marketplace';
import { notifications } from '@stores/notifications';

function emptyListingDraft(): ListingDraft {
  return {
    name: 'Geo Insight Widget',
    slug: 'geo-insight-widget',
    summary: 'Map widget with clustering and route overlays for dashboards.',
    description: 'Provides a marketplace-ready geospatial widget powered by MapLibre previews.',
    publisher: 'Platform UI',
    category_slug: 'widgets',
    package_kind: 'widget',
    repository_slug: 'foundry-widget-kit',
    visibility: 'private',
    tags_text: 'maps, dashboard, geospatial',
    capabilities_text: 'maplibre, clusters, routes',
  };
}

function emptyVersionDraft(): VersionDraft {
  return {
    version: '1.0.0',
    release_channel: 'stable',
    changelog: 'Ships the initial marketplace package metadata and route presets.',
    dependency_mode: 'strict',
    dependencies_text: JSON.stringify([{ package_slug: 'map-style-base', version_req: '~1.1', required: true }], null, 2),
    packaged_resources_text: JSON.stringify(
      [
        { kind: 'widget', name: 'Geo Insight Widget', resource_ref: 'widgets/geo-insight', required: true },
        { kind: 'dashboard', name: 'Geo Ops Dashboard', resource_ref: 'dashboards/geo-ops', required: false },
      ],
      null,
      2,
    ),
    manifest_text: JSON.stringify({ entrypoint: 'widget.json', runtime: 'svelte', rollout_hint: 'rolling' }, null, 2),
  };
}

function emptyReviewDraft(): ReviewDraft {
  return {
    author: 'OpenFoundry User',
    rating: '5',
    headline: 'Great internal package',
    body: 'The install flow was fast and the dependency plan was easy to understand.',
    recommended: true,
  };
}

function emptyInstallDraft(): InstallDraft {
  return {
    version: '',
    workspace_name: 'OpenFoundry Workspace',
    release_channel: 'stable',
    fleet_id: '',
    enrollment_branch: '',
  };
}

function emptyFleetDraft(): FleetDraft {
  return {
    name: 'Operations rollout fleet',
    environment: 'production',
    workspace_targets_text: 'OpenFoundry Workspace',
    release_channel: 'stable',
    auto_upgrade_enabled: true,
    maintenance_days_text: 'sun',
    start_hour_utc: '2',
    duration_minutes: '180',
    branch_strategy: 'isolated_branch_per_feature',
    rollout_strategy: 'rolling',
  };
}

function emptyBranchDraft(): BranchDraft {
  return {
    fleet_id: '',
    name: 'feature/ops-drilldown',
    repository_branch: '',
    notes: 'Sandbox branch for enrollment-level changes before promotion.',
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

function listingToDraft(listing: ListingDefinition): ListingDraft {
  return {
    id: listing.id,
    name: listing.name,
    slug: listing.slug,
    summary: listing.summary,
    description: listing.description,
    publisher: listing.publisher,
    category_slug: listing.category_slug,
    package_kind: listing.package_kind,
    repository_slug: listing.repository_slug,
    visibility: listing.visibility,
    tags_text: listing.tags.join(', '),
    capabilities_text: listing.capabilities.join(', '),
  };
}

export function MarketplacePage() {
  const [overview, setOverview] = useState<MarketplaceOverview | null>(null);
  const [categories, setCategories] = useState<CategoryDefinition[]>([]);
  const [listings, setListings] = useState<ListingDefinition[]>([]);
  const [installs, setInstalls] = useState<InstallRecord[]>([]);
  const [fleets, setFleets] = useState<ProductFleetRecord[]>([]);
  const [enrollmentBranches, setEnrollmentBranches] = useState<EnrollmentBranchRecord[]>([]);
  const [listingDetail, setListingDetail] = useState<ListingDetailModel | null>(null);
  const [scoreById, setScoreById] = useState<Record<string, number>>({});
  const [selectedListingId, setSelectedListingId] = useState('');
  const [selectedCategory, setSelectedCategory] = useState('all');
  const [searchQuery, setSearchQuery] = useState('widget');
  const [loading, setLoading] = useState(true);
  const [busyAction, setBusyAction] = useState('');
  const [uiError, setUiError] = useState('');

  const [listingDraft, setListingDraft] = useState<ListingDraft>(emptyListingDraft);
  const [versionDraft, setVersionDraft] = useState<VersionDraft>(emptyVersionDraft);
  const [reviewDraft, setReviewDraft] = useState<ReviewDraft>(emptyReviewDraft);
  const [installDraft, setInstallDraft] = useState<InstallDraft>(emptyInstallDraft);
  const [fleetDraft, setFleetDraft] = useState<FleetDraft>(emptyFleetDraft);
  const [branchDraft, setBranchDraft] = useState<BranchDraft>(emptyBranchDraft);

  const busy = loading || busyAction.length > 0;

  function fleetMaintenanceWindowFromDraft(): MaintenanceWindow {
    return {
      timezone: 'UTC',
      days: parseCsv(fleetDraft.maintenance_days_text),
      start_hour_utc: Number(fleetDraft.start_hour_utc || '2'),
      duration_minutes: Number(fleetDraft.duration_minutes || '120'),
    };
  }

  async function selectListing(listingId: string, notify = true) {
    setBusyAction('listing');
    setUiError('');
    try {
      setSelectedListingId(listingId);
      const detail = await getListing(listingId);
      setListingDetail(detail);
      setListingDraft(listingToDraft(detail.listing));
      setInstallDraft((current) => ({
        ...current,
        version: detail.latest_version?.version ?? detail.versions[0]?.version ?? '',
        release_channel: detail.latest_version?.release_channel ?? detail.versions[0]?.release_channel ?? 'stable',
        fleet_id: fleets.find((fleet) => fleet.listing_id === listingId)?.id ?? current.fleet_id,
      }));
      if (notify) {
        notifications.info(`Loaded ${detail.listing.name}`);
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to load listing';
      setUiError(message);
      notifications.error(message);
    } finally {
      setBusyAction('');
    }
  }

  async function refreshAll(preferredListingId?: string) {
    setLoading(true);
    setUiError('');
    try {
      const [overviewResponse, categoriesResponse, listingsResponse, installsResponse, fleetsResponse, branchesResponse] = await Promise.all([
        getOverview(),
        listCategories(),
        listListings(),
        listInstalls(),
        listFleets(),
        listEnrollmentBranches(),
      ]);
      setOverview(overviewResponse);
      setCategories(categoriesResponse.items);
      setListings(listingsResponse.items);
      setInstalls(installsResponse.items);
      setFleets(fleetsResponse.items);
      setEnrollmentBranches(branchesResponse.items);
      setScoreById({});
      const nextListingId = preferredListingId ?? selectedListingId ?? listingsResponse.items[0]?.id ?? '';
      if (nextListingId) {
        await selectListing(nextListingId, false);
      } else {
        setListingDetail(null);
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to load marketplace surfaces';
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

  async function runSearch() {
    setBusyAction('search');
    setUiError('');
    try {
      let nextListings: ListingDefinition[];
      if (searchQuery.trim() || selectedCategory !== 'all') {
        const response = await searchListings(searchQuery, selectedCategory === 'all' ? undefined : selectedCategory);
        nextListings = response.results.map(([listing]) => listing);
        setScoreById(Object.fromEntries(response.results.map(([listing, score]) => [listing.id, score])));
      } else {
        const response = await listListings();
        nextListings = response.items;
        setScoreById({});
      }
      setListings(nextListings);
      if (nextListings[0]) {
        await selectListing(nextListings[0].id, false);
      }
      notifications.success(`Loaded ${nextListings.length} listings`);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to search listings';
      setUiError(message);
      notifications.error(message);
    } finally {
      setBusyAction('');
    }
  }

  async function publishListingAction() {
    setBusyAction('publish-listing');
    setUiError('');
    try {
      const payload = {
        name: listingDraft.name,
        slug: listingDraft.slug,
        summary: listingDraft.summary,
        description: listingDraft.description,
        publisher: listingDraft.publisher,
        category_slug: listingDraft.category_slug,
        package_kind: listingDraft.package_kind,
        repository_slug: listingDraft.repository_slug,
        visibility: listingDraft.visibility,
        tags: parseCsv(listingDraft.tags_text),
        capabilities: parseCsv(listingDraft.capabilities_text),
      };
      const listing = listingDraft.id ? await updateListing(listingDraft.id, payload) : await createListing(payload);
      await refreshAll(listing.id);
      notifications.success(`${listingDraft.id ? 'Updated' : 'Created'} ${listing.name}`);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to publish listing';
      setUiError(message);
      notifications.error(message);
    } finally {
      setBusyAction('');
    }
  }

  async function publishVersionAction() {
    if (!selectedListingId) {
      notifications.warning('Select a listing before publishing a version');
      return;
    }
    setBusyAction('publish-version');
    setUiError('');
    try {
      await publishVersion(selectedListingId, {
        version: versionDraft.version,
        release_channel: versionDraft.release_channel,
        changelog: versionDraft.changelog,
        dependency_mode: versionDraft.dependency_mode,
        dependencies: parseJson<DependencyRequirement[]>(versionDraft.dependencies_text),
        packaged_resources: parseJson<PackagedResource[]>(versionDraft.packaged_resources_text),
        manifest: parseJson<Record<string, unknown>>(versionDraft.manifest_text),
      });
      await selectListing(selectedListingId, false);
      notifications.success(`Published ${versionDraft.version}`);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to publish version';
      setUiError(message);
      notifications.error(message);
    } finally {
      setBusyAction('');
    }
  }

  async function createReviewAction() {
    if (!selectedListingId) {
      notifications.warning('Select a listing before publishing a review');
      return;
    }
    setBusyAction('review');
    setUiError('');
    try {
      await createReview(selectedListingId, {
        author: reviewDraft.author,
        rating: Number(reviewDraft.rating),
        headline: reviewDraft.headline,
        body: reviewDraft.body,
        recommended: reviewDraft.recommended,
      });
      await refreshAll(selectedListingId);
      notifications.success('Published review');
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to publish review';
      setUiError(message);
      notifications.error(message);
    } finally {
      setBusyAction('');
    }
  }

  async function installAction() {
    if (!selectedListingId) {
      notifications.warning('Select a listing before installing');
      return;
    }
    setBusyAction('install');
    setUiError('');
    try {
      await createInstall({
        listing_id: selectedListingId,
        version: installDraft.version,
        workspace_name: installDraft.workspace_name,
        release_channel: installDraft.release_channel,
        fleet_id: installDraft.fleet_id || null,
        enrollment_branch: installDraft.enrollment_branch || null,
      });
      await refreshAll(selectedListingId);
      notifications.success(`Installed ${listingDetail?.listing.name ?? 'package'}`);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to install package';
      setUiError(message);
      notifications.error(message);
    } finally {
      setBusyAction('');
    }
  }

  async function createFleetAction() {
    if (!selectedListingId) {
      notifications.warning('Select a listing before creating a fleet');
      return;
    }
    setBusyAction('create-fleet');
    setUiError('');
    try {
      const fleet = await createFleet({
        listing_id: selectedListingId,
        name: fleetDraft.name,
        environment: fleetDraft.environment,
        workspace_targets: parseCsv(fleetDraft.workspace_targets_text),
        release_channel: fleetDraft.release_channel,
        auto_upgrade_enabled: fleetDraft.auto_upgrade_enabled,
        maintenance_window: fleetMaintenanceWindowFromDraft(),
        branch_strategy: fleetDraft.branch_strategy,
        rollout_strategy: fleetDraft.rollout_strategy,
      });
      setBranchDraft((current) => ({ ...current, fleet_id: fleet.id }));
      await refreshAll(selectedListingId);
      notifications.success(`Created fleet ${fleet.name}`);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to create fleet';
      setUiError(message);
      notifications.error(message);
    } finally {
      setBusyAction('');
    }
  }

  async function createBranchAction() {
    if (!branchDraft.fleet_id) {
      notifications.warning('Select a fleet before creating a branch');
      return;
    }
    setBusyAction('create-branch');
    setUiError('');
    try {
      const branch = await createEnrollmentBranch({
        fleet_id: branchDraft.fleet_id,
        name: branchDraft.name,
        repository_branch: branchDraft.repository_branch || null,
        notes: branchDraft.notes,
      });
      setInstallDraft((current) => ({
        ...current,
        fleet_id: branch.fleet_id,
        enrollment_branch: branch.name,
      }));
      await refreshAll(selectedListingId || undefined);
      notifications.success(`Created enrollment branch ${branch.name}`);
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to create enrollment branch';
      setUiError(message);
      notifications.error(message);
    } finally {
      setBusyAction('');
    }
  }

  async function syncFleetAction(fleetId: string) {
    setBusyAction('sync-fleet');
    setUiError('');
    try {
      const result = await syncFleet(fleetId);
      await refreshAll(selectedListingId || undefined);
      if (result.blocked_reason) {
        notifications.warning(result.blocked_reason);
      } else {
        notifications.success(`Synced ${result.upgraded_workspaces.length} workspace(s)`);
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : 'Unable to sync fleet';
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
          background: 'linear-gradient(135deg, #fff7ed 0%, #ffffff 50%, #ecfdf5 100%)',
        }}
      >
        <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-end', justifyContent: 'space-between', gap: 24 }}>
          <div style={{ maxWidth: 720 }}>
            <p style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.28em', color: '#c2410c' }}>
              Marketplace
            </p>
            <h1 className="of-heading-xl" style={{ marginTop: 12 }}>
              Listings, versions, fleets, and one-click installs
            </h1>
            <p className="of-text-muted" style={{ marginTop: 12, fontSize: 13, lineHeight: 1.6 }}>
              Discover internal packages, publish releases, manage rollout fleets and enrollment branches, and operate
              installs across workspaces from one workspace.
            </p>
          </div>
          <div style={{ display: 'grid', gap: 8, gridTemplateColumns: 'repeat(2, 1fr)' }}>
            {[
              { label: 'Listings', value: overview?.listing_count ?? 0 },
              { label: 'Categories', value: overview?.category_count ?? 0 },
              { label: 'Installs', value: installs.length },
              { label: 'Fleets', value: fleets.length },
            ].map((stat) => (
              <div key={stat.label} style={{ borderRadius: 16, background: 'rgba(255,255,255,0.85)', padding: 12 }}>
                <p style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.16em', color: '#c2410c' }}>
                  {stat.label}
                </p>
                <p style={{ marginTop: 6, fontSize: 22, fontWeight: 600, color: 'var(--text-strong)' }}>{stat.value}</p>
              </div>
            ))}
          </div>
        </div>
      </section>

      {uiError && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {uiError}
        </div>
      )}

      <MarketplaceBrowser
        overview={overview}
        categories={categories}
        listings={listings}
        selectedListingId={selectedListingId}
        searchQuery={searchQuery}
        selectedCategory={selectedCategory}
        scoreById={scoreById}
        busy={busy}
        onSearchQueryChange={(query) => setSearchQuery(query)}
        onCategoryChange={(category) => setSelectedCategory(category)}
        onSearch={() => void runSearch()}
        onSelectListing={(id) => void selectListing(id)}
      />

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 1.05fr) minmax(0, 0.95fr)' }}>
        <ListingDetail
          detail={listingDetail}
          fleets={fleets}
          busy={busy}
          reviewDraft={reviewDraft}
          installDraft={installDraft}
          onReviewDraftChange={(patch) => setReviewDraft((current) => ({ ...current, ...patch }))}
          onInstallDraftChange={(patch) => setInstallDraft((current) => ({ ...current, ...patch }))}
          onCreateReview={() => void createReviewAction()}
          onInstall={() => void installAction()}
        />
        <PublishWizard
          listingDraft={listingDraft}
          versionDraft={versionDraft}
          hasSelectedListing={Boolean(selectedListingId)}
          busy={busy}
          onListingDraftChange={(patch) => setListingDraft((current) => ({ ...current, ...patch }))}
          onVersionDraftChange={(patch) => setVersionDraft((current) => ({ ...current, ...patch }))}
          onPublishListing={() => void publishListingAction()}
          onPublishVersion={() => void publishVersionAction()}
        />
      </div>

      <DeliveryStudio
        fleets={fleets}
        branches={enrollmentBranches}
        selectedListingId={selectedListingId}
        busy={busy}
        fleetDraft={fleetDraft}
        branchDraft={branchDraft}
        onFleetDraftChange={(patch) => setFleetDraft((current) => ({ ...current, ...patch }))}
        onBranchDraftChange={(patch) => setBranchDraft((current) => ({ ...current, ...patch }))}
        onCreateFleet={() => void createFleetAction()}
        onCreateBranch={() => void createBranchAction()}
        onSyncFleet={(fleetId) => void syncFleetAction(fleetId)}
      />

      <MyPackages installs={installs} />
    </section>
  );
}
