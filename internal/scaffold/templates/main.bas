' {{name}} — a REST in Peace service
' Run it:  rip dev     Test it:  rip test     Ship it:  rip build

ROUTE GET "/hello" CALL Hello
SERVE

FUNCTION Hello(req)
  RETURN {"message": "HELLO WORLD"}
END FUNCTION
