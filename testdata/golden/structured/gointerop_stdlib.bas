IMPORT strings FROM GO "strings"
IMPORT url FROM GO "net/url"
IMPORT strconv FROM GO "strconv"
IMPORT math FROM GO "math"
IMPORT http FROM GO "net/http"

PRINT strings.ToUpper("rest in peace")
parts = strings.Split("a,b,c", ",")
PRINT strings.Join(parts, "-")
PRINT strconv.Atoi("42") + 8
PRINT INT(math.Sqrt(49))

u = NEW(url.URL)
u.Scheme = "https"
u.Host = "rip.dev"
u.Path = "/v1/hello"
u.RawQuery = "lang=basic"
PRINT u.String()

ref = u.ResolveReference({"path": "/v2/bye", "rawQuery": "ok=1"})
PRINT ref.String()

req = http.NewRequestWithContext("GET", "https://example.com/ping", NIL)
PRINT req.Method
PRINT req.URL.Host

TRY
  n = strconv.Atoi("not-a-number")
  PRINT n
CATCH e
  PRINT e.kind
END TRY
