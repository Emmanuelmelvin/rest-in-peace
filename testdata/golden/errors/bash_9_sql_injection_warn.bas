' SQL query built with & from req triggers ?INJECTION RISK WARNING (Requirement 9.4).
DBCONNECT "sqlite://:memory:"
ROUTE GET "/users/:id" CALL GetUser
SERVE

FUNCTION GetUser(req)
  LET rows = SQL("SELECT * FROM users WHERE id = " & req.params.id)
  RETURN rows
END FUNCTION
