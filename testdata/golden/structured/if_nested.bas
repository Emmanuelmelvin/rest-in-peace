FUNCTION Classify(n)
  IF n > 0 THEN
    IF n > 10 THEN
      RETURN "BIG"
    ELSE
      RETURN "SMALL"
    END IF
  ELSEIF n = 0 THEN
    RETURN "ZERO"
  END IF
  RETURN "NEG"
END FUNCTION

PRINT Classify(20)
PRINT Classify(5)
PRINT Classify(0)
PRINT Classify(-3)
