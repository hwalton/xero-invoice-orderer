-- Dummy data for a campervan conversion business
-- idempotent inserts so running twice is safe

BEGIN;

-- Suppliers
INSERT INTO suppliers (supplier_id, supplier_name, contact_email, phone, created_at) VALUES
  ('S-001', 'Van Supplies Ltd', 'sales@vansupplies.example', '+44 20 7123 0001', now()),
  ('S-002', 'Eco Materials Co', 'info@ecomaterials.example', '+44 20 7123 0002', now()),
  ('S-003', 'ElectroParts', 'sales@electroparts.example', '+44 20 7123 0003', now()),
  ('S-004', 'PlumbRight', 'contact@plumbright.example', '+44 20 7123 0004', now())
ON CONFLICT (supplier_id) DO NOTHING;

-- Core parts (only columns present in current parts table: part_id, name, description, cost_price, sales_price, created_at, updated_at)
INSERT INTO parts (part_id, name, description, cost_price, sales_price, created_at, updated_at) VALUES
  ('P-0001', 'Marine-grade Plywood 18mm', 'High-quality marine plywood, moisture resistant, 1220x2440mm', 60.00, 85.00, now(), now()),
  ('P-0002', 'Insulation Foam 25mm', 'Closed-cell insulation foam sheet, 25mm thick', 6.50, 12.50, now(), now()),
  ('P-0003', '12V LED Strip (5m)', 'Flexible 12V LED strip, 5m roll, warm white', 9.00, 22.00, now(), now()),
  ('P-0004', '12V Water Pump', 'Compact 12V diaphragm water pump for camper sinks', 28.00, 55.00, now(), now()),
  ('P-0005', 'Camper Sink 300x300mm', 'Stainless steel sink, 300x300mm with drain', 25.00, 45.00, now(), now()),
  ('P-0006', 'Propane 2-burner Cooker', 'Portable 2-burner propane cooker for van kitchens', 80.00, 120.00, now(), now()),
  ('P-0007', '100Ah 12V Lithium Battery', 'High-capacity 100Ah 12V LiFePO4 battery', 450.00, 650.00, now(), now()),
  ('P-0008', 'Battery Charger / B2B', 'Battery-to-battery charger for alternator charging', 140.00, 220.00, now(), now()),
  ('KIT-001', 'Kitchen Unit (assembly)', 'Pre-assembled kitchen unit kit (carcass + fittings)', 240.00, 420.00, now(), now()),
  ('P-0009', 'Cabinet Hinge (pair)', 'Concealed cabinet hinge, sold as a pair', 2.50, 6.50, now(), now()),
  ('P-0010', 'Stainless Screws 4x20mm (pack 50)', 'Pack of 50 stainless steel wood screws, 4x20mm', 1.20, 4.50, now(), now())
ON CONFLICT (part_id) DO NOTHING;

-- parts_suppliers (part_id, supplier_id)
INSERT INTO parts_suppliers (part_id, supplier_id) VALUES
  ('P-0001','S-001'),
  ('P-0002','S-002'),
  ('P-0003','S-003'),
  ('P-0004','S-004'),
  ('P-0005','S-004'),
  ('P-0006','S-001'),
  ('P-0007','S-003'),
  ('P-0008','S-003'),
  ('P-0009','S-001'),
  ('P-0010','S-001')
ON CONFLICT (part_id, supplier_id) DO NOTHING;

-- parent_child relationships (parent, child, quantity) — after all parts exist
INSERT INTO parent_child (parent, child, quantity) VALUES
  ('KIT-001','P-0001',3),   -- 3 plywood sheets
  ('KIT-001','P-0005',1),   -- sink
  ('KIT-001','P-0009',4)    -- hinges
ON CONFLICT (parent, child) DO NOTHING;

-- Shopping list sample (list_id auto-generated)
INSERT INTO shopping_list (part_id, quantity, note, created_at) VALUES
  ('KIT-001', 2, 'Buy extra plywood for spare cuts', now()),
  ('P-0003', 2, 'LED strips for ceiling and under-cupboards', now()),
  ('P-0005', 1, 'Sink for kitchen build', now())
ON CONFLICT DO NOTHING;

-- Example updates: ensure supplier_id references exist for parts added afterwards
-- (parts table no longer holds supplier_id directly; these updates are harmless if kept)
UPDATE parts SET -- no-op updates kept for compatibility
  updated_at = updated_at
WHERE part_id IN ('P-0009','P-0010');

COMMIT;