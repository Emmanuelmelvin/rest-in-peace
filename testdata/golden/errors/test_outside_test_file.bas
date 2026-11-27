' TEST blocks are only allowed in *.test.bas files (Requirement 12.1).
PRINT "HELLO"

TEST "IN THE WRONG FILE"
  EXPECT TRUE
END TEST
