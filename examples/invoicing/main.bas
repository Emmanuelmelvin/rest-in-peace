' main.bas — Invoicing API: HTTP routes, API-key auth, transactions, DECIMAL totals.
IMPORT { LineTotal, InvoiceTotals, CustomerSummary } FROM "./billing.bas"
IMPORT { NewString } FROM GO "github.com/google/uuid"

DBCONNECT ENVREQUIRE("DATABASE_URL")
CONST TAX_RATE = DECIMAL(ENV("TAX_RATE", "0.075"))

USE REQUESTLOG()
USE CORS({"origins": ENV("CORS_ORIGIN", "*")})
USE APIKEY("API_KEY") ON "/v1/*"

ROUTE GROUP "/v1"
  ROUTE POST "/invoices"             CALL CreateInvoice
  ROUTE GET  "/invoices"             CALL ListInvoices
  ROUTE GET  "/invoices/:id"         CALL GetInvoice
  ROUTE POST "/invoices/:id/items"   CALL AddInvoiceItem
  ROUTE POST "/invoices/:id/pay"     CALL MarkInvoicePaid
  ROUTE GET  "/customers/:id/totals" CALL CustomerTotals
END GROUP

SERVE

FUNCTION CreateInvoice(req)
  LET b = req.body
  IF b.customerId = NIL OR LEN(b.items) = 0 THEN
    THROW HTTPERROR(400, "CUSTOMER AND ITEMS REQUIRED")
  END IF

  FOR EACH item IN b.items
    IF item.description = NIL OR LEN(TRIM(item.description)) = 0 THEN
      THROW HTTPERROR(422, "ITEM DESCRIPTION REQUIRED")
    END IF
  NEXT

  LET totals = InvoiceTotals(b.items, TAX_RATE)
  LET invoiceId = NewString()

  TRANSACTION
    LET inv = SQLONE("INSERT INTO invoices (id, customer_id, status, subtotal, tax, total) VALUES ($1, $2, 'unpaid', $3, $4, $5) RETURNING *", invoiceId, b.customerId, totals.subtotal, totals.tax, totals.total)
    FOR EACH item IN b.items
      LET itemId = NewString()
      LET lineAmt = LineTotal(item)
      SQLEXEC("INSERT INTO invoice_items (id, invoice_id, description, quantity, unit_price, line_total) VALUES ($1, $2, $3, $4, $5, $6)", itemId, inv.id, TRIM(item.description), item.quantity, DECIMAL(item.unitPrice), lineAmt)
    NEXT
  END TRANSACTION

  inv.items = SQL("SELECT * FROM invoice_items WHERE invoice_id = $1 ORDER BY id", inv.id)
  LOG "INFO", "INVOICE CREATED", {"invoiceId": inv.id, "customerId": inv.customer_id, "total": totals.total}
  RESPOND 201, inv
END FUNCTION

FUNCTION ListInvoices(req)
  LET customerParam = req.query.customerId
  LET statusParam = req.query.status

  IF statusParam <> NIL AND statusParam <> "unpaid" AND statusParam <> "paid" THEN
    THROW HTTPERROR(400, "INVALID STATUS FILTER")
  END IF

  LET rows = []
  IF customerParam <> NIL AND statusParam <> NIL THEN
    LET cid = INT(VAL(customerParam))
    IF cid <= 0 THEN THROW HTTPERROR(400, "INVALID CUSTOMER ID")
    rows = SQL("SELECT * FROM invoices WHERE customer_id = $1 AND status = $2 ORDER BY created_at DESC, id DESC", cid, statusParam)
  ELSEIF customerParam <> NIL THEN
    LET cid = INT(VAL(customerParam))
    IF cid <= 0 THEN THROW HTTPERROR(400, "INVALID CUSTOMER ID")
    rows = SQL("SELECT * FROM invoices WHERE customer_id = $1 ORDER BY created_at DESC, id DESC", cid)
  ELSEIF statusParam <> NIL THEN
    rows = SQL("SELECT * FROM invoices WHERE status = $1 ORDER BY created_at DESC, id DESC", statusParam)
  ELSE
    rows = SQL("SELECT * FROM invoices ORDER BY created_at DESC, id DESC")
  END IF

  RETURN {"invoices": rows, "count": LEN(rows)}
END FUNCTION

FUNCTION GetInvoice(req)
  LET inv = SQLONE("SELECT * FROM invoices WHERE id = $1", req.params.id)
  IF inv = NIL THEN THROW HTTPERROR(404, "INVOICE IS DEAD")
  inv.items = SQL("SELECT * FROM invoice_items WHERE invoice_id = $1 ORDER BY id", inv.id)
  RETURN inv
END FUNCTION

FUNCTION AddInvoiceItem(req)
  LET inv = SQLONE("SELECT * FROM invoices WHERE id = $1", req.params.id)
  IF inv = NIL THEN THROW HTTPERROR(404, "INVOICE IS DEAD")
  IF inv.status = "paid" THEN
    THROW HTTPERROR(409, "CANNOT MODIFY A PAID INVOICE")
  END IF

  LET item = req.body
  IF item.description = NIL OR LEN(TRIM(item.description)) = 0 THEN
    THROW HTTPERROR(422, "ITEM DESCRIPTION REQUIRED")
  END IF
  LET lineAmt = LineTotal(item)
  LET itemId = NewString()

  LET updated = NIL
  TRANSACTION
    SQLEXEC("INSERT INTO invoice_items (id, invoice_id, description, quantity, unit_price, line_total) VALUES ($1, $2, $3, $4, $5, $6)", itemId, inv.id, TRIM(item.description), item.quantity, DECIMAL(item.unitPrice), lineAmt)
    LET allItems = SQL("SELECT quantity, unit_price AS \"unitPrice\" FROM invoice_items WHERE invoice_id = $1", inv.id)
    LET totals = InvoiceTotals(allItems, TAX_RATE)
    updated = SQLONE("UPDATE invoices SET subtotal = $1, tax = $2, total = $3 WHERE id = $4 RETURNING *", totals.subtotal, totals.tax, totals.total, inv.id)
  END TRANSACTION

  updated.items = SQL("SELECT * FROM invoice_items WHERE invoice_id = $1 ORDER BY id", updated.id)
  LOG "INFO", "INVOICE ITEM ADDED", {"invoiceId": updated.id, "itemId": itemId, "total": updated.total}
  RESPOND 201, updated
END FUNCTION

FUNCTION MarkInvoicePaid(req)
  LET inv = SQLONE("SELECT * FROM invoices WHERE id = $1", req.params.id)
  IF inv = NIL THEN THROW HTTPERROR(404, "INVOICE IS DEAD")
  IF inv.status = "paid" THEN
    THROW HTTPERROR(409, "INVOICE IS ALREADY PAID")
  END IF

  LET paid = SQLONE("UPDATE invoices SET status = 'paid', paid_at = CURRENT_TIMESTAMP WHERE id = $1 RETURNING *", inv.id)
  paid.items = SQL("SELECT * FROM invoice_items WHERE invoice_id = $1 ORDER BY id", paid.id)
  LOG "INFO", "INVOICE PAID", {"invoiceId": paid.id, "total": paid.total}
  RETURN paid
END FUNCTION

FUNCTION CustomerTotals(req)
  LET cid = INT(VAL(req.params.id))
  IF cid <= 0 THEN THROW HTTPERROR(400, "INVALID CUSTOMER ID")
  LET rows = SQL("SELECT * FROM invoices WHERE customer_id = $1 ORDER BY created_at DESC", cid)
  RETURN CustomerSummary(cid, rows)
END FUNCTION
