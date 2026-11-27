' A failing EXPECT reports its file and line; the other tests still run,
' and the program exits non-zero (Requirement 12.2).
TEST "PASSES"
  EXPECT 1 + 1 = 2
END TEST

TEST "COMPARES"
  LET status = 404
  EXPECT status = 200
  PRINT "NEVER PRINTED"
END TEST

TEST "SAYS WHY"
  EXPECT LEN("RIP") > 5, "the name is too short"
END TEST

TEST "HITS A RUNTIME ERROR"
  LET ghost = NIL
  PRINT ghost.name
END TEST

TEST "STILL RUNS"
  EXPECT TRUE
END TEST
