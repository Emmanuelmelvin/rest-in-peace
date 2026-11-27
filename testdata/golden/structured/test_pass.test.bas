' TEST blocks run after the top-level code, one at a time (Requirement 12).
FUNCTION Square(n AS INTEGER) AS INTEGER
  RETURN n * n
END FUNCTION

LET total = 0
PRINT "TOP LEVEL RUNS FIRST"

TEST "SQUARES NUMBERS"
  EXPECT Square(3) = 9
  EXPECT Square(-2) = 4, "negative numbers square to positive ones"
END TEST

TEST "HAS ITS OWN VARIABLES"
  LET total = 100
  DIM count AS INTEGER = 0
  FOR i = 1 TO 3
    count = count + i
  NEXT
  EXPECT count = 6
  PRINT "LOCAL TOTAL "; total
END TEST

TEST "READS MODULE VARIABLES"
  EXPECT total = 0
  EXPECT LEN([1, 2, 3]) > 2
END TEST

TEST "CATCHES ERRORS"
  TRY
    THROW ERROR("BOO")
  CATCH e
    EXPECT e.message = "BOO"
  END TRY
END TEST
