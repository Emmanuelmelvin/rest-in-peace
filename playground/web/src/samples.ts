// Requirement 15.6 — playground samples.
// Every sample here has been verified with `go run ./cmd/rip check`.
// The CLI versions of longer programs live in /examples.
export type Sample = { name: string; code: string };

export const samples: Sample[] = [
  // ── 1. Hello World ──────────────────────────────────────────────────────
  {
    name: "Hello World",
    code: `ROUTE GET "/hello" CALL Hello
SERVE

FUNCTION Hello(req)
  RETURN {"message": "HELLO, WORLD FROM RIP BASIC"}
END FUNCTION
`,
  },

  // ── 2. Classic GOTO Todo ─────────────────────────────────────────────────
  {
    name: "Classic GOTO Todo",
    code: `10 REM CLASSIC GOTO TODO — LINE NUMBERS LIVE AGAIN
20 TODOS = {}
30 NEXTID = 1
40 ROUTE GET "/todos" GOSUB 100
50 ROUTE POST "/todos" GOSUB 200
60 ROUTE PATCH "/todos/:id" GOSUB 300
70 SERVE
100 RESPOND 200, TODOS
200 B = BODY()
210 IF B.title = NIL THEN RESPOND 400, {"error": "A TODO NEEDS A TITLE"}
220 ID = STR(NEXTID)
230 TODOS[ID] = {"id": ID, "title": B.title, "done": FALSE}
240 NEXTID = NEXTID + 1
250 RESPOND 201, TODOS[ID]
300 ID = PARAM("id")
310 IF TODOS[ID] = NIL THEN GOTO 400
320 TODOS[ID].done = TRUE
330 RESPOND 200, TODOS[ID]
400 RESPOND 404, {"error": "THAT TODO IS DEAD"}
`,
  },

  // ── 3. Invoicing API ─────────────────────────────────────────────────────
  {
    name: "Invoicing API",
    code: `' Single-file version of examples/invoicing for the playground.
' POST /invoices {"items": [{"description": "Coffin", "unitPrice": "99.95", "quantity": 2}]}
DBCONNECT ENV("DATABASE_URL")
SQLEXEC("CREATE TABLE IF NOT EXISTS invoices (id INTEGER PRIMARY KEY, subtotal TEXT NOT NULL, tax TEXT NOT NULL, total TEXT NOT NULL)")

ROUTE POST "/invoices"     CALL CreateInvoice
ROUTE GET  "/invoices/:id" CALL GetInvoice
SERVE

FUNCTION CreateInvoice(req)
  IF req.body.items = NIL OR LEN(req.body.items) = 0 THEN
    THROW HTTPERROR(400, "ITEMS ARE REQUIRED")
  END IF

  DIM subtotal AS DECIMAL = DECIMAL("0")
  FOR EACH item IN req.body.items
    IF item.quantity = NIL OR item.quantity <= 0 THEN
      THROW HTTPERROR(422, "QUANTITY MUST BE POSITIVE")
    END IF
    IF item.unitPrice = NIL THEN
      THROW HTTPERROR(422, "UNIT PRICE IS REQUIRED")
    END IF
    subtotal = subtotal + ROUND(DECIMAL(item.unitPrice) * item.quantity, 2)
  NEXT

  DIM tax   AS DECIMAL = ROUND(subtotal * DECIMAL("0.075"), 2)
  DIM total AS DECIMAL = subtotal + tax

  TRANSACTION
    LET inv = SQLONE("INSERT INTO invoices (subtotal, tax, total) VALUES ($1, $2, $3) RETURNING *", subtotal, tax, total)
  END TRANSACTION

  LOG "INFO", "INVOICE CREATED", {"invoiceId": inv.id, "total": total}
  RESPOND 201, inv
END FUNCTION

FUNCTION GetInvoice(req)
  LET inv = SQLONE("SELECT * FROM invoices WHERE id = $1", req.params.id)
  IF inv = NIL THEN THROW HTTPERROR(404, "INVOICE IS DEAD")
  RETURN inv
END FUNCTION
`,
  },

  // ── 4. GOTO Hell ─────────────────────────────────────────────────────────
  {
    name: "GOTO Hell",
    code: `10 REM GOTO HELL — THE LOOP GUARD DEMO
20 REM HIT /hell AND WATCH THE WATCHDOG RETURN 508. /heaven STAYS ALIVE.
30 ROUTE GET "/hell" GOSUB 100
40 ROUTE GET "/heaven" GOSUB 200
50 SERVE
100 PRINT "ABANDON ALL HOPE, YE WHO ENTER HERE"
110 GOTO 110
200 RESPOND 200, {"message": "STILL ALIVE"}
`,
  },

  // ── 5. JSON Routes with :params ──────────────────────────────────────────
  {
    name: "Routes & Params",
    code: `ROUTE GET "/greet/:name" CALL Greet
ROUTE GET "/items/:id" CALL GetItem
ROUTE POST "/items" CALL CreateItem
SERVE

FUNCTION Greet(req)
  RETURN {"message": "GREETINGS, " & UPPER(req.params.name) & "!"}
END FUNCTION

FUNCTION GetItem(req)
  DIM id AS STRING = req.params.id
  IF id = "42" THEN
    RETURN {"id": 42, "name": "THE ANSWER", "category": "COSMIC"}
  END IF
  THROW HTTPERROR(404, "ITEM NOT FOUND IN THIS REALM")
END FUNCTION

FUNCTION CreateItem(req)
  IF req.body.name = NIL OR LEN(TRIM(req.body.name)) = 0 THEN
    THROW HTTPERROR(400, "NAME IS REQUIRED")
  END IF
  RETURN {"id": "new", "name": req.body.name}
END FUNCTION
`,
  },

  // ── 6. TRY/CATCH + HTTPERROR ─────────────────────────────────────────────
  {
    name: "TRY/CATCH",
    code: `ROUTE POST "/divide" CALL Divide
ROUTE GET "/explode" CALL Explode
SERVE

FUNCTION Divide(req)
  DIM a AS NUMBER = req.body.a
  DIM b AS NUMBER = req.body.b
  TRY
    IF b = 0 THEN
      THROW HTTPERROR(422, "DIVISION BY ZERO IS FORBIDDEN IN THIS REALM")
    END IF
    RETURN {"result": a / b}
  CATCH e
    LOG "WARN", "DIVIDE FAILED", {"a": a, "b": b, "err": e.message}
    THROW e
  END TRY
END FUNCTION

FUNCTION Explode(req)
  ' This error escapes the handler → 500 with a request ID, details go to the log.
  THROW ERROR("?INTERNAL ERROR — SOMETHING WICKED THIS WAY COMES")
END FUNCTION
`,
  },

  // ── 7. Middleware: CORS + APIKEY ──────────────────────────────────────────
  {
    name: "Middleware",
    code: `' CORS on every route; API key guard on /admin/*.
' Set the API_KEY secret in the Secrets panel (or via ENV in the CLI).
USE CORS({"origins": "*"})
USE REQUESTLOG()
USE APIKEY("API_KEY") ON "/admin/*"

ROUTE GET "/ping" CALL Ping
ROUTE GET "/admin/status" CALL AdminStatus
SERVE

FUNCTION Ping(req)
  RETURN {"status": "ALIVE"}
END FUNCTION

FUNCTION AdminStatus(req)
  ' req.ctx.user is set by BEARERJWT; here we just show the ctx map.
  RETURN {"status": "OPERATIONAL", "ctx": req.ctx}
END FUNCTION
`,
  },

  // ── 8. ROUTE GROUP ────────────────────────────────────────────────────────
  {
    name: "Route Group",
    code: `' Group routes under a common prefix.
ROUTE GROUP "/api/v1"
  ROUTE GET  "/users"     CALL ListUsers
  ROUTE GET  "/users/:id" CALL GetUser
  ROUTE POST "/users"     CALL CreateUser
END GROUP

SERVE

FUNCTION ListUsers(req)
  RETURN {"users": [{"id": "1", "name": "LAZARUS", "status": "RISEN"}]}
END FUNCTION

FUNCTION GetUser(req)
  DIM id AS STRING = req.params.id
  IF id <> "1" THEN
    THROW HTTPERROR(404, "USER NOT FOUND IN THIS REALM")
  END IF
  RETURN {"id": id, "name": "LAZARUS", "status": "RISEN"}
END FUNCTION

FUNCTION CreateUser(req)
  IF req.body.name = NIL THEN
    THROW HTTPERROR(400, "NAME IS REQUIRED")
  END IF
  RETURN {"id": "2", "name": req.body.name, "status": "NEWLY UNDEAD"}
END FUNCTION
`,
  },
];
