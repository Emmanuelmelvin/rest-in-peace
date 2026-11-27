10 REM Classic mode SQL injection warning with inline concatenation (Requirement 9.4)
20 DBCONNECT "sqlite://:memory:"
30 ROUTE GET "/search" GOSUB 100
40 SERVE
100 LET R = SQL("SELECT * FROM items WHERE name = " + QUERY("q"))
110 RESPOND 200, R
