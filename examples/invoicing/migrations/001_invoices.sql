-- Portable SQL schema for both PostgreSQL (production) and SQLite (local/tests).
CREATE TABLE invoices (
  id          TEXT PRIMARY KEY,
  customer_id INTEGER NOT NULL,
  status      TEXT NOT NULL DEFAULT 'unpaid' CHECK (status IN ('unpaid', 'paid')),
  subtotal    NUMERIC(12,2) NOT NULL,
  tax         NUMERIC(12,2) NOT NULL,
  total       NUMERIC(12,2) NOT NULL,
  created_at  TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
  paid_at     TIMESTAMPTZ
);

CREATE TABLE invoice_items (
  id          TEXT PRIMARY KEY,
  invoice_id  TEXT NOT NULL REFERENCES invoices(id) ON DELETE CASCADE,
  description TEXT NOT NULL,
  quantity    INTEGER NOT NULL CHECK (quantity > 0),
  unit_price  NUMERIC(12,2) NOT NULL,
  line_total  NUMERIC(12,2) NOT NULL
);

CREATE INDEX idx_invoices_customer_id ON invoices(customer_id);
CREATE INDEX idx_invoice_items_invoice_id ON invoice_items(invoice_id);
