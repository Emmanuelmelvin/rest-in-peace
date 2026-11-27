' HTTP + SQLite :memory: with NUMERIC -> DECIMAL JSON and TRANSACTION commit/rollback (Requirement 9.1-9.5, 9.7).
DBCONNECT "sqlite://:memory:"
SQLEXEC("CREATE TABLE ledger (id INTEGER PRIMARY KEY, label TEXT NOT NULL, amount NUMERIC(12,2) NOT NULL)")

ROUTE POST "/ledger" CALL AddEntry
ROUTE POST "/ledger/fail" CALL FailEntry
ROUTE GET "/ledger" CALL ListEntries
SERVE

FUNCTION AddEntry(req)
  TRANSACTION
    SQLEXEC("INSERT INTO ledger (id, label, amount) VALUES ($1, $2, $3)", req.body.id, req.body.label, DECIMAL(req.body.amount))
    RESPOND 201, {"saved": TRUE}
  END TRANSACTION
END FUNCTION

FUNCTION FailEntry(req)
  TRANSACTION
    SQLEXEC("INSERT INTO ledger (id, label, amount) VALUES ($1, $2, $3)", req.body.id, req.body.label, DECIMAL(req.body.amount))
    THROW HTTPERROR(400, "ROLLED BACK")
  END TRANSACTION
END FUNCTION

FUNCTION ListEntries(req)
  LET rows = SQL("SELECT id, label, amount FROM ledger ORDER BY id")
  RETURN {"entries": rows}
END FUNCTION
