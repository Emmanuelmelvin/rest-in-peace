' Outbound HTTP via FETCH (GET, POST JSON body, headers, timeout) and goroutine concurrency (Requirement 5.8, 11.1).
ROUTE POST "/echo" CALL Echo
ROUTE GET "/proxy" CALL Proxy
SERVE

FUNCTION Echo(req)
  RESPOND 200, {"seen": req.body.hello, "from": req.headers["x-from"]}, {"X-Echo": "ok"}
END FUNCTION

FUNCTION Proxy(req)
  LET url = "http://127.0.0.1:" & ENV("PORT") & "/echo"
  LET res = FETCH(url, {"method": "POST", "headers": {"X-From": "fetch"}, "body": {"hello": "world"}, "timeout": 5})
  RETURN {"status": res.status, "echoHeader": res.headers["x-echo"], "body": res.body}
END FUNCTION
