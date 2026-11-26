' A JSON request body, parsed into req.body (Requirement 7.2).
ROUTE POST "/echo" CALL Echo
ROUTE POST "/summon" CALL Summon
SERVE

FUNCTION Echo(req)
  RETURN req.body
END FUNCTION

FUNCTION Summon(req)
  LET b = req.body
  RETURN {"name": b.name, "firstTag": b.tags[0], "tagCount": LEN(b.tags), "nextAge": b.age + 1, "undead": NOT b.alive}
END FUNCTION
