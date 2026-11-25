FUNCTION Label(text, prefix = "*", suffix = "")
  RETURN prefix & text & suffix
END FUNCTION

PRINT Label("A")
PRINT Label("B", "#")
PRINT Label("C", "[", "]")
