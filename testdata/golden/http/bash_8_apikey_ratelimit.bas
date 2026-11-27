' Built-in APIKEY and RATELIMIT middleware with path scoping (Requirement 8.1-8.3).
IMPORT os FROM GO "os"

os.Setenv("BASH_TEST_API_KEY", "s3cr3t")

USE APIKEY("BASH_TEST_API_KEY") ON "/secret/*"
USE RATELIMIT(2) ON "/limited/*"

ROUTE GET "/secret/data" CALL SecretData
ROUTE GET "/limited/ping" CALL LimitedPing
SERVE

FUNCTION SecretData(req)
  RETURN {"classified": TRUE}
END FUNCTION

FUNCTION LimitedPing(req)
  RETURN {"ok": TRUE}
END FUNCTION
