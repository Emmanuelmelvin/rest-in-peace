' main.bas — URL Shortener API: short links, 302 redirects, click analytics, rate limiting.
IMPORT { ValidateURL, ValidateCode, GenerateCode } FROM "./links.bas"
IMPORT { NewString } FROM GO "github.com/google/uuid"

DBCONNECT ENV("DATABASE_URL", "sqlite://./url-shortener.db")

USE REQUESTLOG()
USE CORS({"origins": ENV("CORS_ORIGIN", "*")})
USE RATELIMIT(120)
USE APIKEY("API_KEY") ON "/api/*"

ROUTE GROUP "/api"
  ROUTE POST   "/links"       CALL CreateLink
  ROUTE GET    "/links"       CALL ListLinks
  ROUTE GET    "/links/:code" CALL GetLinkStats
  ROUTE DELETE "/links/:code" CALL DeleteLink
END GROUP

ROUTE GET "/r/:code" CALL RedirectLink

SERVE

FUNCTION CreateLink(req)
  LET b = req.body
  IF b = NIL THEN
    THROW HTTPERROR(400, "JSON BODY REQUIRED")
  END IF

  LET targetUrl = ValidateURL(b.url)
  LET code = ""

  IF b.code <> NIL AND b.code <> "" THEN
    code = ValidateCode(b.code)
    LET existing = SQLONE("SELECT code FROM links WHERE code = $1", code)
    IF existing <> NIL THEN
      THROW HTTPERROR(409, "SHORT CODE ALREADY EXISTS")
    END IF
  ELSE
    code = GenerateCode()
  END IF

  LET link = SQLONE("INSERT INTO links (code, target_url) VALUES ($1, $2) RETURNING code, target_url, clicks, created_at", code, targetUrl)
  link.shortPath = "/r/" & link.code

  LOG "INFO", "LINK CREATED", {"code": link.code, "targetUrl": link.target_url}
  RESPOND 201, link, {"location": "/r/" & link.code}
END FUNCTION

FUNCTION ListLinks(req)
  LET rows = SQL("SELECT code, target_url, clicks, created_at FROM links ORDER BY created_at DESC LIMIT 100")
  RETURN {"links": rows, "count": LEN(rows)}
END FUNCTION

FUNCTION GetLinkStats(req)
  LET code = req.params.code
  LET link = SQLONE("SELECT code, target_url, clicks, created_at FROM links WHERE code = $1", code)
  IF link = NIL THEN
    THROW HTTPERROR(404, "LINK NOT FOUND")
  END IF

  LET recent = SQL("SELECT id, user_agent, referer, clicked_at FROM clicks WHERE link_code = $1 ORDER BY clicked_at DESC LIMIT 20", code)
  link.shortPath = "/r/" & link.code
  link.recentClicks = recent
  RETURN link
END FUNCTION

FUNCTION DeleteLink(req)
  LET code = req.params.code
  LET link = SQLONE("SELECT code FROM links WHERE code = $1", code)
  IF link = NIL THEN
    THROW HTTPERROR(404, "LINK NOT FOUND")
  END IF

  TRANSACTION
    SQLEXEC("DELETE FROM clicks WHERE link_code = $1", code)
    SQLEXEC("DELETE FROM links WHERE code = $1", code)
  END TRANSACTION

  LOG "INFO", "LINK DELETED", {"code": code}
  RETURN {"deleted": TRUE, "code": code}
END FUNCTION

FUNCTION RedirectLink(req)
  LET code = req.params.code
  LET link = SQLONE("SELECT code, target_url FROM links WHERE code = $1", code)
  IF link = NIL THEN
    THROW HTTPERROR(404, "LINK NOT FOUND")
  END IF

  LET ua = ""
  IF req.headers["user-agent"] <> NIL THEN ua = req.headers["user-agent"]

  LET ref = ""
  IF req.headers["referer"] <> NIL THEN ref = req.headers["referer"]

  TRANSACTION
    SQLEXEC("UPDATE links SET clicks = clicks + 1 WHERE code = $1", code)
    SQLEXEC("INSERT INTO clicks (id, link_code, user_agent, referer) VALUES ($1, $2, $3, $4)", NewString(), code, ua, ref)
  END TRANSACTION

  RESPOND 302, {"redirect": link.target_url}, {"location": link.target_url}
END FUNCTION
