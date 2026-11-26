10 REM CLASSIC REQUEST HELPERS: PARAM, QUERY, HEADER AND BODY
20 ROUTE POST "/graves/:id" GOSUB 100
30 ROUTE GET "/graves/:id" GOSUB 200
40 SERVE
100 B = BODY()
110 RESPOND 200, {"id": PARAM("id"), "sort": QUERY("sort"), "token": HEADER("x-token"), "name": B.name}
200 B = BODY()
210 RESPOND 200, {"id": PARAM("id"), "body": B}
