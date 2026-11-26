' A body that is not valid JSON never reaches the handler (Requirement 7.9).
ROUTE POST "/graves" CALL Dig
SERVE

FUNCTION Dig(req)
  RESPOND 201, {"dug": TRUE}
END FUNCTION
