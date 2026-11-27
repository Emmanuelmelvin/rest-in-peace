' Caught error exposes message, kind, file, line, and status for HTTPERROR (Requirement 6.1-6.3).
TRY
  THROW ERROR("BROKEN SPELL")
CATCH e
  PRINT e.kind
  PRINT e.message
  PRINT e.file
  PRINT e.line
FINALLY
  PRINT "finally-1"
END TRY

TRY
  THROW HTTPERROR(418, "IM A TEAPOT", {"cup": 1})
CATCH e
  PRINT e.kind
  PRINT e.status
  PRINT e.message
  PRINT e.file
  PRINT e.line
FINALLY
  PRINT "finally-2"
END TRY
