' api.test.bas — HTTP integration tests for the URL Shortener API.

TEST "HEALTH CHECK IS PUBLIC"
  LET r = REQUEST("GET", "/_health")
  EXPECT r.status = 200
  EXPECT r.body.status = "ALIVE"
END TEST

TEST "API ROUTES REQUIRE API KEY"
  LET r = REQUEST("POST", "/api/links", {"url": "https://example.com"})
  EXPECT r.status = 401
END TEST

TEST "REJECTS INVALID URL"
  LET hdrs = {"x-api-key": "test-key"}
  LET r = REQUEST("POST", "/api/links", {"url": "ftp://example.com"}, hdrs)
  EXPECT r.status = 422
END TEST

TEST "FULL SHORTEN, REDIRECT, ANALYTICS, AND DELETE FLOW"
  LET hdrs = {"x-api-key": "test-key"}

  ' Create a link with auto-generated code
  LET created = REQUEST("POST", "/api/links", {"url": "https://example.com/docs"}, hdrs)
  EXPECT created.status = 201
  EXPECT created.body.target_url = "https://example.com/docs"
  EXPECT created.body.clicks = 0
  LET code = created.body.code
  EXPECT LEN(code) = 7

  ' Public redirect works without API key and records clicks
  LET redir1 = REQUEST("GET", "/r/" & code, NIL, {"user-agent": "rip-test/1.0", "referer": "https://news.ycombinator.com"})
  EXPECT redir1.status = 302
  EXPECT redir1.headers["location"] = "https://example.com/docs"

  LET redir2 = REQUEST("GET", "/r/" & code)
  EXPECT redir2.status = 302

  ' Analytics reflect both clicks
  LET stats = REQUEST("GET", "/api/links/" & code, NIL, hdrs)
  EXPECT stats.status = 200
  EXPECT stats.body.clicks = 2
  EXPECT LEN(stats.body.recentClicks) = 2

  ' List links includes our link
  LET listed = REQUEST("GET", "/api/links", NIL, hdrs)
  EXPECT listed.status = 200
  EXPECT listed.body.count >= 1

  ' Delete link and confirm 404 afterwards
  LET deleted = REQUEST("DELETE", "/api/links/" & code, NIL, hdrs)
  EXPECT deleted.status = 200
  EXPECT deleted.body.deleted = TRUE

  LET missing = REQUEST("GET", "/r/" & code)
  EXPECT missing.status = 404
END TEST

TEST "CUSTOM SLUG AND DUPLICATE CONFLICT"
  LET hdrs = {"x-api-key": "test-key"}

  LET first = REQUEST("POST", "/api/links", {"url": "https://rip.dev", "code": "rip-home"}, hdrs)
  EXPECT first.status = 201
  EXPECT first.body.code = "rip-home"

  LET dup = REQUEST("POST", "/api/links", {"url": "https://other.dev", "code": "rip-home"}, hdrs)
  EXPECT dup.status = 409
END TEST
