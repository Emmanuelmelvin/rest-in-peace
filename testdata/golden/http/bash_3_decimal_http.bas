' DECIMAL serializes as a scale-preserving JSON string and maps keep insertion order (Requirement 3.5, 3.7).
ROUTE GET "/invoice" CALL GetInvoice
SERVE

FUNCTION GetInvoice(req)
  LET subtotal = DECIMAL("200.00")
  LET tax = DECIMAL("15.00")
  RETURN {"z_id": "inv-1", "a_subtotal": subtotal, "m_total": subtotal + tax}
END FUNCTION
