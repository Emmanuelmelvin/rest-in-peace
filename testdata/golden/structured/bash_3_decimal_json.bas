' DECIMAL serializes to a scale-preserving string in JSON (Requirement 3.7, Decision).
d1 = DECIMAL("19.99")
d2 = DECIMAL("200.00")
d3 = DECIMAL("0.1") + DECIMAL("0.2")
PRINT d1
PRINT d2
PRINT d3
LET sum = d1 + d2
PRINT sum
LET rounded = ROUND(d3, 1)
PRINT rounded
