SET search_path TO ontology_actions;
INSERT INTO action_types (id, name, display_name, description, object_type_id, operation_kind, input_schema, config, owner_id) VALUES
  (gen_random_uuid(), 'mark_as_reviewed', 'MarkAsReviewed', 'Set review_status=reviewed', '678b55fe-db5f-4d3a-bbf2-8cb643af8d32', 'update_object', '[]'::jsonb, '{"property_mappings":[{"property_name":"review_status","kind":"static","static_value":"reviewed"}]}'::jsonb, '00000000-0000-0000-0000-000000000001'),
  (gen_random_uuid(), 'escalate_anomaly', 'EscalateAnomaly', 'Set review_status=escalated', '678b55fe-db5f-4d3a-bbf2-8cb643af8d32', 'update_object', '[]'::jsonb, '{"property_mappings":[{"property_name":"review_status","kind":"static","static_value":"escalated"}]}'::jsonb, '00000000-0000-0000-0000-000000000001')
ON CONFLICT (name) DO NOTHING;
SELECT name, display_name, operation_kind FROM action_types WHERE object_type_id = '678b55fe-db5f-4d3a-bbf2-8cb643af8d32';
