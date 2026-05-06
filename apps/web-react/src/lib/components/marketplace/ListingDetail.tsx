import type { ListingDetail as ListingDetailModel, ProductFleetRecord } from '@/lib/api/marketplace';
import { InstallDialog } from './InstallDialog';

export interface ReviewDraft {
  author: string;
  rating: string;
  headline: string;
  body: string;
  recommended: boolean;
}

export interface InstallDraft {
  version: string;
  workspace_name: string;
  release_channel: string;
  fleet_id: string;
  enrollment_branch: string;
}

interface Props {
  detail: ListingDetailModel | null;
  reviewDraft: ReviewDraft;
  installDraft: InstallDraft;
  fleets: ProductFleetRecord[];
  busy?: boolean;
  onReviewDraftChange: (patch: Partial<ReviewDraft>) => void;
  onInstallDraftChange: (patch: Partial<InstallDraft>) => void;
  onCreateReview: () => void;
  onInstall: () => void;
}

const darkInput: React.CSSProperties = {
  width: '100%',
  borderRadius: 16,
  border: '1px solid #44403c',
  background: '#1c1917',
  padding: '10px 14px',
  color: '#f5f5f4',
  fontSize: 13,
  outline: 'none',
};

export function ListingDetail({
  detail,
  reviewDraft,
  installDraft,
  fleets,
  busy = false,
  onReviewDraftChange,
  onInstallDraftChange,
  onCreateReview,
  onInstall,
}: Props) {
  return (
    <section className="of-panel" style={{ padding: 20 }}>
      <div>
        <p className="of-eyebrow" style={{ color: '#0e7490' }}>
          Listing Detail
        </p>
        <h3 className="of-heading-md" style={{ marginTop: 6 }}>
          Package metadata, versions, dependency plans, and reviews
        </h3>
        <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
          Inspect the currently selected listing, publish-ready metadata, and installation surface.
        </p>
      </div>

      {detail ? (
        <div style={{ display: 'grid', gap: 16, gridTemplateColumns: 'minmax(0, 1.02fr) minmax(0, 0.98fr)', marginTop: 18 }}>
          <div className="of-panel-muted" style={{ padding: 14, display: 'grid', gap: 12 }}>
            <div className="of-panel" style={{ padding: 14 }}>
              <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 8 }}>
                <div>
                  <p style={{ fontSize: 18, fontWeight: 600, color: 'var(--text-strong)' }}>{detail.listing.name}</p>
                  <p className="of-text-muted" style={{ fontSize: 13 }}>
                    {detail.listing.publisher} · {detail.listing.package_kind} · {detail.listing.repository_slug}
                  </p>
                </div>
                <span className="of-chip" style={{ background: '#cffafe', color: '#0e7490', textTransform: 'uppercase', letterSpacing: '0.16em' }}>
                  {detail.listing.average_rating.toFixed(1)} rating
                </span>
              </div>
              <p style={{ marginTop: 10, fontSize: 13, color: 'var(--text-default)' }}>{detail.listing.description}</p>
              <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4, marginTop: 12 }}>
                {detail.listing.capabilities.map((capability) => (
                  <span key={capability} className="of-chip">
                    {capability}
                  </span>
                ))}
              </div>
            </div>

            <div className="of-panel" style={{ padding: 14 }}>
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
                <p style={{ fontWeight: 600, color: 'var(--text-strong)' }}>Published versions</p>
                <p className="of-eyebrow">{detail.versions.length} versions</p>
              </div>
              <div style={{ display: 'grid', gap: 8, marginTop: 10 }}>
                {detail.versions.map((version) => (
                  <div key={version.version} className="of-panel-muted" style={{ padding: 12 }}>
                    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
                      <p style={{ fontWeight: 500, color: 'var(--text-strong)' }}>{version.version}</p>
                      <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
                        <span className="of-chip" style={{ background: '#cffafe', color: '#0e7490', textTransform: 'uppercase', letterSpacing: '0.16em' }}>
                          {version.release_channel}
                        </span>
                        <span className="of-chip" style={{ textTransform: 'uppercase', letterSpacing: '0.16em' }}>
                          {version.dependency_mode}
                        </span>
                      </div>
                    </div>
                    <p className="of-text-muted" style={{ marginTop: 6, fontSize: 13 }}>
                      {version.changelog}
                    </p>
                    <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4, marginTop: 8 }}>
                      {version.dependencies.map((dependency, index) => (
                        <span key={index} className="of-chip">
                          {dependency.package_slug} {dependency.version_req}
                        </span>
                      ))}
                    </div>
                    {version.packaged_resources.length > 0 && (
                      <div className="of-panel" style={{ padding: 10, marginTop: 8 }}>
                        <p className="of-eyebrow">Packaged resources</p>
                        <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4, marginTop: 6 }}>
                          {version.packaged_resources.map((resource, index) => (
                            <span key={index} className="of-chip">
                              {resource.kind} · {resource.name}
                            </span>
                          ))}
                        </div>
                      </div>
                    )}
                  </div>
                ))}
              </div>
            </div>

            <div className="of-panel" style={{ padding: 14 }}>
              <p style={{ fontWeight: 600, color: 'var(--text-strong)' }}>Reviews</p>
              <div style={{ display: 'grid', gap: 8, marginTop: 10 }}>
                {detail.reviews.map((review) => (
                  <div key={review.id} className="of-panel-muted" style={{ padding: 12 }}>
                    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
                      <div>
                        <p style={{ fontWeight: 500, color: 'var(--text-strong)' }}>{review.headline}</p>
                        <p className="of-text-muted" style={{ fontSize: 13 }}>
                          {review.author}
                        </p>
                      </div>
                      <span className="of-chip" style={{ background: '#fef3c7', color: '#b45309', textTransform: 'uppercase', letterSpacing: '0.16em' }}>
                        {review.rating}/5
                      </span>
                    </div>
                    <p className="of-text-muted" style={{ marginTop: 8, fontSize: 13 }}>
                      {review.body}
                    </p>
                  </div>
                ))}
              </div>
            </div>
          </div>

          <div style={{ borderRadius: 16, padding: 14, background: '#0c0a09', color: '#f5f5f4', display: 'grid', gap: 14 }}>
            <InstallDialog
              versions={detail.versions.map((version) => version.version)}
              version={installDraft.version}
              workspaceName={installDraft.workspace_name}
              releaseChannel={installDraft.release_channel}
              fleetId={installDraft.fleet_id}
              enrollmentBranch={installDraft.enrollment_branch}
              fleets={fleets.filter((fleet) => fleet.listing_id === detail.listing.id)}
              busy={busy}
              onVersionChange={(version) => onInstallDraftChange({ version })}
              onWorkspaceNameChange={(workspace_name) => onInstallDraftChange({ workspace_name })}
              onReleaseChannelChange={(release_channel) => onInstallDraftChange({ release_channel })}
              onFleetChange={(fleet_id) => onInstallDraftChange({ fleet_id })}
              onEnrollmentBranchChange={(enrollment_branch) => onInstallDraftChange({ enrollment_branch })}
              onInstall={onInstall}
            />

            <div>
              <p style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.18em', color: '#67e8f9' }}>
                Add review
              </p>
              <div style={{ display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr', marginTop: 14 }}>
                <label style={{ fontSize: 13 }}>
                  <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Author</span>
                  <input value={reviewDraft.author} onChange={(e) => onReviewDraftChange({ author: e.target.value })} style={darkInput} />
                </label>
                <label style={{ fontSize: 13 }}>
                  <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Rating</span>
                  <input value={reviewDraft.rating} onChange={(e) => onReviewDraftChange({ rating: e.target.value })} style={darkInput} />
                </label>
                <label style={{ fontSize: 13, gridColumn: 'span 2' }}>
                  <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Headline</span>
                  <input value={reviewDraft.headline} onChange={(e) => onReviewDraftChange({ headline: e.target.value })} style={darkInput} />
                </label>
                <label style={{ fontSize: 13, gridColumn: 'span 2' }}>
                  <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Body</span>
                  <textarea
                    value={reviewDraft.body}
                    onChange={(e) => onReviewDraftChange({ body: e.target.value })}
                    style={{ ...darkInput, minHeight: 110, resize: 'vertical' }}
                  />
                </label>
                <label
                  style={{
                    display: 'inline-flex',
                    alignItems: 'center',
                    gap: 8,
                    padding: '10px 14px',
                    borderRadius: 16,
                    border: '1px solid #44403c',
                    background: '#1c1917',
                    fontSize: 13,
                    gridColumn: 'span 2',
                  }}
                >
                  <input type="checkbox" checked={reviewDraft.recommended} onChange={(e) => onReviewDraftChange({ recommended: e.target.checked })} />
                  Recommend this package
                </label>
              </div>
              <button type="button" onClick={onCreateReview} disabled={busy} className="of-button of-button--primary" style={{ marginTop: 14, background: '#06b6d4', color: '#0c0a09' }}>
                Publish review
              </button>
            </div>
          </div>
        </div>
      ) : (
        <div
          style={{
            marginTop: 18,
            border: '1px dashed var(--border-default)',
            borderRadius: 16,
            padding: 32,
            textAlign: 'center',
            fontSize: 13,
            color: 'var(--text-muted)',
            background: 'var(--bg-subtle)',
          }}
        >
          Select a listing to inspect versions, dependencies, and install options.
        </div>
      )}
    </section>
  );
}
