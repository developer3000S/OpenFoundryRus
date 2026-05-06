import { createBrowserRouter } from 'react-router-dom';

import { AppShell } from '@components/AppShell';
import { AuthLayout } from '@components/AuthLayout';
import { Home } from './routes/Home';
import { NotFound } from './routes/NotFound';

export const router = createBrowserRouter([
  {
    path: '/auth',
    element: <AuthLayout />,
    children: [
      {
        path: 'login',
        lazy: async () => ({ Component: (await import('./routes/auth/LoginPage')).LoginPage }),
      },
      {
        path: 'register',
        lazy: async () => ({ Component: (await import('./routes/auth/RegisterPage')).RegisterPage }),
      },
      {
        path: 'mfa',
        lazy: async () => ({ Component: (await import('./routes/auth/MfaPage')).MfaPage }),
      },
      {
        path: 'callback',
        lazy: async () => ({ Component: (await import('./routes/auth/CallbackPage')).CallbackPage }),
      },
    ],
  },
  {
    path: '/',
    element: <AppShell />,
    errorElement: <NotFound />,
    children: [
      { index: true, element: <Home /> },
      {
        path: 'settings',
        lazy: async () => ({ Component: (await import('./routes/settings/SettingsPage')).SettingsPage }),
      },
      {
        path: 'dashboards',
        lazy: async () => ({ Component: (await import('./routes/dashboards/DashboardsListPage')).DashboardsListPage }),
      },
      {
        path: 'dashboards/:id',
        lazy: async () => ({ Component: (await import('./routes/dashboards/DashboardDetailPage')).DashboardDetailPage }),
      },
      {
        path: 'lineage',
        lazy: async () => ({ Component: (await import('./routes/lineage/LineagePage')).LineagePage }),
      },
      {
        path: 'notebooks',
        lazy: async () => ({ Component: (await import('./routes/notebooks/NotebooksListPage')).NotebooksListPage }),
      },
      {
        path: 'notebooks/:id',
        lazy: async () => ({ Component: (await import('./routes/notebooks/NotebookDetailPage')).NotebookDetailPage }),
      },
      {
        path: 'contour',
        lazy: async () => ({ Component: (await import('./routes/contour/ContourPage')).ContourPage }),
      },
      {
        path: 'geospatial',
        lazy: async () => ({ Component: (await import('./routes/geospatial/GeospatialPage')).GeospatialPage }),
      },
      {
        path: 'search',
        lazy: async () => ({ Component: (await import('./routes/search/SearchPage')).SearchPage }),
      },
      {
        path: 'queries',
        lazy: async () => ({ Component: (await import('./routes/queries/QueriesPage')).QueriesPage }),
      },
      {
        path: 'quiver',
        lazy: async () => ({ Component: (await import('./routes/quiver/QuiverPage')).QuiverPage }),
      },
      {
        path: 'vertex',
        lazy: async () => ({ Component: (await import('./routes/vertex/VertexPage')).VertexPage }),
      },
      {
        path: 'notepad',
        lazy: async () => ({ Component: (await import('./routes/notepad/NotepadListPage')).NotepadListPage }),
      },
      {
        path: 'notepad/:id',
        lazy: async () => ({ Component: (await import('./routes/notepad/NotepadDetailPage')).NotepadDetailPage }),
      },
      {
        path: 'reports',
        lazy: async () => ({ Component: (await import('./routes/reports/ReportsPage')).ReportsPage }),
      },
      {
        path: 'global-branching',
        lazy: async () => ({ Component: (await import('./routes/global-branching/GlobalBranchingPage')).GlobalBranchingPage }),
      },
      {
        path: 'developers',
        lazy: async () => ({ Component: (await import('./routes/developers/DevelopersPage')).DevelopersPage }),
      },
      {
        path: 'object-databases',
        lazy: async () => ({ Component: (await import('./routes/object-databases/ObjectDatabasesPage')).ObjectDatabasesPage }),
      },
      {
        path: 'workflows',
        lazy: async () => ({ Component: (await import('./routes/workflows/WorkflowsPage')).WorkflowsPage }),
      },
      {
        path: 'ontology-design',
        lazy: async () => ({ Component: (await import('./routes/ontology-design/OntologyDesignPage')).OntologyDesignPage }),
      },
      {
        path: 'dynamic-scheduling',
        lazy: async () => ({ Component: (await import('./routes/dynamic-scheduling/DynamicSchedulingPage')).DynamicSchedulingPage }),
      },
      {
        path: 'interfaces',
        lazy: async () => ({ Component: (await import('./routes/interfaces/InterfacesPage')).InterfacesPage }),
      },
      {
        path: 'build-schedules',
        lazy: async () => ({ Component: (await import('./routes/build-schedules/BuildSchedulesPage')).BuildSchedulesPage }),
      },
      {
        path: 'build-schedules/sweep',
        lazy: async () => ({ Component: (await import('./routes/build-schedules/SweepPage')).SweepPage }),
      },
      {
        path: 'fusion',
        lazy: async () => ({ Component: (await import('./routes/fusion/FusionPage')).FusionPage }),
      },
      {
        path: 'nexus',
        lazy: async () => ({ Component: (await import('./routes/nexus/NexusPage')).NexusPage }),
      },
      {
        path: 'audit',
        lazy: async () => ({ Component: (await import('./routes/audit/AuditPage')).AuditPage }),
      },
      {
        path: 'code-repos',
        lazy: async () => ({ Component: (await import('./routes/code-repos/CodeReposPage')).CodeReposPage }),
      },
      {
        path: 'marketplace',
        lazy: async () => ({ Component: (await import('./routes/marketplace/MarketplacePage')).MarketplacePage }),
      },
      {
        path: 'marketplace/:id',
        lazy: async () => ({ Component: (await import('./routes/marketplace/MarketplaceProductPage')).MarketplaceProductPage }),
      },
      {
        path: 'virtual-tables',
        lazy: async () => ({ Component: (await import('./routes/virtual-tables/VirtualTablesPage')).VirtualTablesPage }),
      },
      {
        path: 'virtual-tables/:rid',
        lazy: async () => ({ Component: (await import('./routes/virtual-tables/VirtualTableDetailPage')).VirtualTableDetailPage }),
      },
      {
        path: 'ai',
        lazy: async () => ({ Component: (await import('./routes/ai/AiPage')).AiPage }),
      },
      {
        path: 'object-views',
        lazy: async () => ({ Component: (await import('./routes/object-views/ObjectViewsPage')).ObjectViewsPage }),
      },
      {
        path: 'object-explorer',
        lazy: async () => ({ Component: (await import('./routes/object-explorer/ObjectExplorerPage')).ObjectExplorerPage }),
      },
      {
        path: 'iceberg-tables',
        lazy: async () => ({ Component: (await import('./routes/iceberg-tables/IcebergTablesPage')).IcebergTablesPage }),
      },
      {
        path: 'iceberg-tables/:id',
        lazy: async () => ({ Component: (await import('./routes/iceberg-tables/IcebergTableDetailPage')).IcebergTableDetailPage }),
      },
      {
        path: 'ontology-indexing',
        lazy: async () => ({ Component: (await import('./routes/ontology-indexing/OntologyIndexingPage')).OntologyIndexingPage }),
      },
      {
        path: 'ontologies',
        lazy: async () => ({ Component: (await import('./routes/ontologies/OntologiesPage')).OntologiesPage }),
      },
      {
        path: 'object-monitors',
        lazy: async () => ({ Component: (await import('./routes/object-monitors/ObjectMonitorsPage')).ObjectMonitorsPage }),
      },
      {
        path: 'streaming',
        lazy: async () => ({ Component: (await import('./routes/streaming/StreamingPage')).StreamingPage }),
      },
      {
        path: 'streaming/:id',
        lazy: async () => ({ Component: (await import('./routes/streaming/StreamingDetailPage')).StreamingDetailPage }),
      },
      {
        path: 'machinery',
        lazy: async () => ({ Component: (await import('./routes/machinery/MachineryPage')).MachineryPage }),
      },
      {
        path: 'media-sets',
        lazy: async () => ({ Component: (await import('./routes/media-sets/MediaSetsPage')).MediaSetsPage }),
      },
      {
        path: 'media-sets/:rid',
        lazy: async () => ({ Component: (await import('./routes/media-sets/MediaSetDetailPage')).MediaSetDetailPage }),
      },
      {
        path: 'object-link-types',
        lazy: async () => ({ Component: (await import('./routes/object-link-types/ObjectLinkTypesPage')).ObjectLinkTypesPage }),
      },
      {
        path: 'builds',
        lazy: async () => ({ Component: (await import('./routes/builds/BuildsPage')).BuildsPage }),
      },
      {
        path: 'builds/:rid',
        lazy: async () => ({ Component: (await import('./routes/builds/BuildDetailPage')).BuildDetailPage }),
      },
      {
        path: 'foundry-rules',
        lazy: async () => ({ Component: (await import('./routes/foundry-rules/FoundryRulesPage')).FoundryRulesPage }),
      },
      {
        path: 'control-panel',
        lazy: async () => ({ Component: (await import('./routes/control-panel/ControlPanelPage')).ControlPanelPage }),
      },
      {
        path: 'control-panel/streaming-profiles',
        lazy: async () => ({ Component: (await import('./routes/control-panel/StreamingProfilesPage')).StreamingProfilesPage }),
      },
      {
        path: 'control-panel/data-health',
        lazy: async () => ({ Component: (await import('./routes/control-panel/DataHealthPage')).DataHealthPage }),
      },
      {
        path: 'functions',
        lazy: async () => ({ Component: (await import('./routes/functions/FunctionsPage')).FunctionsPage }),
      },
      {
        path: 'pipelines',
        lazy: async () => ({ Component: (await import('./routes/pipelines/PipelinesPage')).PipelinesPage }),
      },
      {
        path: 'pipelines/new',
        lazy: async () => ({ Component: (await import('./routes/pipelines/PipelineNewPage')).PipelineNewPage }),
      },
      {
        path: 'pipelines/:id/edit',
        lazy: async () => ({ Component: (await import('./routes/pipelines/PipelineEditPage')).PipelineEditPage }),
      },
      {
        path: 'schedules/:rid',
        lazy: async () => ({ Component: (await import('./routes/schedules/ScheduleDetailPage')).ScheduleDetailPage }),
      },
      {
        path: 'ml',
        lazy: async () => ({ Component: (await import('./routes/ml/MlPage')).MlPage }),
      },
      {
        path: 'action-types',
        lazy: async () => ({ Component: (await import('./routes/action-types/ActionTypesPage')).ActionTypesPage }),
      },
      {
        path: 'datasets',
        lazy: async () => ({ Component: (await import('./routes/datasets/DatasetsListPage')).DatasetsListPage }),
      },
      {
        path: 'datasets/upload',
        lazy: async () => ({ Component: (await import('./routes/datasets/DatasetUploadPage')).DatasetUploadPage }),
      },
      {
        path: 'datasets/:id',
        lazy: async () => ({ Component: (await import('./routes/datasets/DatasetDetailPage')).DatasetDetailPage }),
      },
      {
        path: 'datasets/:id/branches',
        lazy: async () => ({ Component: (await import('./routes/datasets/DatasetBranchesPage')).DatasetBranchesPage }),
      },
      {
        path: 'datasets/:id/branches/:branch',
        lazy: async () => ({ Component: (await import('./routes/datasets/DatasetBranchDetailPage')).DatasetBranchDetailPage }),
      },
      {
        path: 'apps',
        lazy: async () => ({ Component: (await import('./routes/apps/AppsPage')).AppsPage }),
      },
      {
        path: 'apps/runtime/:slug',
        lazy: async () => ({ Component: (await import('./routes/apps/AppRuntimePage')).AppRuntimePage }),
      },
      {
        path: 'data-connection',
        lazy: async () => ({ Component: (await import('./routes/data-connection/DataConnectionPage')).DataConnectionPage }),
      },
      {
        path: 'data-connection/agents',
        lazy: async () => ({ Component: (await import('./routes/data-connection/AgentsPage')).AgentsPage }),
      },
      {
        path: 'data-connection/egress-policies',
        lazy: async () => ({ Component: (await import('./routes/data-connection/EgressPoliciesPage')).EgressPoliciesPage }),
      },
      {
        path: 'data-connection/new',
        lazy: async () => ({ Component: (await import('./routes/data-connection/NewSourcePage')).NewSourcePage }),
      },
      {
        path: 'data-connection/new/streaming',
        lazy: async () => ({ Component: (await import('./routes/data-connection/NewStreamingSourcePage')).NewStreamingSourcePage }),
      },
      {
        path: 'data-connection/sources/:id',
        lazy: async () => ({ Component: (await import('./routes/data-connection/SourceDetailPage')).SourceDetailPage }),
      },
      {
        path: 'projects',
        lazy: async () => ({ Component: (await import('./routes/projects/ProjectsListPage')).ProjectsListPage }),
      },
      {
        path: 'projects/:projectId',
        lazy: async () => ({ Component: (await import('./routes/projects/ProjectDetailPage')).ProjectDetailPage }),
      },
      {
        path: 'projects/:projectId/:folderId',
        lazy: async () => ({ Component: (await import('./routes/projects/ProjectFolderPage')).ProjectFolderPage }),
      },
      {
        path: 'ontology-manager',
        lazy: async () => ({ Component: (await import('./routes/ontology-manager/OntologyManagerPage')).OntologyManagerPage }),
      },
      {
        path: 'ontology-manager/bindings',
        lazy: async () => ({ Component: (await import('./routes/ontology-manager/BindingsWizardPage')).BindingsWizardPage }),
      },
      {
        path: 'ontology',
        lazy: async () => ({ Component: (await import('./routes/ontology/OntologyHomePage')).OntologyHomePage }),
      },
      {
        path: 'ontology/types',
        lazy: async () => ({ Component: (await import('./routes/ontology/CreateObjectTypePage')).CreateObjectTypePage }),
      },
      {
        path: 'ontology/graph',
        lazy: async () => ({ Component: (await import('./routes/ontology/OntologyGraphPage')).OntologyGraphPage }),
      },
      {
        path: 'ontology/object-sets',
        lazy: async () => ({ Component: (await import('./routes/ontology/ObjectSetsPage')).ObjectSetsPage }),
      },
      {
        path: 'ontology/:id',
        lazy: async () => ({ Component: (await import('./routes/ontology/ObjectTypeDetailPage')).ObjectTypeDetailPage }),
      },
      {
        path: 'charts-demo',
        lazy: async () => ({ Component: (await import('./routes/charts-demo/ChartsDemoPage')).ChartsDemoPage }),
      },
      {
        path: 'monaco-demo',
        lazy: async () => ({ Component: (await import('./routes/monaco-demo/MonacoDemoPage')).MonacoDemoPage }),
      },
      {
        path: 'maplibre-demo',
        lazy: async () => ({ Component: (await import('./routes/maplibre-demo/MapLibreDemoPage')).MapLibreDemoPage }),
      },
      {
        path: 'cytoscape-demo',
        lazy: async () => ({ Component: (await import('./routes/cytoscape-demo/CytoscapeDemoPage')).CytoscapeDemoPage }),
      },
      // Migration pattern: add a route here as you port each SvelteKit folder under apps/web/src/routes/.
      { path: '*', element: <NotFound /> },
    ],
  },
]);
