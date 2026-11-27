' DECIMAL arithmetic, comparisons, NUMBER mixing, and ROUND half-away-from-zero (Requirement 3.2, 3.3, 3.7).
DIM price AS DECIMAL = DECIMAL("19.99")
DIM qty AS INTEGER = 3
LET subtotal = price * qty
PRINT subtotal
PRINT subtotal - DECIMAL("9.97")
PRINT DECIMAL("10.00") / DECIMAL("4")

' Mixing DECIMAL and NUMBER converts to DECIMAL
LET mixed = DECIMAL("10.00") + 2.5
PRINT mixed

' Comparisons
PRINT DECIMAL("1.00") = DECIMAL("1")
PRINT DECIMAL("1.50") <> DECIMAL("1.51")
PRINT DECIMAL("2.00") < DECIMAL("2.01")
PRINT DECIMAL("2.01") > DECIMAL("2.00")
PRINT DECIMAL("2.00") <= DECIMAL("2.00")
PRINT DECIMAL("2.00") >= DECIMAL("1.99")

' ROUND half-away-from-zero
PRINT ROUND(DECIMAL("2.5"), 0)
PRINT ROUND(DECIMAL("-2.5"), 0)
PRINT ROUND(DECIMAL("1.235"), 2)
PRINT ROUND(DECIMAL("-1.235"), 2)
