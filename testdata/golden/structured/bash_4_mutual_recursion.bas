' Mutual recursion, forward calls, typed default params, and local scope isolation (Requirement 4.1-4.5).
LET x = 100
PRINT IsEven(6)
PRINT IsEven(7)
PRINT Greet("Ada")
PRINT Greet("Grace", "Admiral")
Shadow()
PRINT x

FUNCTION IsEven(n AS INTEGER) AS BOOLEAN
  IF n = 0 THEN RETURN TRUE
  RETURN IsOdd(n - 1)
END FUNCTION

FUNCTION IsOdd(n AS INTEGER) AS BOOLEAN
  IF n = 0 THEN RETURN FALSE
  RETURN IsEven(n - 1)
END FUNCTION

FUNCTION Greet(name AS STRING, title AS STRING = "Dr.") AS STRING
  RETURN title & " " & name
END FUNCTION

SUB Shadow()
  LET x = 999
  PRINT x
END SUB
