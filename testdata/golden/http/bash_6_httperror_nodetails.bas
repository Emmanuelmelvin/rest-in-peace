' HTTPERROR without details omits "details"; RESPOND in TRY is not caught by CATCH and runs FINALLY (Requirement 6.4, 6.7).
ROUTE GET "/nodetails" CALL NoDetails
ROUTE GET "/withdetails" CALL WithDetails
ROUTE GET "/respond-try" CALL RespondInTry
SERVE

FUNCTION NoDetails(req)
  THROW HTTPERROR(400, "BAD INPUT")
END FUNCTION

FUNCTION WithDetails(req)
  THROW HTTPERROR(422, "INVALID", {"field": "email"})
END FUNCTION

FUNCTION RespondInTry(req)
  LET ran = "no"
  TRY
    RESPOND 202, {"accepted": TRUE}
  CATCH e
    RESPOND 500, {"caught": TRUE}
  FINALLY
    ran = "yes"
  END TRY
  RETURN {"unreachable": ran}
END FUNCTION
