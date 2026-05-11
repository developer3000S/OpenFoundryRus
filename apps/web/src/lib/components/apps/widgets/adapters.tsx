// Registry adapters — bridge the widget components defined in
// WorkshopEditorPage.tsx (which take typed props like `variables` and
// `objectTypes`) to the registry's uniform `{widget}`-only API.
//
// Each adapter is a thin component that pulls from WorkshopDataContext and
// forwards. This keeps the original components prop-driven (so the editor
// can render them with explicit values) while letting the runtime drive
// them via context.

import {
  ButtonGroupWidgetView,
  ChartPieWidgetView,
  ChartXyWidgetView,
  FilterListWidgetView,
  MapWidgetView,
  ObjectSetTitleWidgetView,
  ObjectTableWidgetView,
  PropertyListWidgetView,
} from '@/routes/apps/WorkshopEditorPage';

import { registerWidget, type WidgetRenderProps } from './registry';
import { useWorkshopData } from './workshop-context';

function ObjectTableAdapter({ widget }: WidgetRenderProps) {
  const { variables } = useWorkshopData();
  return <ObjectTableWidgetView widget={widget} variables={variables} />;
}

function FilterListAdapter({ widget }: WidgetRenderProps) {
  return <FilterListWidgetView widget={widget} />;
}

function ObjectSetTitleAdapter({ widget }: WidgetRenderProps) {
  const { variables, objectTypes } = useWorkshopData();
  return <ObjectSetTitleWidgetView widget={widget} variables={variables} objectTypes={objectTypes} />;
}

function ButtonGroupAdapter({ widget }: WidgetRenderProps) {
  return <ButtonGroupWidgetView widget={widget} />;
}

function PropertyListAdapter({ widget }: WidgetRenderProps) {
  const { variables } = useWorkshopData();
  return <PropertyListWidgetView widget={widget} variables={variables} />;
}

function ChartPieAdapter({ widget }: WidgetRenderProps) {
  const { variables } = useWorkshopData();
  return <ChartPieWidgetView widget={widget} variables={variables} />;
}

function ChartXyAdapter({ widget }: WidgetRenderProps) {
  const { variables } = useWorkshopData();
  return <ChartXyWidgetView widget={widget} variables={variables} />;
}

function MapAdapter({ widget }: WidgetRenderProps) {
  const { variables } = useWorkshopData();
  return <MapWidgetView widget={widget} variables={variables} />;
}

let registered = false;

export function registerWorkshopWidgets(): void {
  if (registered) return;
  registered = true;
  registerWidget({ type: 'object_table', Component: ObjectTableAdapter, label: 'Object table', version: '1.0.0' });
  registerWidget({ type: 'filter_list', Component: FilterListAdapter, label: 'Filter list', version: '1.0.0' });
  registerWidget({ type: 'object_set_title', Component: ObjectSetTitleAdapter, label: 'Object set KPI', version: '1.0.0' });
  registerWidget({ type: 'button_group', Component: ButtonGroupAdapter, label: 'Button group', version: '1.0.0' });
  registerWidget({ type: 'property_list', Component: PropertyListAdapter, label: 'Property list', version: '1.0.0' });
  registerWidget({ type: 'chart_pie', Component: ChartPieAdapter, label: 'Pie chart', version: '1.0.0' });
  registerWidget({ type: 'chart_xy', Component: ChartXyAdapter, label: 'XY chart', version: '1.0.0' });
  registerWidget({ type: 'map', Component: MapAdapter, label: 'Map', version: '1.0.0' });
}
