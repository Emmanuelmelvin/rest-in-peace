' Any other error that escapes a handler is a 500 that hides the details
' (Requirement 6.5).
ROUTE GET "/flood" CALL Flood
ROUTE GET "/divide" CALL Divide
ROUTE GET "/nil" CALL NilMember
ROUTE GET "/ok" CALL Ok
SERVE

FUNCTION Flood(req)
  THROW ERROR("THE CRYPT IS FLOODED")
END FUNCTION

FUNCTION Divide(req)
  LET zero = LEN(req.query.nothing)
  RETURN {"result": 10 / zero}
END FUNCTION

FUNCTION NilMember(req)
  RETURN {"name": req.body.ghost.name}
END FUNCTION

FUNCTION Ok(req)
  RETURN {"ok": TRUE}
END FUNCTION
