' Built-in /_health with global REQUESTLOG middleware and LOG redaction in handler (Requirement 8.3, 10.3, 10.5, 14.6).
USE REQUESTLOG()
ROUTE GET "/work" CALL Work
SERVE

FUNCTION Work(req)
  LOG "INFO", "processing", {"user": "ada", "password": "secret-password", "apiKey": "k-123"}
  RETURN {"ok": TRUE}
END FUNCTION
