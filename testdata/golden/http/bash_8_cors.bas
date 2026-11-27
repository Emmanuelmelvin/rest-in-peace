' Built-in CORS middleware with custom options and OPTIONS preflight (Requirement 8.3).
USE CORS({"origins": "https://rip.dev"})
ROUTE GET "/ping" CALL Ping
SERVE

FUNCTION Ping(req)
  RETURN {"pong": TRUE}
END FUNCTION
