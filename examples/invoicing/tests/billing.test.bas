IMPORT { InvoiceTotals } FROM "../billing.bas"

TEST "ADDS TAX CORRECTLY"
  LET t = InvoiceTotals([{"unitPrice": "100.00", "quantity": 2}], DECIMAL("0.075"))
  EXPECT t.total = DECIMAL("215.00")
END TEST

TEST "REJECTS MISSING API KEY"
  LET r = REQUEST("POST", "/v1/invoices", {"customerId": 1, "items": []})
  EXPECT r.status = 401
END TEST

TEST "REJECTS NEGATIVE QUANTITY"
  LET r = REQUEST("POST", "/v1/invoices", {"customerId": 1, "items": [{"description": "Coffin", "quantity": -1, "unitPrice": "50.00"}]}, {"x-api-key": "test-key"})
  EXPECT r.status = 422
END TEST
