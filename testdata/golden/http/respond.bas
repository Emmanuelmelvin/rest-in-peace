' RESPOND status, value [, headers] (Requirement 7.4).
ROUTE POST "/ghosts" CALL CreateGhost
ROUTE GET "/accepted" CALL Accepted
ROUTE PUT "/early" CALL Early
ROUTE PATCH "/teapot" CALL Teapot
SERVE

FUNCTION CreateGhost(req)
  RESPOND 201, {"id": "1", "name": req.body.name}, {"location": "/ghosts/1", "x-ghost": "boo"}
END FUNCTION

FUNCTION Accepted(req)
  RESPOND 202, {"queued": TRUE}
END FUNCTION

FUNCTION Early(req)
  IF req.query.stop = "yes" THEN
    RESPOND 409, {"error": "STOPPED EARLY"}
  END IF
  RETURN {"stopped": FALSE}
END FUNCTION

FUNCTION Teapot(req)
  RESPOND 418, {"error": "I AM A TEAPOT"}
  RETURN {"error": "RESPOND DID NOT END THE HANDLER"}
END FUNCTION
