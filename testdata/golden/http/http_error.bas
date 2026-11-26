' An HTTPERROR that escapes a handler sets the response (Requirement 6.2, 6.4).
ROUTE GET "/ghosts/:id" CALL GetGhost
ROUTE POST "/ghosts" CALL CreateGhost
SERVE

FUNCTION GetGhost(req)
  THROW HTTPERROR(404, "GHOST IS DEAD", {"id": req.params.id})
END FUNCTION

FUNCTION CreateGhost(req)
  Validate(req.body)
  RETURN {"created": TRUE}
END FUNCTION

SUB Validate(b)
  IF b.name = NIL THEN THROW HTTPERROR(422, "A GHOST NEEDS A NAME", {"missing": ["name"], "got": b})
END SUB
