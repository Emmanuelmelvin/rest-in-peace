' The query string, in req.query (Requirement 7.2).
ROUTE GET "/search" CALL Search
SERVE

FUNCTION Search(req)
  RETURN {"q": req.query.q, "limit": req.query.limit, "missing": req.query.nope, "path": req.path}
END FUNCTION
