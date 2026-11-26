' An app's own /_health route replaces the built-in one (Requirement 14.6).
ROUTE GET "/_health" CALL Health
SERVE

FUNCTION Health(req)
  RETURN {"status": "UNDEAD", "graves": 3}
END FUNCTION
