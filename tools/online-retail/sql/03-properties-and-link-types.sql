SET search_path TO ontology_schema;

-- Properties for transaction (review_status editable enum) + helpful display props
INSERT INTO properties (id, object_type_id, name, display_name, description, property_type, required, unique_constraint, default_value, validation_rules, inline_edit_config) VALUES
  (gen_random_uuid(), '678b55fe-db5f-4d3a-bbf2-8cb643af8d32', 'review_status',  'Review status',   'pending|reviewed|escalated', 'enum',      false, false, '"pending"'::jsonb, '{"enum_values":["pending","reviewed","escalated"]}'::jsonb, '{"enabled":true}'::jsonb),
  (gen_random_uuid(), '678b55fe-db5f-4d3a-bbf2-8cb643af8d32', 'transaction_id', 'Transaction ID',  'invoice_stockcode composite PK', 'string', true,  true,  NULL, NULL, NULL),
  (gen_random_uuid(), '678b55fe-db5f-4d3a-bbf2-8cb643af8d32', 'invoice',        'Invoice',         '', 'string',  false, false, NULL, NULL, NULL),
  (gen_random_uuid(), '678b55fe-db5f-4d3a-bbf2-8cb643af8d32', 'stockcode',      'Stock code',      '', 'string',  false, false, NULL, NULL, NULL),
  (gen_random_uuid(), '678b55fe-db5f-4d3a-bbf2-8cb643af8d32', 'description',    'Description',     '', 'string',  false, false, NULL, NULL, NULL),
  (gen_random_uuid(), '678b55fe-db5f-4d3a-bbf2-8cb643af8d32', 'quantity',       'Quantity',        '', 'integer', false, false, NULL, NULL, NULL),
  (gen_random_uuid(), '678b55fe-db5f-4d3a-bbf2-8cb643af8d32', 'invoice_date',   'Invoice date',    '', 'timestamp', false, false, NULL, NULL, NULL),
  (gen_random_uuid(), '678b55fe-db5f-4d3a-bbf2-8cb643af8d32', 'price',          'Price',           '', 'double',  false, false, NULL, NULL, NULL),
  (gen_random_uuid(), '678b55fe-db5f-4d3a-bbf2-8cb643af8d32', 'customer_id',    'Customer ID',     '', 'integer', false, false, NULL, NULL, NULL),
  (gen_random_uuid(), '678b55fe-db5f-4d3a-bbf2-8cb643af8d32', 'country',        'Country',         '', 'string',  false, false, NULL, NULL, NULL),
  (gen_random_uuid(), '678b55fe-db5f-4d3a-bbf2-8cb643af8d32', 'revenue',        'Revenue',         '', 'double',  false, false, NULL, NULL, NULL),
  (gen_random_uuid(), '678b55fe-db5f-4d3a-bbf2-8cb643af8d32', 'revenue_zscore', 'Revenue z-score', '', 'double',  false, false, NULL, NULL, NULL),
  (gen_random_uuid(), '678b55fe-db5f-4d3a-bbf2-8cb643af8d32', 'is_anomaly',     'Is anomaly',      'abs(z) > 3', 'boolean', false, false, NULL, NULL, NULL),
  -- Customer
  (gen_random_uuid(), '46e2598c-0d11-4ab2-a4aa-301f3e8fb5a7', 'customer_id',    'Customer ID',     '', 'integer', true,  true,  NULL, NULL, NULL),
  (gen_random_uuid(), '46e2598c-0d11-4ab2-a4aa-301f3e8fb5a7', 'total_revenue',  'Total revenue',   '', 'double',  false, false, NULL, NULL, NULL),
  (gen_random_uuid(), '46e2598c-0d11-4ab2-a4aa-301f3e8fb5a7', 'num_orders',     'Number of orders','', 'integer', false, false, NULL, NULL, NULL),
  (gen_random_uuid(), '46e2598c-0d11-4ab2-a4aa-301f3e8fb5a7', 'num_countries',  'Distinct countries','', 'integer', false, false, NULL, NULL, NULL),
  -- Product
  (gen_random_uuid(), '616c7a42-6522-4f94-b696-ddb056cf9b11', 'stockcode',      'Stock code',      '', 'string',  true,  true,  NULL, NULL, NULL),
  (gen_random_uuid(), '616c7a42-6522-4f94-b696-ddb056cf9b11', 'description',    'Description',     '', 'string',  false, false, NULL, NULL, NULL)
ON CONFLICT (object_type_id, name) DO NOTHING;

-- Link types
INSERT INTO link_types (id, name, display_name, description, source_type_id, target_type_id, cardinality, owner_id) VALUES
  (gen_random_uuid(), 'customer_transactions',  'Transactions',  'Customer → Transaction (FK customer_id)', '46e2598c-0d11-4ab2-a4aa-301f3e8fb5a7', '678b55fe-db5f-4d3a-bbf2-8cb643af8d32', 'one_to_many',  '00000000-0000-0000-0000-000000000001'),
  (gen_random_uuid(), 'transaction_product',    'Product',       'Transaction → Product (FK stockcode)',     '678b55fe-db5f-4d3a-bbf2-8cb643af8d32', '616c7a42-6522-4f94-b696-ddb056cf9b11', 'many_to_one',  '00000000-0000-0000-0000-000000000001')
ON CONFLICT (name, source_type_id, target_type_id) DO NOTHING;

SELECT 'properties' AS tbl, COUNT(*) FROM properties WHERE object_type_id IN ('46e2598c-0d11-4ab2-a4aa-301f3e8fb5a7','678b55fe-db5f-4d3a-bbf2-8cb643af8d32','616c7a42-6522-4f94-b696-ddb056cf9b11')
UNION ALL
SELECT 'link_types', COUNT(*) FROM link_types WHERE source_type_id IN ('46e2598c-0d11-4ab2-a4aa-301f3e8fb5a7','678b55fe-db5f-4d3a-bbf2-8cb643af8d32','616c7a42-6522-4f94-b696-ddb056cf9b11');
