' A trailing * in a route path (Requirement 7.1).
ROUTE GET "/files/*" CALL ServeFile
SERVE

FUNCTION ServeFile(req)
  RETURN {"path": req.path}
END FUNCTION
