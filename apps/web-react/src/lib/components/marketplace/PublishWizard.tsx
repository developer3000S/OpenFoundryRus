import type { PackageType } from '@/lib/api/marketplace';

export interface ListingDraft {
  id?: string;
  name: string;
  slug: string;
  summary: string;
  description: string;
  publisher: string;
  category_slug: string;
  package_kind: PackageType;
  repository_slug: string;
  visibility: string;
  tags_text: string;
  capabilities_text: string;
}

export interface VersionDraft {
  version: string;
  release_channel: string;
  changelog: string;
  dependency_mode: string;
  dependencies_text: string;
  packaged_resources_text: string;
  manifest_text: string;
}

interface Props {
  listingDraft: ListingDraft;
  versionDraft: VersionDraft;
  hasSelectedListing: boolean;
  busy?: boolean;
  onListingDraftChange: (patch: Partial<ListingDraft>) => void;
  onVersionDraftChange: (patch: Partial<VersionDraft>) => void;
  onPublishListing: () => void;
  onPublishVersion: () => void;
}

const PACKAGE_TYPES: PackageType[] = ['connector', 'transform', 'widget', 'app_template', 'ml_model', 'ai_agent'];

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

export function PublishWizard({
  listingDraft,
  versionDraft,
  hasSelectedListing,
  busy = false,
  onListingDraftChange,
  onVersionDraftChange,
  onPublishListing,
  onPublishVersion,
}: Props) {
  return (
    <section className="of-panel" style={{ padding: 20 }}>
      <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 12 }}>
        <div>
          <p className="of-eyebrow" style={{ color: '#7c3aed' }}>
            Publish Wizard
          </p>
          <h3 className="of-heading-md" style={{ marginTop: 6 }}>
            Listing metadata and version publication
          </h3>
          <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
            Create or update a registry listing, then publish a new version with dependency resolution metadata.
          </p>
        </div>
        <div style={{ display: 'flex', gap: 6 }}>
          <button type="button" onClick={onPublishListing} disabled={busy} className="of-button of-button--primary" style={{ background: '#9333ea' }}>
            {listingDraft.id ? 'Update listing' : 'Create listing'}
          </button>
          <button type="button" onClick={onPublishVersion} disabled={busy || !hasSelectedListing} className="of-button" style={{ borderColor: '#c4b5fd', color: '#7c3aed' }}>
            Publish version
          </button>
        </div>
      </div>

      <div style={{ display: 'grid', gap: 16, gridTemplateColumns: '1fr 1fr', marginTop: 18 }}>
        <div className="of-panel-muted" style={{ padding: 14, display: 'grid', gap: 12 }}>
          <p className="of-eyebrow">Listing draft</p>
          <div style={{ display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr' }}>
            <label style={{ fontSize: 13, gridColumn: 'span 2' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Name</span>
              <input value={listingDraft.name} onChange={(e) => onListingDraftChange({ name: e.target.value })} className="of-input" />
            </label>
            <label style={{ fontSize: 13 }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Slug</span>
              <input value={listingDraft.slug} onChange={(e) => onListingDraftChange({ slug: e.target.value })} className="of-input" />
            </label>
            <label style={{ fontSize: 13 }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Publisher</span>
              <input value={listingDraft.publisher} onChange={(e) => onListingDraftChange({ publisher: e.target.value })} className="of-input" />
            </label>
            <label style={{ fontSize: 13, gridColumn: 'span 2' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Summary</span>
              <input value={listingDraft.summary} onChange={(e) => onListingDraftChange({ summary: e.target.value })} className="of-input" />
            </label>
            <label style={{ fontSize: 13, gridColumn: 'span 2' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Description</span>
              <textarea
                value={listingDraft.description}
                onChange={(e) => onListingDraftChange({ description: e.target.value })}
                className="of-input"
                style={{ minHeight: 100, resize: 'vertical' }}
              />
            </label>
            <label style={{ fontSize: 13 }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Category</span>
              <input value={listingDraft.category_slug} onChange={(e) => onListingDraftChange({ category_slug: e.target.value })} className="of-input" />
            </label>
            <label style={{ fontSize: 13 }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Package type</span>
              <select
                value={listingDraft.package_kind}
                onChange={(e) => onListingDraftChange({ package_kind: e.target.value as PackageType })}
                className="of-input"
              >
                {PACKAGE_TYPES.map((type) => (
                  <option key={type} value={type}>
                    {type}
                  </option>
                ))}
              </select>
            </label>
            <label style={{ fontSize: 13 }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Repository slug</span>
              <input value={listingDraft.repository_slug} onChange={(e) => onListingDraftChange({ repository_slug: e.target.value })} className="of-input" />
            </label>
            <label style={{ fontSize: 13 }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Visibility</span>
              <input value={listingDraft.visibility} onChange={(e) => onListingDraftChange({ visibility: e.target.value })} className="of-input" />
            </label>
            <label style={{ fontSize: 13, gridColumn: 'span 2' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Tags</span>
              <input value={listingDraft.tags_text} onChange={(e) => onListingDraftChange({ tags_text: e.target.value })} className="of-input" />
            </label>
            <label style={{ fontSize: 13, gridColumn: 'span 2' }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Capabilities</span>
              <input value={listingDraft.capabilities_text} onChange={(e) => onListingDraftChange({ capabilities_text: e.target.value })} className="of-input" />
            </label>
          </div>
        </div>

        <div style={{ borderRadius: 16, padding: 14, background: '#0c0a09', color: '#f5f5f4', display: 'grid', gap: 12 }}>
          <p style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.18em', color: '#c4b5fd' }}>Version draft</p>
          <label style={{ fontSize: 13 }}>
            <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Version</span>
            <input value={versionDraft.version} onChange={(e) => onVersionDraftChange({ version: e.target.value })} style={darkInput} />
          </label>
          <label style={{ fontSize: 13 }}>
            <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Release channel</span>
            <input value={versionDraft.release_channel} onChange={(e) => onVersionDraftChange({ release_channel: e.target.value })} style={darkInput} />
          </label>
          <label style={{ fontSize: 13 }}>
            <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Dependency mode</span>
            <input value={versionDraft.dependency_mode} onChange={(e) => onVersionDraftChange({ dependency_mode: e.target.value })} style={darkInput} />
          </label>
          <label style={{ fontSize: 13 }}>
            <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>Changelog</span>
            <textarea
              value={versionDraft.changelog}
              onChange={(e) => onVersionDraftChange({ changelog: e.target.value })}
              style={{ ...darkInput, minHeight: 90, resize: 'vertical' }}
            />
          </label>
          {(['dependencies_text', 'packaged_resources_text', 'manifest_text'] as const).map((field) => (
            <label key={field} style={{ fontSize: 13 }}>
              <span style={{ display: 'block', marginBottom: 6, fontWeight: 500 }}>
                {field === 'dependencies_text' ? 'Dependencies JSON' : field === 'packaged_resources_text' ? 'Packaged resources JSON' : 'Manifest JSON'}
              </span>
              <textarea
                value={versionDraft[field]}
                onChange={(e) => onVersionDraftChange({ [field]: e.target.value } as Partial<VersionDraft>)}
                style={{ ...darkInput, minHeight: 130, fontFamily: 'var(--font-mono)', fontSize: 11, color: '#c4b5fd', resize: 'vertical' }}
              />
            </label>
          ))}
        </div>
      </div>
    </section>
  );
}
