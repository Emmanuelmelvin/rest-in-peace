' All 5 HTTP methods, custom response headers, req.id, and unmatched method -> 404 realm message (Requirement 7.1, 7.2, 7.4, 7.8).
ROUTE GET "/items/:id" CALL GetItem
ROUTE POST "/items" CALL CreateItem
ROUTE PUT "/items/:id" CALL ReplaceItem
ROUTE PATCH "/items/:id" CALL PatchItem
ROUTE DELETE "/items/:id" CALL DeleteItem
ROUTE GET "/readonly" CALL ReadOnly
SERVE

FUNCTION GetItem(req)
  RETURN {"method": req.method, "id": req.params.id, "reqId": req.id}
END FUNCTION

FUNCTION CreateItem(req)
  RESPOND 201, {"method": req.method, "name": req.body.name}, {"X-Created": "true"}
END FUNCTION

FUNCTION ReplaceItem(req)
  RETURN {"method": req.method, "id": req.params.id, "name": req.body.name}
END FUNCTION

FUNCTION PatchItem(req)
  RETURN {"method": req.method, "id": req.params.id, "name": req.body.name}
END FUNCTION

FUNCTION DeleteItem(req)
  RESPOND 200, {"method": req.method, "id": req.params.id}
END FUNCTION

FUNCTION ReadOnly(req)
  RETURN {"ok": TRUE}
END FUNCTION
