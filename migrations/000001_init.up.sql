CREATE TABLE IF NOT EXISTS parts (
    part_id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    description TEXT,
    cost_price NUMERIC(12,2),
    sales_price NUMERIC(12,2),
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE IF NOT EXISTS parent_child (
    parent TEXT NOT NULL,
    child TEXT NOT NULL,
    quantity INTEGER DEFAULT 1,
    PRIMARY KEY (parent, child),
    FOREIGN KEY (parent) REFERENCES parts(part_id) ON DELETE CASCADE,
    FOREIGN KEY (child) REFERENCES parts(part_id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS suppliers (
    supplier_id TEXT PRIMARY KEY,
    supplier_name TEXT,
    contact_email TEXT,
    phone TEXT,
    created_at TIMESTAMPTZ DEFAULT now()
);

CREATE TABLE IF NOT EXISTS parts_suppliers (
    part_id TEXT NOT NULL,
    supplier_id TEXT NOT NULL,
    price NUMERIC(12,2),
    PRIMARY KEY (part_id, supplier_id),
    FOREIGN KEY (part_id) REFERENCES parts(part_id) ON DELETE CASCADE,
    FOREIGN KEY (supplier_id) REFERENCES suppliers(supplier_id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS shopping_list (
    list_id INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    part_id TEXT NOT NULL,
    quantity INTEGER NOT NULL DEFAULT 1,
    note TEXT,
    created_at TIMESTAMPTZ DEFAULT now(),
    FOREIGN KEY (part_id) REFERENCES parts(part_id) ON DELETE RESTRICT
);


ALTER TABLE parts ENABLE ROW LEVEL SECURITY;
CREATE POLICY allow_authenticated_read_on_parts
  ON parts
  FOR SELECT
  USING (auth.uid() IS NOT NULL);

ALTER TABLE parent_child ENABLE ROW LEVEL SECURITY;
CREATE POLICY allow_authenticated_read_on_parent_child
  ON parent_child
  FOR SELECT
  USING (auth.uid() IS NOT NULL);

ALTER TABLE suppliers ENABLE ROW LEVEL SECURITY;
CREATE POLICY allow_authenticated_read_on_suppliers
  ON suppliers
  FOR SELECT
  USING (auth.uid() IS NOT NULL);

ALTER TABLE parts_suppliers ENABLE ROW LEVEL SECURITY;
CREATE POLICY allow_authenticated_read_on_parts_suppliers
  ON parts_suppliers
  FOR SELECT
  USING (auth.uid() IS NOT NULL);

ALTER TABLE shopping_list ENABLE ROW LEVEL SECURITY;
CREATE POLICY allow_authenticated_read_on_shopping_list
  ON shopping_list
  FOR SELECT
  USING (auth.uid() IS NOT NULL);
