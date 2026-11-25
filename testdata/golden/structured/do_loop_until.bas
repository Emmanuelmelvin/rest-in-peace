n = 10
DO
  PRINT n
  n = n + 1
LOOP UNTIL n > 12
' The body runs once before the condition is checked.
DO
  PRINT "ONCE"
LOOP UNTIL TRUE
