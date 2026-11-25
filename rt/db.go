package rt

// Database access (Requirement 9). Two drivers sit behind one interface:
// pgx pools for postgres:// and database/sql + modernc.org/sqlite for sqlite://.
// Queries always use $1, $2… placeholders; the SQLite driver rewrites them to ?1, ?2…

// DB is one open database (one DBCONNECT).
type DB interface {
	Query(c *Ctx, query string, args []Value) (*Array, error)
	Exec(c *Ctx, query string, args []Value) (int64, error)
	Close() error
}

// DBConnect implements DBCONNECT url [AS name].
func DBConnect(c *Ctx, url string, name string) {
	// TODO(task 3.1): open the pgx pool or SQLite database; register NUMERIC → DECIMAL.
	panic(notBuilt("DBCONNECT", "3.1"))
}

// SQL implements SQL(query, args…), which returns an array of row maps.
func SQL(c *Ctx, query string, args ...Value) *Array {
	// TODO(task 3.1)
	panic(notBuilt("SQL", "3.1"))
}

// SQLOne implements SQLONE(query, args…), which returns the first row or NIL.
func SQLOne(c *Ctx, query string, args ...Value) Value {
	// TODO(task 3.1)
	panic(notBuilt("SQLONE", "3.1"))
}

// SQLExec implements SQLEXEC(query, args…), which returns the affected row count.
func SQLExec(c *Ctx, query string, args ...Value) int64 {
	// TODO(task 3.1)
	panic(notBuilt("SQLEXEC", "3.1"))
}

// Tx implements TRANSACTION [ON name] … END TRANSACTION (Requirement 9.5).
// It commits when body returns normally or panics with *Respond, and rolls back
// on any other panic, then re-panics.
func Tx(c *Ctx, name string, body func()) {
	// TODO(task 3.2): store the transaction on c so every SQL call in body uses it.
	panic(notBuilt("TRANSACTION", "3.2"))
}
