BEGIN;

CREATE OR REPLACE FUNCTION set_updated_at_epoch() RETURNS trigger AS $$
BEGIN
  NEW.updated_at := (extract(epoch from now()))::bigint;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

/* Attach triggers to tables that have updated_at */
CREATE TRIGGER parts_set_updated_at
  BEFORE UPDATE ON parts
  FOR EACH ROW EXECUTE FUNCTION set_updated_at_epoch();

CREATE TRIGGER parent_child_set_updated_at
  BEFORE UPDATE ON parent_child
  FOR EACH ROW EXECUTE FUNCTION set_updated_at_epoch();

CREATE TRIGGER suppliers_set_updated_at
  BEFORE UPDATE ON suppliers
  FOR EACH ROW EXECUTE FUNCTION set_updated_at_epoch();

CREATE TRIGGER parts_suppliers_set_updated_at
  BEFORE UPDATE ON parts_suppliers
  FOR EACH ROW EXECUTE FUNCTION set_updated_at_epoch();

CREATE TRIGGER shopping_list_set_updated_at
  BEFORE UPDATE ON shopping_list
  FOR EACH ROW EXECUTE FUNCTION set_updated_at_epoch();

CREATE TRIGGER xero_connections_set_updated_at
  BEFORE UPDATE ON xero_connections
  FOR EACH ROW EXECUTE FUNCTION set_updated_at_epoch();

CREATE TRIGGER oauth_states_set_updated_at
  BEFORE UPDATE ON oauth_states
  FOR EACH ROW EXECUTE FUNCTION set_updated_at_epoch();

COMMIT;