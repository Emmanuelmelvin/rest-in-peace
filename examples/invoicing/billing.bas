' billing.bas — pure business logic, easy to test.

EXPORT FUNCTION LineTotal(item) AS DECIMAL
  RETURN ROUND(DECIMAL(item.unitPrice) * item.quantity, 2)
END FUNCTION

EXPORT FUNCTION InvoiceTotals(items AS ARRAY, taxRate AS DECIMAL) AS MAP
  DIM subtotal AS DECIMAL = DECIMAL("0")
  FOR EACH item IN items
    IF item.quantity = NIL OR item.quantity <= 0 THEN THROW HTTPERROR(422, "QUANTITY MUST BE POSITIVE")
    subtotal = subtotal + LineTotal(item)
  NEXT
  LET tax = ROUND(subtotal * taxRate, 2)
  RETURN {"subtotal": subtotal, "tax": tax, "total": subtotal + tax}
END FUNCTION
