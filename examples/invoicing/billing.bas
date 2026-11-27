' billing.bas — pure money math and invoice calculations (no DB, no HTTP).

EXPORT FUNCTION LineTotal(item) AS DECIMAL
  IF item.quantity = NIL OR item.quantity <= 0 THEN
    THROW HTTPERROR(422, "QUANTITY MUST BE POSITIVE")
  END IF
  IF item.unitPrice = NIL THEN
    THROW HTTPERROR(422, "UNIT PRICE REQUIRED")
  END IF
  DIM price AS DECIMAL = DECIMAL(item.unitPrice)
  IF price < DECIMAL("0") THEN
    THROW HTTPERROR(422, "UNIT PRICE CANNOT BE NEGATIVE")
  END IF
  RETURN ROUND(price * item.quantity, 2)
END FUNCTION

EXPORT FUNCTION InvoiceTotals(items AS ARRAY, taxRate AS DECIMAL) AS MAP
  IF LEN(items) = 0 THEN
    THROW HTTPERROR(422, "AT LEAST ONE ITEM REQUIRED")
  END IF
  DIM subtotal AS DECIMAL = DECIMAL("0.00")
  FOR EACH item IN items
    subtotal = subtotal + LineTotal(item)
  NEXT
  LET tax = ROUND(subtotal * taxRate, 2)
  RETURN {"subtotal": subtotal, "tax": tax, "total": subtotal + tax}
END FUNCTION

EXPORT FUNCTION CustomerSummary(customerId, invoices AS ARRAY) AS MAP
  DIM totalBilled AS DECIMAL = DECIMAL("0.00")
  DIM totalPaid AS DECIMAL = DECIMAL("0.00")
  DIM paidCount AS INTEGER = 0
  DIM unpaidCount AS INTEGER = 0
  FOR EACH inv IN invoices
    DIM amt AS DECIMAL = DECIMAL(inv.total)
    totalBilled = totalBilled + amt
    IF inv.status = "paid" THEN
      totalPaid = totalPaid + amt
      paidCount = paidCount + 1
    ELSE
      unpaidCount = unpaidCount + 1
    END IF
  NEXT
  RETURN {"customerId": customerId, "invoiceCount": LEN(invoices), "paidCount": paidCount, "unpaidCount": unpaidCount, "totalBilled": totalBilled, "totalPaid": totalPaid, "totalOutstanding": totalBilled - totalPaid}
END FUNCTION
