' Missing map key -> NIL, LEN(NIL) = 0, and short-circuit guarding NIL access (Requirement 3.5, 3.6, 3.8, 3.9).
PRINT LEN(NIL)
PRINT LEN("")
PRINT LEN([])
PRINT LEN({})

LET m = {"a": 1}
PRINT m.missing = NIL
PRINT m["missing"] = NIL

LET u = NIL
PRINT u <> NIL AND u.name = "Ada"
PRINT u = NIL OR u.name = "Ada"
