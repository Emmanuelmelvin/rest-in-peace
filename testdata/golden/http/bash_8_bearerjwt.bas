' Built-in BEARERJWT middleware rejects missing/malformed/invalid tokens with 401 (Requirement 8.3).
IMPORT os FROM GO "os"

os.Setenv("BASH_JWT_SECRET", "super-secret-key")

USE BEARERJWT("BASH_JWT_SECRET") ON "/jwt/*"

ROUTE GET "/jwt/me" CALL Me
ROUTE GET "/open" CALL Open
SERVE

FUNCTION Me(req)
  RETURN {"user": req.ctx.user}
END FUNCTION

FUNCTION Open(req)
  RETURN {"public": TRUE}
END FUNCTION
