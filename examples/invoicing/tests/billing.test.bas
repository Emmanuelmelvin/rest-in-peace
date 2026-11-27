' billing.test.bas — unit tests for DECIMAL money math in billing.bas.
IMPORT { LineTotal, InvoiceTotals, CustomerSummary } FROM "../billing.bas"

TEST "COMPUTES LINE TOTAL WITH EXACT DECIMAL"
  LET amt = LineTotal({"unitPrice": "19.99", "quantity": 3})
  EXPECT amt = DECIMAL("59.97"), "expected 19.99 * 3 = 59.97"
END TEST

TEST "AVOIDS FLOATING POINT DRIFT"
  LET t = InvoiceTotals([{"unitPrice": "0.10", "quantity": 1}, {"unitPrice": "0.20", "quantity": 1}], DECIMAL("0"))
  EXPECT t.subtotal = DECIMAL("0.30"), "0.10 + 0.20 must equal exact 0.30"
  EXPECT t.total = DECIMAL("0.30")
END TEST

TEST "ADDS TAX AND ROUNDS CORRECTLY"
  LET t = InvoiceTotals([{"unitPrice": "100.00", "quantity": 2}], DECIMAL("0.075"))
  EXPECT t.subtotal = DECIMAL("200.00")
  EXPECT t.tax = DECIMAL("15.00")
  EXPECT t.total = DECIMAL("215.00")

  LET r = InvoiceTotals([{"unitPrice": "33.33", "quantity": 1}], DECIMAL("0.075"))
  EXPECT r.tax = DECIMAL("2.50"), "33.33 * 0.075 = 2.49975 rounds to 2.50"
  EXPECT r.total = DECIMAL("35.83")
END TEST

TEST "SUMMARIZES CUSTOMER TOTALS"
  LET s = CustomerSummary(1, [{"total": "215.00", "status": "paid"}, {"total": "107.50", "status": "unpaid"}])
  EXPECT s.customerId = 1
  EXPECT s.invoiceCount = 2
  EXPECT s.paidCount = 1
  EXPECT s.unpaidCount = 1
  EXPECT s.totalBilled = DECIMAL("322.50")
  EXPECT s.totalPaid = DECIMAL("215.00")
  EXPECT s.totalOutstanding = DECIMAL("107.50")
END TEST
