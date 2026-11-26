' RETURN and EXIT inside TRY and CATCH reach the function and the loop.
FUNCTION SafeDivide(a, b)
  TRY
    RETURN a / b
  CATCH e
    RETURN "CANNOT DIVIDE: " & e.kind
  FINALLY
    PRINT "CHECKED "; a; "/"; b
  END TRY
END FUNCTION

FUNCTION Half(n AS INTEGER) AS INTEGER
  TRY
    IF n < 0 THEN THROW ERROR("NO NEGATIVE SOULS")
    RETURN n \ 2
  CATCH e
    RETURN -1
  END TRY
END FUNCTION

PRINT SafeDivide(10, 4)
PRINT SafeDivide(1, 0)
PRINT Half(9)
PRINT Half(-9)

FOR i = 1 TO 10
  TRY
    IF i = 4 THEN EXIT FOR
    IF i = 2 THEN THROW ERROR("TWO IS CURSED")
    PRINT i
  CATCH e
    PRINT e.message
  END TRY
NEXT
PRINT "STOPPED AT "; i

tries = 0
WHILE TRUE
  tries = tries + 1
  TRY
    IF tries = 3 THEN THROW ERROR("GIVE UP")
  CATCH e
    EXIT WHILE
  FINALLY
    PRINT "TRY "; tries
  END TRY
WEND
PRINT "GAVE UP AFTER "; tries
