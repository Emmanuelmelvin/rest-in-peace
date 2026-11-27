' SHARED DIM variables work across top-level code and functions (Requirement 14.4).
SHARED DIM counter AS INTEGER = 10
SHARED DIM label AS STRING = "HI"
SHARED DIM cache AS MAP

SUB Bump(delta AS INTEGER)
  counter = counter + delta
  label = label & "!"
END SUB

FUNCTION Shadow(counter AS INTEGER) AS INTEGER
  counter = counter + 100
  RETURN counter
END FUNCTION

Bump(5)
Bump(2)
cache["a"] = "ONE"
cache.b = "TWO"

PRINT counter
PRINT label
PRINT cache["a"]
PRINT cache.b
PRINT Shadow(1)
PRINT counter
