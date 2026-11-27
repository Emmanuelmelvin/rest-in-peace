' Assigning a non-SHARED module-level variable inside a handler warns
' with ?SHARED STATE WARNING (Requirement 14.4).
hits = 0
cache = {}
SHARED DIM safeCount AS INTEGER = 0

ROUTE GET "/hit" CALL Hit
SERVE

FUNCTION Hit(req)
  hits = hits + 1
  cache["last"] = "ok"
  safeCount = safeCount + 1
  DIM localVar AS INTEGER = 1
  localVar = localVar + 1
  RETURN {"hits": hits, "safe": safeCount}
END FUNCTION
