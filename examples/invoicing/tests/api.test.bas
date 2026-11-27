' api.test.bas — HTTP integration tests for the Invoicing API.

TEST "HEALTH CHECK IS PUBLIC"
  LET r = REQUEST("GET", "/_health")
  EXPECT r.status = 200
  EXPECT r.body.status = "ALIVE"
END TEST

TEST "REJECTS MISSING API KEY"
  LET r = REQUEST("POST", "/v1/invoices", {"customerId": 1, "items": []})
  EXPECT r.status = 401
END TEST

TEST "REJECTS NEGATIVE QUANTITY"
  LET r = REQUEST("POST", "/v1/invoices", {"customerId": 1, "items": [{"description": "Coffin", "quantity": -1, "unitPrice": "50.00"}]}, {"x-api-key": "test-key"})
  EXPECT r.status = 422
END TEST

TEST "RETURNS 404 FOR MISSING INVOICE"
  LET r = REQUEST("GET", "/v1/invoices/does-not-exist", NIL, {"x-api-key": "test-key"})
  EXPECT r.status = 404
  EXPECT r.body.error = "INVOICE IS DEAD"
END TEST

TEST "FULL INVOICE LIFECYCLE"
  LET hdrs = {"x-api-key": "test-key"}

  ' Create invoice with one item
  LET created = REQUEST("POST", "/v1/invoices", {"customerId": 42, "items": [{"description": "BASIC Resurrection", "quantity": 2, "unitPrice": "100.00"}]}, hdrs)
  EXPECT created.status = 201
  EXPECT created.body.status = "unpaid"
  EXPECT created.body.subtotal = "200.00"
  EXPECT created.body.tax = "15.00"
  EXPECT created.body.total = "215.00"
  EXPECT LEN(created.body.items) = 1

  LET id = created.body.id

  ' Fetch invoice by ID
  LET fetched = REQUEST("GET", "/v1/invoices/" & id, NIL, hdrs)
  EXPECT fetched.status = 200
  EXPECT fetched.body.id = id
  EXPECT LEN(fetched.body.items) = 1

  ' Add a second item (recalculates totals in a transaction)
  LET updated = REQUEST("POST", "/v1/invoices/" & id & "/items", {"description": "Go Binary Packaging", "quantity": 1, "unitPrice": "100.00"}, hdrs)
  EXPECT updated.status = 201
  EXPECT updated.body.subtotal = "300.00"
  EXPECT updated.body.tax = "22.50"
  EXPECT updated.body.total = "322.50"
  EXPECT LEN(updated.body.items) = 2

  ' List invoices filtered by customer and status
  LET listed = REQUEST("GET", "/v1/invoices?customerId=42&status=unpaid", NIL, hdrs)
  EXPECT listed.status = 200
  EXPECT listed.body.count >= 1

  ' Mark paid
  LET paid = REQUEST("POST", "/v1/invoices/" & id & "/pay", NIL, hdrs)
  EXPECT paid.status = 200
  EXPECT paid.body.status = "paid"

  ' Cannot pay twice or add items to a paid invoice
  LET payAgain = REQUEST("POST", "/v1/invoices/" & id & "/pay", NIL, hdrs)
  EXPECT payAgain.status = 409

  LET addAfterPay = REQUEST("POST", "/v1/invoices/" & id & "/items", {"description": "Late Fee", "quantity": 1, "unitPrice": "10.00"}, hdrs)
  EXPECT addAfterPay.status = 409

  ' Customer totals reflect paid invoice
  LET summary = REQUEST("GET", "/v1/customers/42/totals", NIL, hdrs)
  EXPECT summary.status = 200
  EXPECT summary.body.customerId = 42
  EXPECT summary.body.paidCount >= 1
  EXPECT summary.body.totalPaid = "322.50"
  EXPECT summary.body.totalOutstanding = "0.00"
END TEST
