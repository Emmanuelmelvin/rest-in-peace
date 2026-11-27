' Recursion beyond the depth limit raises ?STACK OVERFLOW ERROR (Requirement 4.5).
FUNCTION Infinite(n)
  RETURN Infinite(n + 1)
END FUNCTION

PRINT Infinite(0)
