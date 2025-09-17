CREATE TABLE IF NOT EXISTS parts (
    part_id TEXT PRIMARY KEY,
    description TEXT
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
    supplier_name TEXT
);

CREATE TABLE IF NOT EXISTS parts_suppliers (
    part_id TEXT NOT NULL,
    supplier_id TEXT NOT NULL,
    price DOUBLE PRECISION,
    PRIMARY KEY (part_id, supplier_id),
    FOREIGN KEY (part_id) REFERENCES parts(part_id) ON DELETE CASCADE,
    FOREIGN KEY (supplier_id) REFERENCES suppliers(supplier_id) ON DELETE CASCADE
);

CREATE TABLE IF NOT EXISTS shopping_list (
    list_id INTEGER GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    part_id TEXT NOT NULL,
    quantity INTEGER NOT NULL,
    FOREIGN KEY (part_id) REFERENCES parts(part_id) ON DELETE RESTRICT
);