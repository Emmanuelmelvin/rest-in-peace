' Assigning a wrong dynamic type to a typed DIM raises ?TYPE MISMATCH ERROR (Requirement 3.2).
DIM count AS INTEGER = 5
LET bad = "not-an-int"
count = bad
PRINT count
