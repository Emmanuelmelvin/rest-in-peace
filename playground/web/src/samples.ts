export type Sample = { name: string; code: string };

// Requirement 15.6. The CLI versions live in /examples.
export const samples: Sample[] = [
  {
    name: "Hello World",
    code: `ROUTE GET "/hello" CALL Hello
SERVE

FUNCTION Hello(req)
  RETURN {"message": "HELLO WORLD"}
END FUNCTION
`,
  },
  {
    name: "Classic GOTO Todo",
    code: `10 REM CLASSIC GOTO TODO
20 TODOS = {}
30 NEXTID = 1
40 ROUTE GET "/todos" GOSUB 100
50 ROUTE POST "/todos" GOSUB 200
60 SERVE
100 RESPOND 200, TODOS
200 B = BODY()
210 IF B.title = NIL THEN RESPOND 400, {"error": "A TODO NEEDS A TITLE"}
220 ID = STR(NEXTID)
230 TODOS[ID] = {"id": ID, "title": B.title, "done": FALSE}
240 NEXTID = NEXTID + 1
250 RESPOND 201, TODOS[ID]
`,
  },
  {
    name: "Invoicing API",
    code: `' Single-file version of examples/invoicing for the playground.
DBCONNECT ENV("DATABASE_URL")
SQLEXEC("CREATE TABLE IF NOT EXISTS invoices (id INTEGER PRIMARY KEY, total NUMERIC NOT NULL)")

ROUTE POST "/invoices" CALL CreateInvoice
SERVE

FUNCTION CreateInvoice(req)
  DIM subtotal AS DECIMAL = DECIMAL("0")
  FOR EACH item IN req.body.items
    IF item.quantity = NIL OR item.quantity <= 0 THEN THROW HTTPERROR(422, "QUANTITY MUST BE POSITIVE")
    subtotal = subtotal + ROUND(DECIMAL(item.unitPrice) * item.quantity, 2)
  NEXT
  LET total = subtotal + ROUND(subtotal * DECIMAL("0.075"), 2)
  TRANSACTION
    LET inv = SQLONE("INSERT INTO invoices (total) VALUES ($1) RETURNING *", total)
  END TRANSACTION
  RESPOND 201, inv
END FUNCTION
`,
  },
  {
    name: "GOTO Hell",
    code: `10 REM GOTO HELL — THE LOOP GUARD DEMO
20 ROUTE GET "/hell" GOSUB 100
30 SERVE
100 PRINT "ABANDON ALL HOPE"
110 GOTO 110
`,
  },
];
