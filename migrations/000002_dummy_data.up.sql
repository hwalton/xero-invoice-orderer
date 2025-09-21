BEGIN;

-- items_contacts now links by contact_id
INSERT INTO items_contacts (item_id, contact_id, created_at, updated_at) VALUES
  ('P-0001','S-001', (extract(epoch from now()))::bigint, (extract(epoch from now()))::bigint),
  ('P-0002','S-002', (extract(epoch from now()))::bigint, (extract(epoch from now()))::bigint),
  ('P-0003','S-003', (extract(epoch from now()))::bigint, (extract(epoch from now()))::bigint),
  ('P-0004','S-004', (extract(epoch from now()))::bigint, (extract(epoch from now()))::bigint),
  ('P-0005','S-004', (extract(epoch from now()))::bigint, (extract(epoch from now()))::bigint),
  ('P-0006','S-001', (extract(epoch from now()))::bigint, (extract(epoch from now()))::bigint),
  ('P-0007','S-003', (extract(epoch from now()))::bigint, (extract(epoch from now()))::bigint),
  ('P-0008','S-003', (extract(epoch from now()))::bigint, (extract(epoch from now()))::bigint),
  ('P-0009','S-001', (extract(epoch from now()))::bigint, (extract(epoch from now()))::bigint),
  ('P-0010','S-001', (extract(epoch from now()))::bigint, (extract(epoch from now()))::bigint)
ON CONFLICT (item_id, contact_id) DO NOTHING;

-- parent_id_child_id relationships (parent_id, child_id, quantity) — after all items exist
INSERT INTO parent_child (parent_id, child_id, quantity, created_at, updated_at) VALUES
  ('KIT-001','P-0001',3, (extract(epoch from now()))::bigint, (extract(epoch from now()))::bigint),
  ('KIT-001','P-0005',1, (extract(epoch from now()))::bigint, (extract(epoch from now()))::bigint),
  ('KIT-001','P-0009',4, (extract(epoch from now()))::bigint, (extract(epoch from now()))::bigint)
ON CONFLICT (parent_id, child_id) DO NOTHING;

COMMIT;