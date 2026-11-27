' REQUEST calls the program's routes in-process, and SERVE does not bind a
' port while tests run (Requirement 12.3).
ROUTE GET "/hello/:name" CALL Hello
ROUTE POST "/echo" CALL Echo
USE Guard ON "/secret/*"
ROUTE GET "/secret/thing" CALL Hello
SERVE

FUNCTION Hello(req)
  RETURN {"message": "HELLO " + req.params.name}
END FUNCTION

FUNCTION Echo(req)
  RESPOND 201, {"got": req.body, "key": req.headers["x-api-key"]}, {"X-Made-By": "rip"}
END FUNCTION

SUB Guard(req)
  IF req.headers["x-api-key"] <> "test-key" THEN RESPOND 401, {"error": "WHO GOES THERE"}
END SUB

TEST "GETS A ROUTE"
  LET r = REQUEST("GET", "/hello/ghost")
  EXPECT r.status = 200
  EXPECT r.body.message = "HELLO ghost"
  PRINT r.headers["content-type"]
END TEST

TEST "POSTS A BODY WITH HEADERS"
  LET r = REQUEST("POST", "/echo", {"items": [1, 2]}, {"x-api-key": "test-key"})
  EXPECT r.status = 201
  EXPECT r.headers["x-made-by"] = "rip"
  EXPECT r.body.got.items[1] = 2
  EXPECT r.body.key = "test-key"
END TEST

TEST "RUNS MIDDLEWARE"
  EXPECT REQUEST("GET", "/secret/thing").status = 401
  EXPECT REQUEST("GET", "/secret/thing", NIL, {"x-api-key": "test-key"}).status = 200
END TEST

TEST "MISSES A ROUTE"
  LET r = REQUEST("DELETE", "/nowhere")
  EXPECT r.status = 404
  EXPECT r.body.error = "ROUTE NOT FOUND IN THIS REALM"
END TEST
