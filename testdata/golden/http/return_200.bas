' A structured handler's RETURN v responds 200 with v (Requirement 7.5).
ROUTE GET "/map" CALL GiveMap
ROUTE POST "/array" CALL GiveArray
ROUTE DELETE "/nested" CALL GiveNested
SERVE

FUNCTION GiveMap(req)
  LET m = {"b": 2}
  m.a = 1
  m.z = "last"
  RETURN m
END FUNCTION

FUNCTION GiveArray(req)
  RETURN [1, 2.5, "three", TRUE, NIL]
END FUNCTION

FUNCTION GiveNested(req)
  RETURN Build(req.method)
END FUNCTION

FUNCTION Build(m)
  RETURN {"method": m, "list": [{"x": 1}, []]}
END FUNCTION
