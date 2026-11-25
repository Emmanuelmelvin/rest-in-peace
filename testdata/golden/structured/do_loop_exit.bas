n = 1
DO
  PRINT n
  IF n = 3 THEN EXIT DO
  n = n + 1
LOOP
PRINT "AFTER"

k = 1
DO
  IF k = 2 THEN EXIT DO
  PRINT "K=" & k
  k = k + 1
LOOP UNTIL k > 5
PRINT "END " & k
