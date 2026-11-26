' Runtime errors, including Go panics, are caught with their kind and line.
zero = 0
DIM n AS INTEGER = 0
ghost = NIL
items = [1, 2, 3]

TRY
  PRINT 10 / zero
CATCH e
  PRINT e.kind; " AT LINE "; e.line
END TRY

TRY
  PRINT 7 \ n
CATCH e
  PRINT e.kind; " AT LINE "; e.line
END TRY

TRY
  PRINT ghost.name
CATCH e
  PRINT e.kind; " AT LINE "; e.line
END TRY

TRY
  PRINT items[3]
CATCH e
  PRINT e.kind; " AT LINE "; e.line
END TRY

TRY
  PRINT Haunt()
CATCH e
  PRINT e.kind; " AT LINE "; e.line
END TRY

FUNCTION Haunt()
  RETURN ghost.address.city
END FUNCTION
