' Nested TRY: HTTPERROR fields, rethrowing e, and the order FINALLY runs in.
FUNCTION LoadInvoice(id)
  TRY
    THROW HTTPERROR(404, "INVOICE IS DEAD", {"id": id})
  CATCH e
    PRINT "LOAD FAILED WITH "; e.status
    THROW e
  FINALLY
    PRINT "INNER FINALLY"
  END TRY
END FUNCTION

TRY
  LoadInvoice(7)
CATCH err
  PRINT err.kind; " "; err.status; " "; err.message
  PRINT "INVOICE "; err.details.id; " FROM LINE "; err.line
FINALLY
  PRINT "OUTER FINALLY"
END TRY

TRY
  TRY
    THROW ERROR("disk full")
  CATCH e
    e.message = "saving invoice: " & e.message
    THROW e
  END TRY
CATCH e
  PRINT e.message; " (LINE "; e.line; ")"
END TRY
