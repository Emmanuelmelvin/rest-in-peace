' main.bas — HTTP, auth, database.
IMPORT { InvoiceTotals } FROM "./billing.bas"

DBCONNECT ENVREQUIRE("DATABASE_URL")
CONST TAX_RATE = DECIMAL(ENV("TAX_RATE", "0.075"))

USE REQUESTLOG()
USE CORS({"origin": ENV("CORS_ORIGIN", "*")})
USE APIKEY("API_KEY") ON "/v1/*"

ROUTE GROUP "/v1"
  ROUTE POST "/invoices"     CALL CreateInvoice
  ROUTE GET  "/invoices/:id" CALL GetInvoice
END GROUP

SERVE

FUNCTION CreateInvoice(req)
  LET b = req.body
  IF b.customerId = NIL OR LEN(b.items) = 0 THEN
    THROW HTTPERROR(400, "CUSTOMER AND ITEMS REQUIRED")
  END IF

  LET totals = InvoiceTotals(b.items, TAX_RATE)

  TRANSACTION
    LET inv = SQLONE("INSERT INTO invoices (customer_id, subtotal, tax, total) VALUES ($1, $2, $3, $4) RETURNING *", b.customerId, totals.subtotal, totals.tax, totals.total)
    FOR EACH item IN b.items
      SQLEXEC("INSERT INTO invoice_items (invoice_id, description, quantity, unit_price) VALUES ($1, $2, $3, $4)", inv.id, item.description, item.quantity, item.unitPrice)
    NEXT
  END TRANSACTION

  LOG "INFO", "INVOICE CREATED", {"invoiceId": inv.id, "total": totals.total}
  RESPOND 201, inv
END FUNCTION

FUNCTION GetInvoice(req)
  LET inv = SQLONE("SELECT * FROM invoices WHERE id = $1", req.params.id)
  IF inv = NIL THEN THROW HTTPERROR(404, "INVOICE IS DEAD")
  inv.items = SQL("SELECT * FROM invoice_items WHERE invoice_id = $1", inv.id)
  RETURN inv
END FUNCTION
