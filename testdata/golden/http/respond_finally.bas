' RESPOND inside TRY is not caught by CATCH, and runs FINALLY (Requirement 6.7).
' FINALLY counts its runs in a module-level map that a second route reads.
LET haunting = {"finallyRuns": 0}
ROUTE GET "/haunt" CALL Haunt
ROUTE GET "/cleanups" CALL CountCleanups
SERVE

FUNCTION Haunt(req)
  TRY
    RESPOND 202, {"status": "HAUNTING"}
  CATCH e
    RESPOND 500, {"error": "CATCH CAUGHT RESPOND"}
  FINALLY
    haunting.finallyRuns = haunting.finallyRuns + 1
  END TRY
  RETURN {"status": "RAN ON AFTER TRY"}
END FUNCTION

FUNCTION CountCleanups(req)
  RETURN {"cleanups": haunting.finallyRuns}
END FUNCTION
