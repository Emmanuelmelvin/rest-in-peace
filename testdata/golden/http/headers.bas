' Request headers have lowercase keys (Requirement 7.2), and map keys are
' case-sensitive (Requirement 1.5).
ROUTE GET "/whoami" CALL WhoAmI
SERVE

FUNCTION WhoAmI(req)
  RETURN {"ghost": req.headers["x-ghost-name"], "asSent": req.headers["X-Ghost-Name"], "token": req.headers["x-token"]}
END FUNCTION
