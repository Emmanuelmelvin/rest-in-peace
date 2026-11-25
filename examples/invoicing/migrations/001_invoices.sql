-- Postgres syntax. Whether migrations must also run on SQLite for `rip test`
-- is an open decision (see "Decisions to confirm" in requirements.md).
CREATE TABLE invoices (
  id          SERIAL PRIMARY KEY,
  customer_id INTEGER NOT NULL,
  subtotal    NUMERIC(12,2) NOT NULL,
  tax         NUMERIC(12,2) NOT NULL,
  total       NUMERIC(12,2) NOT NULL,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE invoice_items (
  id          SERIAL PRIMARY KEY,
  invoice_id  INTEGER NOT NULL REFERENCES invoices(id),
  description TEXT NOT NULL,
  quantity    INTEGER NOT NULL CHECK (quantity > 0),
  unit_price  NUMERIC(12,2) NOT NULL
);
