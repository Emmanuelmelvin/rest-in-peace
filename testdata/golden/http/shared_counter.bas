' SHARED DIM state persisted across HTTP requests without ?SHARED STATE WARNING.
SHARED DIM hits AS INTEGER = 0
SHARED DIM notes AS MAP

ROUTE POST "/hit" CALL Hit
ROUTE GET "/hit" CALL GetHits
ROUTE POST "/note/:key" CALL SetNote
ROUTE GET "/note/:key" CALL GetNote
SERVE

FUNCTION Hit(req)
  hits = hits + 1
  RETURN {"hits": hits}
END FUNCTION

FUNCTION GetHits(req)
  RETURN {"hits": hits}
END FUNCTION

FUNCTION SetNote(req)
  notes[req.params.key] = req.body.value
  RETURN {"ok": TRUE}
END FUNCTION

FUNCTION GetNote(req)
  RETURN {"value": notes[req.params.key]}
END FUNCTION
