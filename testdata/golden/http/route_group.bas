' ROUTE GROUP prefixes every route inside it (Requirement 7.7).
ROUTE GROUP "/v1"
  ROUTE GET "/ping" CALL Ping
  ROUTE GET "/ghosts/:id" CALL GetGhost
END GROUP
SERVE

FUNCTION Ping(req)
  RETURN {"pong": TRUE}
END FUNCTION

FUNCTION GetGhost(req)
  RETURN {"id": req.params.id}
END FUNCTION
