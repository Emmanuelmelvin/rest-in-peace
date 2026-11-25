TEST "SAYS HELLO"
  LET r = REQUEST("GET", "/hello")
  EXPECT r.status = 200
  EXPECT r.body.message = "HELLO WORLD"
END TEST
