' Path parameters (Requirement 7.1, 7.2).
ROUTE GET "/graves/:id" CALL ShowGrave
ROUTE GET "/crypts/:crypt/graves/:grave" CALL ShowCryptGrave
SERVE

FUNCTION ShowGrave(req)
  RETURN {"id": req.params.id}
END FUNCTION

FUNCTION ShowCryptGrave(req)
  RETURN {"crypt": req.params.crypt, "grave": req.params.grave}
END FUNCTION
