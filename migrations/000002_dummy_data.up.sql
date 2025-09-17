-- Dummy data for a campervan conversion business
-- idempotent inserts so running twice is safe

-- Suppliers
INSERT INTO suppliers (supplier_id, supplier_name, contact_email, phone, created_at) VALUES
  ('S-001', 'Van Supplies Ltd', 'sales@vansupplies.example', '+44 20 7123 0001', now()),
  ('S-002', 'Eco Materials Co', 'info@ecomaterials.example', '+44 20 7123 0002', now()),
  ('S-003', 'ElectroParts', 'sales@electroparts.example', '+44 20 7123 0003', now()),
  ('S-004', 'PlumbRight', 'contact@plumbright.example', '+44 20 7123 0004', now())
ON CONFLICT (supplier_id) DO NOTHING;

-- Core parts (including ones referenced by parent_child)
INSERT INTO parts (part_id, name, description, barcode, sales_price, purchase_price, sales_account_code, purchase_account_code, tax_type, is_tracked, inventory_asset_account_code, supplier_id, created_at, updated_at) VALUES
  ('P-0001', 'Marine-grade Plywood 18mm', '18mm plywood sheet, moisture resistant, 1220x2440mm', 'PLY18-001', 85.00, 60.00, '4000', '5000', 'STANDARD', TRUE, '1500', 'S-001', now(), now()),
  ('P-0002', 'Insulation Foam 25mm', 'Closed-cell insulation roll, 25mm x 1m', 'INS25-001', 12.50, 6.50, '4000', '5000', 'ZERO', TRUE, '1500', 'S-002', now(), now()),
  ('P-0003', '12V LED Strip (5m)', 'Warm white 12V LED strip, IP20', 'LED5M-001', 22.00, 9.00, '4000', '5000', 'ZERO', FALSE, NULL, 'S-003', now(), now()),
  ('P-0004', '12V Water Pump', 'Shurflo-style 12V diaphragm water pump', 'PUMP12V-001', 55.00, 28.00, '4000', '5000', 'STANDARD', FALSE, NULL, 'S-004', now(), now()),
  ('P-0005', 'Camper Sink 300x300mm', 'Stainless steel inset sink', 'SINK300-001', 45.00, 25.00, '4000', '5000', 'STANDARD', FALSE, NULL, 'S-004', now(), now()),
  ('P-0006', 'Propane 2-burner Cooker', 'Compact camper 2-burner cooker, propane', 'COOK2-001', 120.00, 80.00, '4000', '5000', 'STANDARD', FALSE, NULL, 'S-001', now(), now()),
  ('P-0007', '100Ah 12V Lithium Battery', '12V deep-cycle lithium battery, 100Ah', 'BAT100-001', 650.00, 450.00, '4000', '5000', 'STANDARD', TRUE, '1510', 'S-003', now(), now()),
  ('P-0008', 'Battery Charger / B2B', '12V battery charger / DC-DC B2B for alternator charging', 'CHARGER-001', 220.00, 140.00, '4000', '5000', 'STANDARD', FALSE, NULL, 'S-003', now(), now()),
  ('KIT-001', 'Kitchen Unit (assembly)', 'Completed kitchen unit assembly (for parent/child demo)', 'KIT001', 420.00, 240.00, '4000', '5000', 'STANDARD', FALSE, NULL, 'S-001', now(), now())
ON CONFLICT (part_id) DO NOTHING;

-- Add small parts (ensure these exist before parent_child references)
INSERT INTO parts (part_id, name, description, barcode, sales_price, purchase_price, sales_account_code, purchase_account_code, tax_type, is_tracked, inventory_asset_account_code, supplier_id, created_at, updated_at) VALUES
  ('P-0009', 'Cabinet Hinge (pair)', 'Soft-close hinge pair for cabinet doors', 'HINGE-001', 6.50, 2.50, '4000', '5000', 'STANDARD', FALSE, NULL, 'S-001', now(), now()),
  ('P-0010', 'Stainless Screws 4x20mm (pack 50)', 'Stainless steel screws pack', 'SCRW420-050', 4.50, 1.20, '4000', '5000', 'STANDARD', FALSE, NULL, 'S-001', now(), now())
ON CONFLICT (part_id) DO NOTHING;

-- parts_suppliers (part_id, supplier_id, price)
INSERT INTO parts_suppliers (part_id, supplier_id, price) VALUES
  ('P-0001','S-001',60.00),
  ('P-0002','S-002',6.50),
  ('P-0003','S-003',9.00),
  ('P-0004','S-004',28.00),
  ('P-0005','S-004',25.00),
  ('P-0006','S-001',80.00),
  ('P-0007','S-003',450.00),
  ('P-0008','S-003',140.00),
  ('KIT-001','S-001',240.00),
  ('P-0009','S-001',2.50),
  ('P-0010','S-001',1.20)
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
UPDATE parts SET supplier_id = 'S-001' WHERE part_id = 'P-0009' AND supplier_id IS NULL;
UPDATE parts SET supplier_id = 'S-001' WHERE part_id = 'P-0010' AND supplier_id IS NULL;