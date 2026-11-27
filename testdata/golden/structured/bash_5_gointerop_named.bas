' Named Go imports, NEW(pkg.Type) method calls, and (value, error) -> e.kind = "GO" (Requirement 5.4-5.7).
IMPORT { ToUpper, HasPrefix } FROM GO "strings"
IMPORT strings FROM GO "strings"
IMPORT { Atoi } FROM GO "strconv"

PRINT ToUpper("hello")
PRINT HasPrefix("rest-in-peace", "rest")

LET sb = NEW(strings.Builder)
sb.WriteString("RIP")
sb.WriteString(" ")
sb.WriteString("BASIC")
PRINT sb.String()

TRY
  LET n = Atoi("nope")
  PRINT n
CATCH e
  PRINT e.kind
  PRINT LEN(e.message) > 0
END TRY
