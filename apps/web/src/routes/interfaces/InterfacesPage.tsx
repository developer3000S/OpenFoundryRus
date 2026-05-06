import { useEffect, useMemo, useState } from 'react';

import {
  attachInterfaceToType,
  createInterface,
  createInterfaceProperty,
  deleteInterface,
  deleteInterfaceProperty,
  detachInterfaceFromType,
  listInterfaceProperties,
  listInterfaces,
  listObjectTypes,
  listTypeInterfaces,
  updateInterface,
  updateInterfaceProperty,
  type InterfaceProperty,
  type ObjectType,
  type OntologyInterface,
} from '@/lib/api/ontology';

type WorkbenchTab = 'library' | 'definition' | 'implementation' | 'reference';

interface TypeInterfaceBindingRow {
  objectType: ObjectType;
  interfaces: OntologyInterface[];
}

interface InterfaceStats {
  propertyCount: number;
  implementationCount: number;
  requiredPropertyCount: number;
  timeDependentCount: number;
}

const WORKBENCH_TABS: Array<{ id: WorkbenchTab; label: string }> = [
  { id: 'library', label: 'Library' },
  { id: 'definition', label: 'Definition' },
  { id: 'implementation', label: 'Implementation' },
  { id: 'reference', label: 'Reference' },
];

const PROPERTY_TYPE_OPTIONS = [
  'string',
  'integer',
  'float',
  'boolean',
  'date',
  'json',
  'array',
  'reference',
  'geo_point',
  'media_reference',
  'vector',
];

function parseLooseJson(source: string, label: string): unknown {
  try {
    return JSON.parse(source);
  } catch (error) {
    throw new Error(`${label}: ${error instanceof Error ? error.message : 'Invalid JSON'}`);
  }
}

export function InterfacesPage() {
  const [interfaces, setInterfaces] = useState<OntologyInterface[]>([]);
  const [properties, setProperties] = useState<InterfaceProperty[]>([]);
  const [objectTypes, setObjectTypes] = useState<ObjectType[]>([]);
  const [typeBindings, setTypeBindings] = useState<TypeInterfaceBindingRow[]>([]);

  const [loading, setLoading] = useState(true);
  const [contextLoading, setContextLoading] = useState(false);
  const [bindingLoading, setBindingLoading] = useState(false);
  const [savingInterface, setSavingInterface] = useState(false);
  const [deletingInterface, setDeletingInterface] = useState(false);
  const [savingProperty, setSavingProperty] = useState(false);
  const [bindingTypeId, setBindingTypeId] = useState('');

  const [pageError, setPageError] = useState('');
  const [saveError, setSaveError] = useState('');
  const [saveSuccess, setSaveSuccess] = useState('');
  const [propertyError, setPropertyError] = useState('');
  const [propertySuccess, setPropertySuccess] = useState('');

  const [activeTab, setActiveTab] = useState<WorkbenchTab>('library');
  const [catalogQuery, setCatalogQuery] = useState('');
  const [selectedInterfaceId, setSelectedInterfaceId] = useState('');

  const [interfaceName, setInterfaceName] = useState('');
  const [interfaceDisplayName, setInterfaceDisplayName] = useState('');
  const [interfaceDescription, setInterfaceDescription] = useState('');

  const [propertyName, setPropertyName] = useState('');
  const [propertyDisplayName, setPropertyDisplayName] = useState('');
  const [propertyDescription, setPropertyDescription] = useState('');
  const [propertyType, setPropertyType] = useState('string');
  const [propertyRequired, setPropertyRequired] = useState(false);
  const [propertyUnique, setPropertyUnique] = useState(false);
  const [propertyTimeDependent, setPropertyTimeDependent] = useState(false);
  const [propertyDefaultValueText, setPropertyDefaultValueText] = useState('null');
  const [propertyValidationRulesText, setPropertyValidationRulesText] = useState('{}');

  const selectedInterface = useMemo(
    () => interfaces.find((item) => item.id === selectedInterfaceId) ?? null,
    [interfaces, selectedInterfaceId],
  );

  const filteredInterfaces = useMemo(() => {
    const query = catalogQuery.trim().toLowerCase();
    if (!query) return interfaces;
    return interfaces.filter((item) =>
      `${item.name} ${item.display_name} ${item.description}`.toLowerCase().includes(query),
    );
  }, [interfaces, catalogQuery]);

  const implementationRows = useMemo(() => {
    if (!selectedInterfaceId) return [];
    return typeBindings.filter((row) => row.interfaces.some((iface) => iface.id === selectedInterfaceId));
  }, [typeBindings, selectedInterfaceId]);

  const availableTypes = useMemo(
    () => typeBindings.filter((row) => !row.interfaces.some((iface) => iface.id === selectedInterfaceId)),
    [typeBindings, selectedInterfaceId],
  );

  const interfaceStats = useMemo<InterfaceStats>(
    () => ({
      propertyCount: properties.length,
      implementationCount: implementationRows.length,
      requiredPropertyCount: properties.filter((property) => property.required).length,
      timeDependentCount: properties.filter((property) => property.time_dependent).length,
    }),
    [properties, implementationRows],
  );

  function resetInterfaceDraft() {
    setInterfaceName('');
    setInterfaceDisplayName('');
    setInterfaceDescription('');
    setSelectedInterfaceId('');
    setProperties([]);
    setSaveError('');
    setSaveSuccess('');
    setPropertyError('');
    setPropertySuccess('');
  }

  function syncInterfaceDraft(iface: OntologyInterface | null) {
    if (!iface) {
      resetInterfaceDraft();
      return;
    }
    setInterfaceName(iface.name);
    setInterfaceDisplayName(iface.display_name);
    setInterfaceDescription(iface.description);
    setSaveError('');
    setSaveSuccess('');
  }

  function resetPropertyDraft() {
    setPropertyName('');
    setPropertyDisplayName('');
    setPropertyDescription('');
    setPropertyType('string');
    setPropertyRequired(false);
    setPropertyUnique(false);
    setPropertyTimeDependent(false);
    setPropertyDefaultValueText('null');
    setPropertyValidationRulesText('{}');
  }

  async function loadBindingMatrix(types: ObjectType[]) {
    setBindingLoading(true);
    try {
      const rows = await Promise.all(
        types.map(async (objectType) => ({
          objectType,
          interfaces: await listTypeInterfaces(objectType.id).catch(() => []),
        })),
      );
      setTypeBindings(rows);
    } catch (error) {
      setPageError(error instanceof Error ? error.message : 'Failed to load interface implementations');
      setTypeBindings([]);
    } finally {
      setBindingLoading(false);
    }
  }

  async function loadInterfaceContext(interfaceId: string, source = interfaces) {
    if (!interfaceId) {
      setProperties([]);
      return;
    }
    setContextLoading(true);
    setPageError('');
    try {
      const propertyResponse = await listInterfaceProperties(interfaceId);
      setProperties(propertyResponse);
      syncInterfaceDraft(source.find((item) => item.id === interfaceId) ?? null);
    } catch (error) {
      setPageError(error instanceof Error ? error.message : 'Failed to load interface definition');
      setProperties([]);
    } finally {
      setContextLoading(false);
    }
  }

  async function loadPage() {
    setLoading(true);
    setPageError('');
    try {
      const [interfaceResponse, typeResponse] = await Promise.all([
        listInterfaces({ page: 1, per_page: 200 }),
        listObjectTypes({ per_page: 200 }),
      ]);
      setInterfaces(interfaceResponse.data);
      setObjectTypes(typeResponse.data);
      const nextId = selectedInterfaceId || interfaceResponse.data[0]?.id || '';
      setSelectedInterfaceId(nextId);
      await Promise.all([loadInterfaceContext(nextId, interfaceResponse.data), loadBindingMatrix(typeResponse.data)]);
    } catch (error) {
      setPageError(error instanceof Error ? error.message : 'Failed to load Interfaces');
    } finally {
      setLoading(false);
    }
  }

  useEffect(() => {
    void loadPage();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  async function selectInterface(interfaceId: string) {
    setSelectedInterfaceId(interfaceId);
    await loadInterfaceContext(interfaceId);
  }

  async function saveInterface() {
    setSaveError('');
    setSaveSuccess('');
    setSavingInterface(true);
    try {
      if (selectedInterfaceId) {
        await updateInterface(selectedInterfaceId, {
          display_name: interfaceDisplayName.trim() || undefined,
          description: interfaceDescription.trim() || undefined,
        });
        setSaveSuccess('Interface updated.');
      } else {
        if (!interfaceName.trim()) throw new Error('Interface name is required.');
        await createInterface({
          name: interfaceName.trim(),
          display_name: interfaceDisplayName.trim() || undefined,
          description: interfaceDescription.trim() || undefined,
        });
        setSaveSuccess('Interface created.');
      }
      const response = await listInterfaces({ page: 1, per_page: 200 });
      setInterfaces(response.data);
      const matched =
        response.data.find((item) => item.name === interfaceName.trim()) ??
        response.data.find((item) => item.id === selectedInterfaceId) ??
        response.data[0] ??
        null;
      const nextId = matched?.id ?? '';
      setSelectedInterfaceId(nextId);
      await Promise.all([loadInterfaceContext(nextId, response.data), loadBindingMatrix(objectTypes)]);
    } catch (error) {
      setSaveError(error instanceof Error ? error.message : 'Failed to save interface');
    } finally {
      setSavingInterface(false);
    }
  }

  async function removeInterface() {
    if (!selectedInterfaceId) return;
    if (typeof window !== 'undefined' && !window.confirm('Delete this interface?')) return;
    setDeletingInterface(true);
    setSaveError('');
    setSaveSuccess('');
    try {
      await deleteInterface(selectedInterfaceId);
      setSaveSuccess('Interface deleted.');
      const response = await listInterfaces({ page: 1, per_page: 200 });
      setInterfaces(response.data);
      const nextId = response.data[0]?.id ?? '';
      setSelectedInterfaceId(nextId);
      await Promise.all([loadInterfaceContext(nextId, response.data), loadBindingMatrix(objectTypes)]);
    } catch (error) {
      setSaveError(error instanceof Error ? error.message : 'Failed to delete interface');
    } finally {
      setDeletingInterface(false);
    }
  }

  async function createPropertyRecord() {
    if (!selectedInterfaceId) return;
    setPropertyError('');
    setPropertySuccess('');
    setSavingProperty(true);
    try {
      if (!propertyName.trim()) throw new Error('Property name is required.');
      await createInterfaceProperty(selectedInterfaceId, {
        name: propertyName.trim(),
        display_name: propertyDisplayName.trim() || undefined,
        description: propertyDescription.trim() || undefined,
        property_type: propertyType,
        required: propertyRequired,
        unique_constraint: propertyUnique,
        time_dependent: propertyTimeDependent,
        default_value: parseLooseJson(propertyDefaultValueText, 'Default value JSON'),
        validation_rules: parseLooseJson(propertyValidationRulesText, 'Validation rules JSON'),
      });
      setPropertySuccess('Interface property created.');
      resetPropertyDraft();
      await loadInterfaceContext(selectedInterfaceId);
    } catch (error) {
      setPropertyError(error instanceof Error ? error.message : 'Failed to create interface property');
    } finally {
      setSavingProperty(false);
    }
  }

  async function toggleRequired(property: InterfaceProperty) {
    if (!selectedInterfaceId) return;
    try {
      await updateInterfaceProperty(selectedInterfaceId, property.id, { required: !property.required });
      await loadInterfaceContext(selectedInterfaceId);
    } catch (error) {
      setPropertyError(error instanceof Error ? error.message : 'Failed to update interface property');
    }
  }

  async function toggleTimeDependent(property: InterfaceProperty) {
    if (!selectedInterfaceId) return;
    try {
      await updateInterfaceProperty(selectedInterfaceId, property.id, { time_dependent: !property.time_dependent });
      await loadInterfaceContext(selectedInterfaceId);
    } catch (error) {
      setPropertyError(error instanceof Error ? error.message : 'Failed to update interface property');
    }
  }

  async function removeProperty(propertyId: string) {
    if (!selectedInterfaceId) return;
    try {
      await deleteInterfaceProperty(selectedInterfaceId, propertyId);
      setPropertySuccess('Interface property deleted.');
      await loadInterfaceContext(selectedInterfaceId);
    } catch (error) {
      setPropertyError(error instanceof Error ? error.message : 'Failed to delete interface property');
    }
  }

  async function attachToType(typeId: string) {
    if (!selectedInterfaceId) return;
    setBindingTypeId(typeId);
    try {
      await attachInterfaceToType(typeId, selectedInterfaceId);
      await loadBindingMatrix(objectTypes);
      setPropertySuccess('Interface attached to object type.');
    } catch (error) {
      setPropertyError(error instanceof Error ? error.message : 'Failed to attach interface');
    } finally {
      setBindingTypeId('');
    }
  }

  async function detachFromType(typeId: string) {
    if (!selectedInterfaceId) return;
    setBindingTypeId(typeId);
    try {
      await detachInterfaceFromType(typeId, selectedInterfaceId);
      await loadBindingMatrix(objectTypes);
      setPropertySuccess('Interface detached from object type.');
    } catch (error) {
      setPropertyError(error instanceof Error ? error.message : 'Failed to detach interface');
    } finally {
      setBindingTypeId('');
    }
  }

  return (
    <section className="of-page" style={{ display: 'grid', gap: 16 }}>
      <div className="of-panel" style={{ padding: 24 }}>
        <div style={{ display: 'grid', gap: 24, gridTemplateColumns: '1.45fr 1fr' }}>
          <div>
            <p className="of-eyebrow" style={{ color: '#047857' }}>
              Define ontologies / interfaces
            </p>
            <h1 className="of-heading-xl" style={{ marginTop: 8 }}>
              Interfaces
            </h1>
            <p className="of-text-muted" style={{ marginTop: 12, fontSize: 14, lineHeight: 1.7 }}>
              Build reusable ontology contracts with interface definitions, canonical properties, and
              implementation bindings across object types from one dedicated product surface.
            </p>
            <div style={{ display: 'flex', flexWrap: 'wrap', gap: 8, marginTop: 12, fontSize: 12 }}>
              {[
                { href: '/ontology-manager', label: 'Ontology Manager' },
                { href: '/object-views', label: 'Object Views' },
                { href: '/action-types', label: 'Action Types' },
              ].map((link) => (
                <a key={link.href} href={link.href} className="of-chip">
                  {link.label}
                </a>
              ))}
            </div>
          </div>

          <div style={{ display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr' }}>
            {[
              { label: 'Interfaces', value: interfaces.length, sub: 'Reusable schema contracts in the ontology.' },
              { label: 'Object types', value: objectTypes.length, sub: 'Candidate implementations across your ontology.' },
              { label: 'Selected properties', value: interfaceStats.propertyCount, sub: 'Interface-level fields and validation requirements.' },
              { label: 'Implementations', value: interfaceStats.implementationCount, sub: 'Object types currently bound to the selected interface.' },
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

      {pageError && (
        <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>
          {pageError}
        </div>
      )}

      {loading ? (
        <div className="of-panel" style={{ padding: 56, textAlign: 'center', fontSize: 13, color: 'var(--text-muted)' }}>
          Loading interfaces…
        </div>
      ) : (
        <div style={{ display: 'grid', gap: 16, gridTemplateColumns: '330px minmax(0, 1fr)' }}>
          <aside style={{ display: 'grid', gap: 16 }}>
            <section className="of-panel" style={{ padding: 20 }}>
              <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
                <div>
                  <p className="of-eyebrow">Library</p>
                  <h2 className="of-heading-md" style={{ marginTop: 4 }}>
                    Interface catalog
                  </h2>
                </div>
                <button type="button" onClick={resetInterfaceDraft} className="of-button of-button--primary" style={{ background: '#059669' }}>
                  + New
                </button>
              </div>

              <label style={{ display: 'block', marginTop: 14, fontSize: 11, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.16em', color: 'var(--text-muted)' }}>
                Search
                <input
                  type="search"
                  value={catalogQuery}
                  onChange={(e) => setCatalogQuery(e.target.value)}
                  placeholder="Search interface name, display name, or description"
                  className="of-input"
                  style={{ marginTop: 6, fontSize: 13 }}
                />
              </label>

              <div style={{ display: 'grid', gap: 6, marginTop: 14 }}>
                {filteredInterfaces.length === 0 ? (
                  <div
                    style={{
                      border: '1px dashed var(--border-default)',
                      borderRadius: 'var(--radius-md)',
                      padding: 20,
                      textAlign: 'center',
                      fontSize: 13,
                      color: 'var(--text-muted)',
                    }}
                  >
                    No interfaces match this filter.
                  </div>
                ) : (
                  filteredInterfaces.map((iface) => {
                    const active = selectedInterfaceId === iface.id;
                    return (
                      <button
                        key={iface.id}
                        type="button"
                        onClick={() => void selectInterface(iface.id)}
                        style={{
                          width: '100%',
                          textAlign: 'left',
                          padding: 12,
                          border: `1px solid ${active ? '#059669' : 'var(--border-default)'}`,
                          background: active ? '#ecfdf5' : 'var(--bg-elevated)',
                          borderRadius: 'var(--radius-md)',
                          cursor: 'pointer',
                        }}
                      >
                        <p style={{ fontWeight: 600, fontSize: 13, color: 'var(--text-strong)' }}>{iface.display_name}</p>
                        <p style={{ marginTop: 2, fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--text-muted)' }}>
                          {iface.name}
                        </p>
                        <p className="of-text-muted" style={{ marginTop: 6, fontSize: 12, overflow: 'hidden', display: '-webkit-box', WebkitLineClamp: 2, WebkitBoxOrient: 'vertical' }}>
                          {iface.description || 'No description provided yet.'}
                        </p>
                      </button>
                    );
                  })
                )}
              </div>
            </section>

            <section className="of-panel" style={{ padding: 20 }}>
              <p className="of-eyebrow">Coverage</p>
              <div style={{ display: 'grid', gap: 8, marginTop: 12 }}>
                <div className="of-panel-muted" style={{ padding: 12 }}>
                  <p style={{ fontWeight: 600, color: 'var(--text-strong)' }}>Required properties</p>
                  <p style={{ marginTop: 6, fontSize: 22, fontWeight: 600 }}>{interfaceStats.requiredPropertyCount}</p>
                </div>
                <div className="of-panel-muted" style={{ padding: 12 }}>
                  <p style={{ fontWeight: 600, color: 'var(--text-strong)' }}>Time-dependent fields</p>
                  <p style={{ marginTop: 6, fontSize: 22, fontWeight: 600 }}>{interfaceStats.timeDependentCount}</p>
                </div>
              </div>
            </section>
          </aside>

          <main style={{ display: 'grid', gap: 12 }}>
            <section className="of-panel" style={{ padding: 14 }}>
              <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
                <div>
                  <p className="of-eyebrow">Workbench</p>
                  <h2 className="of-heading-md" style={{ marginTop: 4 }}>
                    {selectedInterface ? selectedInterface.display_name : 'New interface'}
                  </h2>
                </div>
                <div style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
                  {WORKBENCH_TABS.map((tab) => (
                    <button
                      key={tab.id}
                      type="button"
                      onClick={() => setActiveTab(tab.id)}
                      className={activeTab === tab.id ? 'of-button of-button--primary' : 'of-button'}
                      style={{ fontSize: 13 }}
                    >
                      {tab.label}
                    </button>
                  ))}
                </div>
              </div>
            </section>

            {saveError && <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>{saveError}</div>}
            {saveSuccess && <div className="of-status-success" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13, background: '#ecfdf5', color: '#047857' }}>{saveSuccess}</div>}
            {propertyError && <div className="of-status-danger" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13 }}>{propertyError}</div>}
            {propertySuccess && <div className="of-status-success" style={{ padding: '10px 14px', borderRadius: 'var(--radius-md)', fontSize: 13, background: '#ecfdf5', color: '#047857' }}>{propertySuccess}</div>}

            {activeTab === 'library' && (
              <div style={{ display: 'grid', gap: 12, gridTemplateColumns: 'minmax(0, 1fr) 340px' }}>
                <section className="of-panel" style={{ padding: 20 }}>
                  <div style={{ display: 'grid', gap: 12, gridTemplateColumns: '1fr 1fr' }}>
                    <label style={{ display: 'grid', gap: 6, fontSize: 13 }}>
                      <span style={{ fontWeight: 600 }}>Interface name</span>
                      <input
                        value={interfaceName}
                        onChange={(e) => setInterfaceName(e.target.value)}
                        disabled={Boolean(selectedInterfaceId)}
                        placeholder="case_contract"
                        className="of-input"
                        style={{ fontFamily: 'var(--font-mono)' }}
                      />
                    </label>
                    <label style={{ display: 'grid', gap: 6, fontSize: 13 }}>
                      <span style={{ fontWeight: 600 }}>Display name</span>
                      <input
                        value={interfaceDisplayName}
                        onChange={(e) => setInterfaceDisplayName(e.target.value)}
                        placeholder="Case contract"
                        className="of-input"
                      />
                    </label>
                  </div>
                  <label style={{ display: 'grid', gap: 6, fontSize: 13, marginTop: 12 }}>
                    <span style={{ fontWeight: 600 }}>Description</span>
                    <textarea
                      rows={4}
                      value={interfaceDescription}
                      onChange={(e) => setInterfaceDescription(e.target.value)}
                      placeholder="Describe the shared semantic contract this interface should enforce across object types."
                      className="of-input"
                    />
                  </label>
                  <div style={{ marginTop: 14, display: 'flex', flexWrap: 'wrap', gap: 8 }}>
                    <button
                      type="button"
                      onClick={() => void saveInterface()}
                      disabled={savingInterface}
                      className="of-button of-button--primary"
                      style={{ background: '#059669' }}
                    >
                      {savingInterface ? 'Saving…' : selectedInterfaceId ? 'Save changes' : 'Create interface'}
                    </button>
                    {selectedInterfaceId && (
                      <button
                        type="button"
                        onClick={() => void removeInterface()}
                        disabled={deletingInterface}
                        className="of-button"
                        style={{ borderColor: '#fecaca', color: '#b91c1c' }}
                      >
                        {deletingInterface ? 'Deleting…' : 'Delete interface'}
                      </button>
                    )}
                  </div>
                </section>

                <section className="of-panel" style={{ padding: 20 }}>
                  <p style={{ fontWeight: 600, color: 'var(--text-strong)' }}>Overview</p>
                  <div className="of-text-muted" style={{ marginTop: 12, fontSize: 13, lineHeight: 1.7, display: 'grid', gap: 8 }}>
                    <p>Interfaces define reusable contracts that several object types can implement without duplicating the conceptual schema.</p>
                    <p>Use the definition tab to curate the canonical property set, then attach the interface to object types from the implementation tab.</p>
                    <p>The backend stores interface resources independently from object types, so implementations can evolve while keeping the shared contract visible.</p>
                  </div>
                </section>
              </div>
            )}

            {activeTab === 'definition' && (
              <div style={{ display: 'grid', gap: 12, gridTemplateColumns: 'minmax(0, 1fr) 380px' }}>
                <section className="of-panel" style={{ padding: 20 }}>
                  {contextLoading ? (
                    <div
                      style={{
                        border: '1px dashed var(--border-default)',
                        borderRadius: 'var(--radius-md)',
                        padding: 24,
                        textAlign: 'center',
                        fontSize: 13,
                        color: 'var(--text-muted)',
                      }}
                    >
                      Loading interface definition…
                    </div>
                  ) : !selectedInterfaceId ? (
                    <div
                      style={{
                        border: '1px dashed var(--border-default)',
                        borderRadius: 'var(--radius-md)',
                        padding: 24,
                        textAlign: 'center',
                        fontSize: 13,
                        color: 'var(--text-muted)',
                      }}
                    >
                      Create or select an interface first.
                    </div>
                  ) : (
                    <>
                      <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 8 }}>
                        <div>
                          <p style={{ fontWeight: 600, color: 'var(--text-strong)' }}>Interface properties</p>
                          <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
                            Canonical fields that implementations should expose consistently.
                          </p>
                        </div>
                        <span className="of-chip">{properties.length} properties</span>
                      </div>

                      <div style={{ display: 'grid', gap: 10, marginTop: 14 }}>
                        {properties.length === 0 ? (
                          <div
                            style={{
                              border: '1px dashed var(--border-default)',
                              borderRadius: 'var(--radius-md)',
                              padding: 24,
                              textAlign: 'center',
                              fontSize: 13,
                              color: 'var(--text-muted)',
                            }}
                          >
                            No properties defined yet for this interface.
                          </div>
                        ) : (
                          properties.map((property) => (
                            <div key={property.id} className="of-panel-muted" style={{ padding: 14 }}>
                              <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'flex-start', justifyContent: 'space-between', gap: 8 }}>
                                <div>
                                  <p style={{ fontWeight: 600, color: 'var(--text-strong)' }}>{property.display_name}</p>
                                  <p style={{ marginTop: 4, fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--text-muted)' }}>{property.name}</p>
                                </div>
                                <div style={{ display: 'flex', flexWrap: 'wrap', gap: 4 }}>
                                  <span className="of-chip">{property.property_type}</span>
                                  {property.required && (
                                    <span className="of-chip" style={{ background: '#fffbeb', color: '#b45309' }}>Required</span>
                                  )}
                                  {property.time_dependent && (
                                    <span className="of-chip" style={{ background: '#eff6ff', color: '#1d4ed8' }}>Time dependent</span>
                                  )}
                                </div>
                              </div>
                              {property.description && (
                                <p className="of-text-muted" style={{ marginTop: 10, fontSize: 13 }}>
                                  {property.description}
                                </p>
                              )}
                              <div style={{ marginTop: 12, display: 'flex', flexWrap: 'wrap', gap: 6 }}>
                                <button type="button" onClick={() => void toggleRequired(property)} className="of-button" style={{ fontSize: 12 }}>
                                  {property.required ? 'Mark optional' : 'Mark required'}
                                </button>
                                <button type="button" onClick={() => void toggleTimeDependent(property)} className="of-button" style={{ fontSize: 12 }}>
                                  {property.time_dependent ? 'Disable time dependence' : 'Enable time dependence'}
                                </button>
                                <button
                                  type="button"
                                  onClick={() => void removeProperty(property.id)}
                                  className="of-button"
                                  style={{ fontSize: 12, color: '#b91c1c', borderColor: '#fecaca' }}
                                >
                                  Delete
                                </button>
                              </div>
                            </div>
                          ))
                        )}
                      </div>
                    </>
                  )}
                </section>

                <section className="of-panel" style={{ padding: 20 }}>
                  <p style={{ fontWeight: 600, color: 'var(--text-strong)' }}>Create interface property</p>
                  <div style={{ display: 'grid', gap: 12, marginTop: 14 }}>
                    <label style={{ display: 'grid', gap: 6, fontSize: 13 }}>
                      <span style={{ fontWeight: 600 }}>Property name</span>
                      <input value={propertyName} onChange={(e) => setPropertyName(e.target.value)} placeholder="status" className="of-input" style={{ fontFamily: 'var(--font-mono)' }} />
                    </label>
                    <label style={{ display: 'grid', gap: 6, fontSize: 13 }}>
                      <span style={{ fontWeight: 600 }}>Display name</span>
                      <input value={propertyDisplayName} onChange={(e) => setPropertyDisplayName(e.target.value)} placeholder="Status" className="of-input" />
                    </label>
                    <label style={{ display: 'grid', gap: 6, fontSize: 13 }}>
                      <span style={{ fontWeight: 600 }}>Description</span>
                      <input
                        value={propertyDescription}
                        onChange={(e) => setPropertyDescription(e.target.value)}
                        placeholder="Canonical state field shared across all implementations"
                        className="of-input"
                      />
                    </label>
                    <label style={{ display: 'grid', gap: 6, fontSize: 13 }}>
                      <span style={{ fontWeight: 600 }}>Property type</span>
                      <select value={propertyType} onChange={(e) => setPropertyType(e.target.value)} className="of-input">
                        {PROPERTY_TYPE_OPTIONS.map((option) => (
                          <option key={option} value={option}>
                            {option}
                          </option>
                        ))}
                      </select>
                    </label>
                    <div style={{ display: 'grid', gap: 6, gridTemplateColumns: 'repeat(3, 1fr)' }}>
                      {[
                        { label: 'Required', value: propertyRequired, set: setPropertyRequired },
                        { label: 'Unique', value: propertyUnique, set: setPropertyUnique },
                        { label: 'Time dependent', value: propertyTimeDependent, set: setPropertyTimeDependent },
                      ].map((flag) => (
                        <label
                          key={flag.label}
                          style={{
                            display: 'flex',
                            alignItems: 'center',
                            gap: 6,
                            padding: '8px 10px',
                            border: '1px solid var(--border-default)',
                            borderRadius: 'var(--radius-md)',
                            fontSize: 13,
                          }}
                        >
                          <input type="checkbox" checked={flag.value} onChange={(e) => flag.set(e.target.checked)} />
                          {flag.label}
                        </label>
                      ))}
                    </div>
                    <label style={{ display: 'grid', gap: 6, fontSize: 13 }}>
                      <span style={{ fontWeight: 600 }}>Default value JSON</span>
                      <textarea
                        rows={4}
                        value={propertyDefaultValueText}
                        onChange={(e) => setPropertyDefaultValueText(e.target.value)}
                        spellCheck={false}
                        className="of-input"
                        style={{ background: '#0c0a09', color: '#f5f5f4', fontFamily: 'var(--font-mono)', fontSize: 11 }}
                      />
                    </label>
                    <label style={{ display: 'grid', gap: 6, fontSize: 13 }}>
                      <span style={{ fontWeight: 600 }}>Validation rules JSON</span>
                      <textarea
                        rows={4}
                        value={propertyValidationRulesText}
                        onChange={(e) => setPropertyValidationRulesText(e.target.value)}
                        spellCheck={false}
                        className="of-input"
                        style={{ background: '#0c0a09', color: '#f5f5f4', fontFamily: 'var(--font-mono)', fontSize: 11 }}
                      />
                    </label>
                    <button
                      type="button"
                      onClick={() => void createPropertyRecord()}
                      disabled={!selectedInterfaceId || savingProperty}
                      className="of-button of-button--primary"
                      style={{ background: '#059669' }}
                    >
                      {savingProperty ? 'Creating…' : 'Create property'}
                    </button>
                  </div>
                </section>
              </div>
            )}

            {activeTab === 'implementation' && (
              <div style={{ display: 'grid', gap: 12, gridTemplateColumns: 'minmax(0, 1fr) 380px' }}>
                <section className="of-panel" style={{ padding: 20 }}>
                  <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 8 }}>
                    <div>
                      <p style={{ fontWeight: 600, color: 'var(--text-strong)' }}>Implementing object types</p>
                      <p className="of-text-muted" style={{ marginTop: 4, fontSize: 13 }}>
                        Bind the selected interface to object types that should expose this contract.
                      </p>
                    </div>
                    {bindingLoading && <span style={{ fontSize: 12, color: 'var(--text-muted)' }}>Refreshing…</span>}
                  </div>

                  <div style={{ display: 'grid', gap: 10, marginTop: 14 }}>
                    {!selectedInterfaceId ? (
                      <div
                        style={{
                          border: '1px dashed var(--border-default)',
                          borderRadius: 'var(--radius-md)',
                          padding: 24,
                          textAlign: 'center',
                          fontSize: 13,
                          color: 'var(--text-muted)',
                        }}
                      >
                        Select an interface to manage implementations.
                      </div>
                    ) : implementationRows.length === 0 ? (
                      <div
                        style={{
                          border: '1px dashed var(--border-default)',
                          borderRadius: 'var(--radius-md)',
                          padding: 24,
                          textAlign: 'center',
                          fontSize: 13,
                          color: 'var(--text-muted)',
                        }}
                      >
                        No object types implement this interface yet.
                      </div>
                    ) : (
                      implementationRows.map((row) => (
                        <div key={row.objectType.id} className="of-panel-muted" style={{ padding: 14 }}>
                          <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 8 }}>
                            <div>
                              <p style={{ fontWeight: 600, color: 'var(--text-strong)' }}>{row.objectType.display_name}</p>
                              <p style={{ marginTop: 4, fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--text-muted)' }}>{row.objectType.name}</p>
                            </div>
                            <button
                              type="button"
                              onClick={() => void detachFromType(row.objectType.id)}
                              disabled={bindingTypeId === row.objectType.id}
                              className="of-button"
                              style={{ fontSize: 12, color: '#b91c1c', borderColor: '#fecaca' }}
                            >
                              {bindingTypeId === row.objectType.id ? 'Updating…' : 'Detach'}
                            </button>
                          </div>
                          <p className="of-text-muted" style={{ marginTop: 10, fontSize: 13 }}>
                            {row.objectType.description || 'No object type description provided.'}
                          </p>
                        </div>
                      ))
                    )}
                  </div>
                </section>

                <section className="of-panel" style={{ padding: 20 }}>
                  <p style={{ fontWeight: 600, color: 'var(--text-strong)' }}>Available object types</p>
                  <div style={{ display: 'grid', gap: 10, marginTop: 14 }}>
                    {availableTypes.length === 0 ? (
                      <div
                        style={{
                          border: '1px dashed var(--border-default)',
                          borderRadius: 'var(--radius-md)',
                          padding: 24,
                          textAlign: 'center',
                          fontSize: 13,
                          color: 'var(--text-muted)',
                        }}
                      >
                        Every loaded object type already implements this interface.
                      </div>
                    ) : (
                      availableTypes.map((row) => (
                        <div key={row.objectType.id} className="of-panel-muted" style={{ padding: 14 }}>
                          <div style={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 8 }}>
                            <div>
                              <p style={{ fontWeight: 600, color: 'var(--text-strong)' }}>{row.objectType.display_name}</p>
                              <p style={{ marginTop: 4, fontFamily: 'var(--font-mono)', fontSize: 11, color: 'var(--text-muted)' }}>{row.objectType.name}</p>
                            </div>
                            <button
                              type="button"
                              onClick={() => void attachToType(row.objectType.id)}
                              disabled={!selectedInterfaceId || bindingTypeId === row.objectType.id}
                              className="of-button"
                              style={{ fontSize: 12, color: '#047857', borderColor: '#a7f3d0' }}
                            >
                              {bindingTypeId === row.objectType.id ? 'Updating…' : 'Attach'}
                            </button>
                          </div>
                          <p className="of-text-muted" style={{ marginTop: 10, fontSize: 13 }}>
                            {row.objectType.description || 'No object type description provided.'}
                          </p>
                        </div>
                      ))
                    )}
                  </div>
                </section>
              </div>
            )}

            {activeTab === 'reference' && (
              <div style={{ display: 'grid', gap: 12, gridTemplateColumns: 'minmax(0, 1fr) 360px' }}>
                <section className="of-panel" style={{ padding: 20 }}>
                  <p style={{ fontWeight: 600, color: 'var(--text-strong)' }}>Metadata reference</p>
                  <div style={{ display: 'grid', gap: 10, marginTop: 14, gridTemplateColumns: '1fr 1fr' }}>
                    {[
                      {
                        title: 'Interface resource',
                        detail: 'Each interface has a stable name, a user-facing display_name, descriptive metadata, owner, and timestamps.',
                      },
                      {
                        title: 'Interface properties',
                        detail: 'Properties track property_type, requiredness, uniqueness, time dependence, default values, and validation rules.',
                      },
                      {
                        title: 'Implementation binding',
                        detail: 'Bindings connect an object type to an interface so the contract can be reused across schema definitions.',
                      },
                      {
                        title: 'Edit model',
                        detail: 'Definition and implementation changes are governed independently, which keeps shared contracts reusable without duplicating object-type schemas.',
                      },
                    ].map((card) => (
                      <div key={card.title} className="of-panel-muted" style={{ padding: 14 }}>
                        <p style={{ fontWeight: 600, color: 'var(--text-strong)' }}>{card.title}</p>
                        <p className="of-text-muted" style={{ marginTop: 6, fontSize: 13 }}>
                          {card.detail}
                        </p>
                      </div>
                    ))}
                  </div>
                </section>

                <section className="of-panel" style={{ padding: 20 }}>
                  <p style={{ fontWeight: 600, color: 'var(--text-strong)' }}>Notes</p>
                  <div className="of-text-muted" style={{ marginTop: 14, fontSize: 13, lineHeight: 1.7, display: 'grid', gap: 8 }}>
                    <p>Use interfaces when several object types should expose a shared semantic shape and implementation contract.</p>
                    <p>Keep high-signal fields required here, then attach the interface to every object type that should participate in the contract.</p>
                    <p>Follow with Object Views, Action Types, and Functions once the interface is stable enough to power reusable application behavior.</p>
                  </div>
                </section>
              </div>
            )}
          </main>
        </div>
      )}
    </section>
  );
}
