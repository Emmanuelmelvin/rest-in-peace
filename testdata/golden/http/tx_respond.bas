' RESPOND inside TRANSACTION commits; THROW HTTPERROR rolls back (Requirements 6.7, 9.5).
DBCONNECT "sqlite://:memory:"
SQLEXEC("CREATE TABLE notes (id INTEGER PRIMARY KEY AUTOINCREMENT, text TEXT NOT NULL, amount NUMERIC(12,2) NOT NULL)")

ROUTE POST "/notes" CALL CreateNote
ROUTE POST "/notes/fail" CALL FailNote
ROUTE GET  "/notes" CALL ListNotes
SERVE

FUNCTION CreateNote(req)
  LET b = req.body
  TRANSACTION
    LET row = SQLONE("INSERT INTO notes (text, amount) VALUES ($1, $2) RETURNING id, text, amount", b.text, DECIMAL(b.amount))
    RESPOND 201, row
  END TRANSACTION
END FUNCTION

FUNCTION FailNote(req)
  LET b = req.body
  TRANSACTION
    SQLEXEC("INSERT INTO notes (text, amount) VALUES ($1, $2)", b.text, DECIMAL(b.amount))
    THROW HTTPERROR(422, "NOTE REJECTED")
  END TRANSACTION
END FUNCTION

FUNCTION ListNotes(req)
  RETURN SQL("SELECT id, text, amount FROM notes ORDER BY id")
END FUNCTION
