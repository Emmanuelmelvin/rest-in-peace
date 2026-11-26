' USE middleware: order, req.ctx, path scoping, and short-circuiting
' with RESPOND or THROW (Requirement 8.1, 8.2).
USE TraceFirst
USE TraceSecond
USE GuardAdmin ON "/admin/*"

ROUTE GET "/public" CALL Public
ROUTE GET "/admin/vault" CALL Vault
SERVE

SUB TraceFirst(req)
  req.ctx.order = ["first"]
END SUB

SUB TraceSecond(req)
  req.ctx.order = [req.ctx.order[0], "second"]
END SUB

SUB GuardAdmin(req)
  IF req.headers["x-pass"] = "deny" THEN
    RESPOND 403, {"error": "FORBIDDEN BY THE CRYPT"}
  END IF
  IF req.headers["x-pass"] = "throw" THEN
    THROW HTTPERROR(401, "NO PASS PROVIDED")
  END IF
  req.ctx.admin = TRUE
END SUB

FUNCTION Public(req)
  RETURN {"order": req.ctx.order, "admin": req.ctx.admin}
END FUNCTION

FUNCTION Vault(req)
  RETURN {"order": req.ctx.order, "admin": req.ctx.admin}
END FUNCTION
