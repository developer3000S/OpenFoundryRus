SET search_path TO ontology_actions;
INSERT INTO object_types (id, name, display_name, description, primary_key_property, icon, color, owner_id) VALUES
  ('46e2598c-0d11-4ab2-a4aa-301f3e8fb5a7','customer','Customer','Aggregated metrics per customer','customer_id','users','#2d72d2','00000000-0000-0000-0000-000000000001'),
  ('678b55fe-db5f-4d3a-bbf2-8cb643af8d32','transaction','Transaction','Order line with anomaly flag and editable review_status','transaction_id','object','#cf923f','00000000-0000-0000-0000-000000000001'),
  ('616c7a42-6522-4f94-b696-ddb056cf9b11','product','Product','Stock-keeping unit','stockcode','cube','#15803d','00000000-0000-0000-0000-000000000001')
ON CONFLICT (id) DO NOTHING;
