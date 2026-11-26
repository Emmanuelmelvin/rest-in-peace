' Runtime type checks for AS <TYPE> on dynamic boundaries (Requirement 3.2).
DIM m AS MAP = {"k": 1}
DIM xs AS ARRAY = [10, 20]
DIM nothing AS NIL = NIL
PRINT m.k; " "; xs[1]; " "; nothing = NIL

' Assigning wrong dynamic types raises TYPE MISMATCH.
TRY
  m = [1, 2]
CATCH e
  PRINT "ASSIGN MAP: "; e.kind
END TRY

TRY
  xs = {"not": "array"}
CATCH e
  PRINT "ASSIGN ARRAY: "; e.kind
END TRY

TRY
  nothing = "nope"
CATCH e
  PRINT "ASSIGN NIL: "; e.kind
END TRY

' Function parameters and return values check AS MAP and AS ARRAY.
PRINT ReadKey({"name": "Casper"})
TRY
  PRINT ReadKey("not a map")
CATCH e
  PRINT "PARAM MAP: "; e.kind
END TRY

PRINT FirstItem(["ghost", "bat"])
TRY
  PRINT FirstItem({"a": 1})
CATCH e
  PRINT "PARAM ARRAY: "; e.kind
END TRY

TRY
  PRINT BadReturn(TRUE)
CATCH e
  PRINT "RETURN MAP: "; e.kind
END TRY
PRINT BadReturn(FALSE).ok

' Typed FOR EACH variable checks each element.
DIM item AS MAP
TRY
  FOR EACH item IN [{"id": 1}, "bad"]
    PRINT "ITEM "; item.id
  NEXT
CATCH e
  PRINT "FOREACH MAP: "; e.kind
END TRY

FUNCTION ReadKey(obj AS MAP) AS STRING
  RETURN obj.name
END FUNCTION

FUNCTION FirstItem(items AS ARRAY)
  RETURN items[0]
END FUNCTION

FUNCTION BadReturn(fail AS BOOLEAN) AS MAP
  IF fail THEN RETURN [1, 2, 3]
  RETURN {"ok": TRUE}
END FUNCTION
