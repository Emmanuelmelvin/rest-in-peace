' DBCONNECT, SQL, SQLONE, SQLEXEC, $n placeholders, and NUMERIC -> DECIMAL (Requirement 9).
DBCONNECT "sqlite://:memory:"

SQLEXEC("CREATE TABLE items (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, price NUMERIC(12,2) NOT NULL)")
LET n1 = SQLEXEC("INSERT INTO items (name, price) VALUES ($1, $2)", "Coffin", DECIMAL("199.95"))
LET n2 = SQLEXEC("INSERT INTO items (name, price) VALUES ($1, $2)", "Candle", DECIMAL("15.00"))
PRINT n1 + n2

LET rows = SQL("SELECT name, price FROM items ORDER BY id")
PRINT LEN(rows)
FOR EACH row IN rows
  PRINT row.name & ": " & row.price
NEXT

LET total = rows[0].price + rows[1].price
PRINT total

LET one = SQLONE("SELECT '$1' AS tag, name, price FROM items WHERE name = $1", "Candle")
PRINT one.tag & " " & one.name & " " & one.price

LET missing = SQLONE("SELECT name FROM items WHERE name = $1", "Ghost")
PRINT missing = NIL

LET updated = SQLEXEC("UPDATE items SET price = $1 WHERE name = $2", DECIMAL("20.00"), "Candle")
PRINT updated
PRINT SQLONE("SELECT price FROM items WHERE name = $1", "Candle").price

LET deleted = SQLEXEC("DELETE FROM items WHERE name = $1", "Coffin")
PRINT deleted
PRINT LEN(SQL("SELECT id FROM items"))
