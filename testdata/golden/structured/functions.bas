PRINT Double(21)
PRINT Greet("Ada")

FUNCTION Double(n AS INTEGER) AS INTEGER
  RETURN n * 2
END FUNCTION

FUNCTION Greet(name, greeting = "HELLO")
  RETURN greeting & ", " & name
END FUNCTION
