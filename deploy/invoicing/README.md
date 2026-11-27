# Deploying the Invoicing API Showcase

This directory contains a self-contained Docker Compose setup for the
`examples/invoicing` API backed by PostgreSQL 16.

## Quick start (Docker Compose)

1. Set an API key and database password (never commit real secrets):

   ```bash
   export API_KEY="your-secret-key"
   export POSTGRES_PASSWORD="your-db-password"
   ```

2. Build the static binary with `rip build` and start Postgres + the API:

   ```bash
   cd deploy/invoicing
   docker compose up --build
   ```

   On a fresh Postgres volume, `examples/invoicing/migrations/*.sql` is applied
   automatically via `/docker-entrypoint-initdb.d`. For subsequent schema
   changes against an existing database, run:

   ```bash
   DATABASE_URL="postgres://invoicing:$POSTGRES_PASSWORD@localhost:5432/invoicing?sslmode=disable" \
     rip migrate examples/invoicing/main.bas
   ```

## Smoke test

```bash
# Public health check
curl http://localhost:8080/_health

# Create an invoice
curl -X POST http://localhost:8080/v1/invoices \
  -H "Content-Type: application/json" \
  -H "X-Api-Key: $API_KEY" \
  -d '{"customerId": 1, "items": [{"description": "BASIC Resurrection", "quantity": 2, "unitPrice": "100.00"}]}'

# List invoices
curl http://localhost:8080/v1/invoices -H "X-Api-Key: $API_KEY"

# Customer totals
curl http://localhost:8080/v1/customers/1/totals -H "X-Api-Key: $API_KEY"
```

## Endpoints

| Method | Path | Auth | Description |
|---|---|---|---|
| `GET` | `/_health` | Public | Returns `200 {"status": "ALIVE"}` |
| `POST` | `/v1/invoices` | `X-Api-Key` | Create an invoice + line items in a transaction |
| `GET` | `/v1/invoices` | `X-Api-Key` | List invoices (`?customerId=`, `?status=unpaid\|paid`) |
| `GET` | `/v1/invoices/:id` | `X-Api-Key` | Fetch an invoice and its items (`404 INVOICE IS DEAD`) |
| `POST` | `/v1/invoices/:id/items` | `X-Api-Key` | Add a line item to an unpaid invoice and recompute totals |
| `POST` | `/v1/invoices/:id/pay` | `X-Api-Key` | Mark an unpaid invoice as paid |
| `GET` | `/v1/customers/:id/totals` | `X-Api-Key` | Exact `DECIMAL` billing summary for a customer |
