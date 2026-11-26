' req.body is {} when there is no body (Requirement 7.2).
ROUTE GET "/body" CALL ShowBody
ROUTE POST "/body" CALL ShowBody
SERVE

FUNCTION ShowBody(req)
  RETURN {"body": req.body, "name": req.body.name}
END FUNCTION
