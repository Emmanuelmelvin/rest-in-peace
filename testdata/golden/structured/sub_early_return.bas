ShowPositive(5)
ShowPositive(-2)
ShowPositive(0)
PRINT FindFirst([3, 8, 12, 15], 10)
PRINT FindFirst([1, 2, 3], 10) = NIL

SUB ShowPositive(n)
  IF n <= 0 THEN RETURN
  PRINT n
END SUB

FUNCTION FindFirst(arr, limit)
  FOR EACH v IN arr
    IF v > limit THEN RETURN v
  NEXT
  RETURN NIL
END FUNCTION
