' TRANSACTION commit, rollback on error, helper functions, RETURN, and named connections (Requirement 9.5).
DBCONNECT "sqlite://:memory:"
DBCONNECT "sqlite://:memory:" AS audit

SQLEXEC("CREATE TABLE accounts (id INTEGER PRIMARY KEY, balance NUMERIC(12,2) NOT NULL)")
SQLEXEC("INSERT INTO accounts (id, balance) VALUES ($1, $2)", 1, DECIMAL("100.00"))
SQLEXEC("INSERT INTO accounts (id, balance) VALUES ($1, $2)", 2, DECIMAL("50.00"))

' Commit via helper function inside TRANSACTION.
TRANSACTION
  Transfer(1, 2, DECIMAL("25.50"))
END TRANSACTION
PRINT Balances()

' Rollback when an error escapes TRANSACTION (including writes in helper functions).
TRY
  TRANSACTION
    Transfer(1, 2, DECIMAL("40.00"))
    THROW ERROR("INSUFFICIENT SPIRIT")
  END TRANSACTION
CATCH e
  PRINT e.kind & ": " & e.message
END TRY
PRINT Balances()

' RETURN inside TRANSACTION commits before returning.
PRINT Bonus(2, DECIMAL("10.00"))
PRINT Balances()

' Named connection via TRANSACTION ON audit.
TRANSACTION ON audit
  SQLEXEC("CREATE TABLE events (note TEXT NOT NULL)")
  SQLEXEC("INSERT INTO events (note) VALUES ($1)", "AUDIT OK")
  PRINT SQLONE("SELECT note FROM events").note
END TRANSACTION

SUB Transfer(fromId, toId, amount)
  SQLEXEC("UPDATE accounts SET balance = balance - $1 WHERE id = $2", amount, fromId)
  SQLEXEC("UPDATE accounts SET balance = balance + $1 WHERE id = $2", amount, toId)
END SUB

FUNCTION Bonus(id, amount)
  TRANSACTION
    SQLEXEC("UPDATE accounts SET balance = balance + $1 WHERE id = $2", amount, id)
    RETURN SQLONE("SELECT balance FROM accounts WHERE id = $1", id).balance
  END TRANSACTION
END FUNCTION

FUNCTION Balances()
  LET a = SQLONE("SELECT balance FROM accounts WHERE id = 1").balance
  LET b = SQLONE("SELECT balance FROM accounts WHERE id = 2").balance
  RETURN a & "/" & b
END FUNCTION
