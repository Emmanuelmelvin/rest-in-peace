package rt

import (
	"context"
	"encoding/json"
	"math"
	"math/big"
	"net"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"
)

func catchError(t *testing.T, fn func()) (err *Error) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			return
		}
		e, ok := r.(*Error)
		if !ok {
			t.Fatalf("expected *rt.Error panic, got %T (%v)", r, r)
		}
		err = e
	}()
	fn()
	return nil
}

func TestRewritePlaceholders(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "no dollar sign",
			in:   "SELECT id, name FROM users",
			want: "SELECT id, name FROM users",
		},
		{
			name: "basic placeholders",
			in:   "SELECT * FROM users WHERE id = $1 AND role = $2",
			want: "SELECT * FROM users WHERE id = ?1 AND role = ?2",
		},
		{
			name: "multi-digit and repeated placeholders",
			in:   "SELECT $10, $2, $1, $10 FROM t",
			want: "SELECT ?10, ?2, ?1, ?10 FROM t",
		},
		{
			name: "single-quoted string literal untouched",
			in:   "SELECT '$1', 'it''s $2 here', $1",
			want: "SELECT '$1', 'it''s $2 here', ?1",
		},
		{
			name: "empty and escaped quotes around placeholder",
			in:   "SELECT '' || $1, '''$2''' || $2",
			want: "SELECT '' || ?1, '''$2''' || ?2",
		},
		{
			name: "double-quoted identifier untouched",
			in:   `SELECT "col$1", "a""$2", $1 FROM "tbl$3"`,
			want: `SELECT "col$1", "a""$2", ?1 FROM "tbl$3"`,
		},
		{
			name: "backtick and bracket identifiers untouched",
			in:   "SELECT `col$1`, [col$2], $1 FROM t",
			want: "SELECT `col$1`, [col$2], ?1 FROM t",
		},
		{
			name: "line comment untouched",
			in:   "SELECT $1 -- ignore $2\n+ $2",
			want: "SELECT ?1 -- ignore $2\n+ ?2",
		},
		{
			name: "block comment untouched",
			in:   "SELECT /* $1 and $2 */ $1, /* $3 */ $2",
			want: "SELECT /* $1 and $2 */ ?1, /* $3 */ ?2",
		},
		{
			name: "identifier containing dollar untouched",
			in:   "SELECT foo$1, a1$2, $1 FROM t",
			want: "SELECT foo$1, a1$2, ?1 FROM t",
		},
		{
			name: "zero and non-numeric dollar untouched",
			in:   "SELECT $0, $abc, $, $1abc, $1",
			want: "SELECT $0, $abc, $, $1abc, ?1",
		},
		{
			name: "unterminated string and comment safe",
			in:   "SELECT 'unterminated $1",
			want: "SELECT 'unterminated $1",
		},
		{
			name: "unterminated block comment safe",
			in:   "SELECT $1 /* unterminated $2",
			want: "SELECT ?1 /* unterminated $2",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := rewritePlaceholders(tc.in); got != tc.want {
				t.Errorf("rewritePlaceholders(%q)\n got: %q\nwant: %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestParseSQLiteDSN(t *testing.T) {
	cases := []struct {
		url      string
		wantDSN  string
		wantMem  bool
		wantFail bool
	}{
		{"sqlite://:memory:", ":memory:", true, false},
		{"sqlite://:memory:?mode=memory", ":memory:?mode=memory", true, false},
		{"sqlite:///abs/path.db", "/abs/path.db", false, false},
		{"sqlite:///C:/Users/test/app.db", "C:/Users/test/app.db", false, false},
		{"sqlite://./rel.db", "./rel.db", false, false},
		{"sqlite://file.db", "file.db", false, false},
		{"SQLITE://:memory:", ":memory:", true, false},
		{"sqlite://", "", false, true},
		{"postgres://localhost/db", "", false, true},
	}

	for _, tc := range cases {
		t.Run(tc.url, func(t *testing.T) {
			dsn, mem, err := parseSQLiteDSN(tc.url)
			if tc.wantFail {
				if err == nil {
					t.Fatalf("parseSQLiteDSN(%q) succeeded (%q), want error", tc.url, dsn)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSQLiteDSN(%q) unexpected error: %v", tc.url, err)
			}
			if dsn != tc.wantDSN || mem != tc.wantMem {
				t.Errorf("parseSQLiteDSN(%q) = (%q, %v), want (%q, %v)", tc.url, dsn, mem, tc.wantDSN, tc.wantMem)
			}
		})
	}
}

func TestSanitizeURL(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"postgres://alice:supersecret@db.example.com:5432/app", "postgres://alice:REDACTED@db.example.com:5432/app"},
		{"postgres://alice@db.example.com:5432/app", "postgres://alice@db.example.com:5432/app"},
		{"sqlite://:memory:", "sqlite://:memory:"},
	}
	for _, tc := range cases {
		if got := sanitizeURL(tc.in); got != tc.want {
			t.Errorf("sanitizeURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestParseDecimalColumnType(t *testing.T) {
	cases := []struct {
		in       string
		isDec    bool
		scale    int32
		hasScale bool
	}{
		{"NUMERIC", true, 0, false},
		{"DECIMAL", true, 0, false},
		{"NUMERIC(12,2)", true, 2, true},
		{"decimal(10, 4)", true, 4, true},
		{"NUMERIC(12)", true, 0, false},
		{"TEXT", false, 0, false},
		{"INTEGER", false, 0, false},
		{"REAL", false, 0, false},
		{"", false, 0, false},
	}
	for _, tc := range cases {
		isDec, scale, hasScale := parseDecimalColumnType(tc.in)
		if isDec != tc.isDec || scale != tc.scale || hasScale != tc.hasScale {
			t.Errorf("parseDecimalColumnType(%q) = (%v, %d, %v), want (%v, %d, %v)",
				tc.in, isDec, scale, hasScale, tc.isDec, tc.scale, tc.hasScale)
		}
	}
}

func TestPGConversions(t *testing.T) {
	t.Run("pgtype.Numeric decodes to rt.Decimal with scale preserved", func(t *testing.T) {
		n := pgtype.Numeric{Int: big.NewInt(21500), Exp: -2, Valid: true}
		v := pgValueToRT(n)
		d, ok := v.(Decimal)
		if !ok {
			t.Fatalf("got %T, want rt.Decimal", v)
		}
		if got := formatDecimal(d); got != "215.00" {
			t.Errorf("formatDecimal = %q, want %q", got, "215.00")
		}
	})

	t.Run("invalid pgtype.Numeric decodes to nil", func(t *testing.T) {
		if v := pgValueToRT(pgtype.Numeric{Valid: false}); v != nil {
			t.Errorf("got %v, want nil", v)
		}
	})

	t.Run("NaN and Infinity pgtype.Numeric", func(t *testing.T) {
		if f, ok := pgValueToRT(pgtype.Numeric{NaN: true, Valid: true}).(float64); !ok || !math.IsNaN(f) {
			t.Errorf("expected NaN float64, got %#v", f)
		}
		if f, ok := pgValueToRT(pgtype.Numeric{InfinityModifier: pgtype.Infinity, Valid: true}).(float64); !ok || !math.IsInf(f, 1) {
			t.Errorf("expected +Inf, got %#v", f)
		}
		if f, ok := pgValueToRT(pgtype.Numeric{InfinityModifier: pgtype.NegativeInfinity, Valid: true}).(float64); !ok || !math.IsInf(f, -1) {
			t.Errorf("expected -Inf, got %#v", f)
		}
	})

	t.Run("UUID and JSONB map/array conversion", func(t *testing.T) {
		uuidBytes := [16]byte{0x12, 0x34, 0x56, 0x78, 0x9a, 0xbc, 0xde, 0xf0, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88}
		if got := pgValueToRT(uuidBytes); got != "12345678-9abc-def0-1122-334455667788" {
			t.Errorf("UUID = %q", got)
		}
		m, ok := pgValueToRT(map[string]any{"b": int32(2), "a": "x"}).(*Map)
		if !ok {
			t.Fatalf("expected *Map")
		}
		if !reflect.DeepEqual(m.Keys(), []string{"a", "b"}) {
			t.Errorf("keys = %v, want [a b]", m.Keys())
		}
		if v, _ := m.Get("b"); v != int64(2) {
			t.Errorf("m[b] = %#v, want int64(2)", v)
		}
	})

	t.Run("toPGArgs converts Decimal to pgtype.Numeric without float", func(t *testing.T) {
		d := DecimalOf("12345678901234567890.25")
		args, err := toPGArgs([]Value{d, MapOf("k", "v"), int64(7)})
		if err != nil {
			t.Fatal(err)
		}
		n, ok := args[0].(pgtype.Numeric)
		if !ok || !n.Valid {
			t.Fatalf("args[0] = %#v, want valid pgtype.Numeric", args[0])
		}
		roundTrip := pgNumericToDecimal(n).(Decimal)
		if !roundTrip.Equal(d) || formatDecimal(roundTrip) != "12345678901234567890.25" {
			t.Errorf("roundTrip = %q, want 12345678901234567890.25", formatDecimal(roundTrip))
		}
		if args[1] != `{"k":"v"}` {
			t.Errorf("args[1] = %#v, want JSON string", args[1])
		}
		if args[2] != int64(7) {
			t.Errorf("args[2] = %#v, want 7", args[2])
		}
	})
}

func TestSQLiteCRUDAndColumnOrder(t *testing.T) {
	defer ResetDBs()
	ResetDBs()

	c := NewCtx(context.Background())
	DBConnect(c, "sqlite://:memory:", "")

	SQLExec(c, `CREATE TABLE users (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		bio TEXT,
		score REAL
	)`)

	if n := SQLExec(c, `INSERT INTO users (name, bio, score) VALUES ($1, $2, $3)`, "Ada", "says $1 literally", float64(9.5)); n != 1 {
		t.Fatalf("INSERT affected = %d, want 1", n)
	}
	if n := SQLExec(c, `INSERT INTO users (name, bio, score) VALUES ($1, $2, $3)`, "Grace", nil, float64(10.0)); n != 1 {
		t.Fatalf("INSERT 2 affected = %d, want 1", n)
	}

	// SQL returns *Array of *Map preserving column order from SELECT.
	rows := SQL(c, `SELECT score, name, bio, id FROM users WHERE name = $1 OR name = $2 ORDER BY id`, "Ada", "Grace")
	if rows.Len() != 2 {
		t.Fatalf("len(rows) = %d, want 2", rows.Len())
	}
	r0 := rows.Items[0].(*Map)
	wantKeys := []string{"score", "name", "bio", "id"}
	if !reflect.DeepEqual(r0.Keys(), wantKeys) {
		t.Errorf("row keys = %v, want %v", r0.Keys(), wantKeys)
	}
	if got := Get(r0, "name"); got != "Ada" {
		t.Errorf("r0.name = %#v, want Ada", got)
	}
	if got := Get(r0, "bio"); got != "says $1 literally" {
		t.Errorf("r0.bio = %#v", got)
	}
	r1 := rows.Items[1].(*Map)
	if got := Get(r1, "bio"); got != nil {
		t.Errorf("r1.bio = %#v, want nil", got)
	}

	// Empty result returns empty *Array (non-nil) for SQL and nil for SQLOne.
	empty := SQL(c, `SELECT id FROM users WHERE name = $1`, "Nobody")
	if empty == nil || empty.Len() != 0 {
		t.Fatalf("empty SQL = %#v, want non-nil empty *Array", empty)
	}
	if one := SQLOne(c, `SELECT id FROM users WHERE name = $1`, "Nobody"); one != nil {
		t.Fatalf("empty SQLOne = %#v, want nil", one)
	}

	// INSERT ... RETURNING * via SQLOne (mirrors examples/invoicing/main.bas).
	inserted := SQLOne(c, `INSERT INTO users (name, bio, score) VALUES ($1, $2, $3) RETURNING id, name`, "Alan", "$1 literal", float64(8.0))
	if inserted == nil {
		t.Fatal("SQLOne RETURNING returned nil")
	}
	if got := Get(inserted, "name"); got != "Alan" {
		t.Errorf("inserted.name = %#v, want Alan", got)
	}

	// Repeated placeholder $1 and string literal containing '$1'.
	found := SQLOne(c, `SELECT '$1' AS lit, name FROM users WHERE name = $1 OR bio = $1`, "Ada")
	if Get(found, "lit") != "$1" || Get(found, "name") != "Ada" {
		t.Errorf("repeated placeholder row = %#v", found)
	}

	// UPDATE and DELETE affected counts.
	if n := SQLExec(c, `UPDATE users SET score = $1 WHERE name = $2`, float64(9.9), "Ada"); n != 1 {
		t.Errorf("UPDATE affected = %d, want 1", n)
	}
	if n := SQLExec(c, `DELETE FROM users WHERE score >= $1`, float64(9.0)); n != 2 {
		t.Errorf("DELETE affected = %d, want 2", n)
	}
}

func TestSQLiteFilePersistence(t *testing.T) {
	defer ResetDBs()
	ResetDBs()

	dbFile := filepath.Join(t.TempDir(), "app.db")
	dbURL := "sqlite://" + filepath.ToSlash(dbFile)

	c := NewCtx(context.Background())
	DBConnect(c, dbURL, "")
	SQLExec(c, `CREATE TABLE kv (k TEXT PRIMARY KEY, v TEXT)`)
	SQLExec(c, `INSERT INTO kv (k, v) VALUES ($1, $2)`, "hello", "world")
	ResetDBs()

	// Reconnect and verify data persisted on disk.
	DBConnect(c, dbURL, "")
	row := SQLOne(c, `SELECT v FROM kv WHERE k = $1`, "hello")
	if Get(row, "v") != "world" {
		t.Fatalf("persisted value = %#v, want world", Get(row, "v"))
	}
}

func TestSQLiteDecimalRoundTrip(t *testing.T) {
	defer ResetDBs()
	ResetDBs()

	c := NewCtx(context.Background())
	DBConnect(c, "sqlite://:memory:", "")

	SQLExec(c, `CREATE TABLE invoices (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		subtotal NUMERIC(12,2) NOT NULL,
		tax DECIMAL(10,4) NOT NULL,
		total NUMERIC(12,2) NOT NULL,
		raw_amt NUMERIC,
		discount NUMERIC(12,2)
	)`)

	sub := DecimalOf("200.00")
	tax := DecimalOf("15.0000")
	total := DecimalOf("215.00")
	raw := DecimalOf("19.99")

	inv := SQLOne(c,
		`INSERT INTO invoices (subtotal, tax, total, raw_amt, discount) VALUES ($1, $2, $3, $4, $5) RETURNING *`,
		sub, tax, total, raw, nil,
	)

	// Verify every NUMERIC/DECIMAL column scanned back as rt.Decimal (not float64).
	for _, tc := range []struct {
		col  string
		want string
	}{
		{"subtotal", "200.00"},
		{"tax", "15.0000"},
		{"total", "215.00"},
		{"raw_amt", "19.99"},
	} {
		v := Get(inv, tc.col)
		d, ok := v.(Decimal)
		if !ok {
			t.Fatalf("column %s got %T (%#v), want rt.Decimal", tc.col, v, v)
		}
		if got := formatDecimal(d); got != tc.want {
			t.Errorf("column %s = %q, want %q", tc.col, got, tc.want)
		}
	}
	if disc := Get(inv, "discount"); disc != nil {
		t.Errorf("NULL NUMERIC column got %#v, want nil", disc)
	}

	// Verify SELECT returns exact Decimal values and JSON serializes scale-preserving strings.
	fetched := SQLOne(c, `SELECT subtotal, total FROM invoices WHERE id = $1`, Get(inv, "id"))
	sum := Add(Get(fetched, "subtotal"), DecimalOf("0.50"))
	if d, ok := sum.(Decimal); !ok || formatDecimal(d) != "200.50" {
		t.Errorf("DECIMAL arithmetic on fetched column = %#v, want 200.50", sum)
	}
	b, err := json.Marshal(fetched)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(b), `{"subtotal":"200.00","total":"215.00"}`; got != want {
		t.Errorf("JSON = %s, want %s", got, want)
	}
}

func TestTransactionSemantics(t *testing.T) {
	defer ResetDBs()
	ResetDBs()

	c := NewCtx(context.Background())
	DBConnect(c, "sqlite://:memory:", "")
	SQLExec(c, `CREATE TABLE accounts (id INTEGER PRIMARY KEY, balance INTEGER NOT NULL CHECK (balance >= 0))`)
	SQLExec(c, `INSERT INTO accounts (id, balance) VALUES (1, 100), (2, 50)`)

	transfer := func(ctx *Ctx, from, to, amount int64) {
		SQLExec(ctx, `UPDATE accounts SET balance = balance - $1 WHERE id = $2`, amount, from)
		SQLExec(ctx, `UPDATE accounts SET balance = balance + $1 WHERE id = $2`, amount, to)
	}

	t.Run("commit on normal return including helper function calls", func(t *testing.T) {
		Tx(c, "", func() {
			transfer(c, 1, 2, 25)
		})
		if c.activeTx() != nil {
			t.Fatal("activeTx not cleared after commit")
		}
		if got := Get(SQLOne(c, `SELECT balance FROM accounts WHERE id = 1`), "balance"); got != int64(75) {
			t.Errorf("acct 1 balance = %v, want 75", got)
		}
		if got := Get(SQLOne(c, `SELECT balance FROM accounts WHERE id = 2`), "balance"); got != int64(75) {
			t.Errorf("acct 2 balance = %v, want 75", got)
		}
	})

	t.Run("rollback on thrown error including helper function writes", func(t *testing.T) {
		e := catchError(t, func() {
			Tx(c, "", func() {
				transfer(c, 1, 2, 30)
				// Uncommitted state is visible inside the tx.
				if got := Get(SQLOne(c, `SELECT balance FROM accounts WHERE id = 1`), "balance"); got != int64(45) {
					t.Errorf("inside tx acct 1 = %v, want 45", got)
				}
				Throw(NewError("abort transfer"))
			})
		})
		if e == nil || e.Kind != KindUser || e.Message != "abort transfer" {
			t.Fatalf("got error %#v, want USER abort transfer", e)
		}
		if c.activeTx() != nil {
			t.Fatal("activeTx not cleared after rollback")
		}
		// Balances must remain 75 / 75.
		if got := Get(SQLOne(c, `SELECT balance FROM accounts WHERE id = 1`), "balance"); got != int64(75) {
			t.Errorf("after rollback acct 1 = %v, want 75", got)
		}
		if got := Get(SQLOne(c, `SELECT balance FROM accounts WHERE id = 2`), "balance"); got != int64(75) {
			t.Errorf("after rollback acct 2 = %v, want 75", got)
		}
	})

	t.Run("rollback on SQL constraint error", func(t *testing.T) {
		e := catchError(t, func() {
			Tx(c, "", func() {
				// First statement succeeds, second violates CHECK (balance >= 0).
				SQLExec(c, `UPDATE accounts SET balance = balance + $1 WHERE id = 2`, int64(500))
				SQLExec(c, `UPDATE accounts SET balance = balance - $1 WHERE id = 1`, int64(500))
			})
		})
		if e == nil || e.Kind != KindDatabase {
			t.Fatalf("got error %#v, want KindDatabase", e)
		}
		if got := Get(SQLOne(c, `SELECT balance FROM accounts WHERE id = 2`), "balance"); got != int64(75) {
			t.Errorf("after SQL error rollback acct 2 = %v, want 75", got)
		}
	})

	t.Run("commit on RESPOND panic and re-panic *Respond", func(t *testing.T) {
		var caughtResp *Respond
		func() {
			defer func() {
				if r := recover(); r != nil {
					resp, ok := r.(*Respond)
					if !ok {
						t.Fatalf("expected *Respond, got %T (%v)", r, r)
					}
					caughtResp = resp
				}
			}()
			Tx(c, "", func() {
				transfer(c, 1, 2, 10)
				RespondNow(c, 201, MapOf("ok", true))
			})
		}()
		if caughtResp == nil || caughtResp.Status != 201 {
			t.Fatalf("caughtResp = %#v, want status 201", caughtResp)
		}
		if c.activeTx() != nil {
			t.Fatal("activeTx not cleared after RESPOND commit")
		}
		if got := Get(SQLOne(c, `SELECT balance FROM accounts WHERE id = 1`), "balance"); got != int64(65) {
			t.Errorf("after RESPOND commit acct 1 = %v, want 65", got)
		}
		if got := Get(SQLOne(c, `SELECT balance FROM accounts WHERE id = 2`), "balance"); got != int64(85) {
			t.Errorf("after RESPOND commit acct 2 = %v, want 85", got)
		}
	})

	t.Run("nested transaction uses savepoint", func(t *testing.T) {
		Tx(c, "", func() {
			SQLExec(c, `UPDATE accounts SET balance = balance + 5 WHERE id = 1`) // 65 -> 70
			// Inner tx fails and is caught; outer tx keeps going and commits.
			e := catchError(t, func() {
				Tx(c, "", func() {
					SQLExec(c, `UPDATE accounts SET balance = balance + 100 WHERE id = 1`)
					Throw(NewError("inner fail"))
				})
			})
			if e == nil || e.Message != "inner fail" {
				t.Fatalf("expected inner fail error, got %#v", e)
			}
		})
		if got := Get(SQLOne(c, `SELECT balance FROM accounts WHERE id = 1`), "balance"); got != int64(70) {
			t.Errorf("after nested savepoint rollback + outer commit acct 1 = %v, want 70", got)
		}
	})
}

func TestNamedConnections(t *testing.T) {
	defer ResetDBs()
	ResetDBs()

	c := NewCtx(context.Background())
	DBConnect(c, "sqlite://:memory:", "")
	DBConnect(c, "sqlite://:memory:", "reports")

	SQLExec(c, `CREATE TABLE items (label TEXT)`)
	SQLExec(c, `INSERT INTO items (label) VALUES ($1)`, "default-item")

	// Setup and query the named "reports" connection via TRANSACTION ON reports
	// (case-insensitive connection name).
	Tx(c, "REPORTS", func() {
		SQLExec(c, `CREATE TABLE summaries (label TEXT)`)
		SQLExec(c, `INSERT INTO summaries (label) VALUES ($1)`, "report-1")
	})

	// Nested: outer tx on default, inner tx on reports; verify routing and restoration.
	Tx(c, "", func() {
		SQLExec(c, `INSERT INTO items (label) VALUES ($1)`, "default-tx-item")
		Tx(c, "reports", func() {
			SQLExec(c, `INSERT INTO summaries (label) VALUES ($1)`, "report-2")
			rows := SQL(c, `SELECT label FROM summaries ORDER BY label`)
			if rows.Len() != 2 {
				t.Fatalf("reports summaries count = %d, want 2", rows.Len())
			}
		})
		// After inner reports tx ends, SQL routes back to default tx.
		rows := SQL(c, `SELECT label FROM items ORDER BY label`)
		if rows.Len() != 2 {
			t.Fatalf("default items count = %d, want 2", rows.Len())
		}
	})

	// Default DB should not have the summaries table.
	if e := catchError(t, func() { SQL(c, `SELECT * FROM summaries`) }); e == nil || e.Kind != KindDatabase {
		t.Fatalf("expected KindDatabase querying reports table on default DB, got %#v", e)
	}

	// Reports DB retains its committed rows.
	Tx(c, "reports", func() {
		rows := SQL(c, `SELECT label FROM summaries ORDER BY label`)
		if rows.Len() != 2 {
			t.Fatalf("persisted reports count = %d, want 2", rows.Len())
		}
	})
}

func TestDatabaseErrors(t *testing.T) {
	defer ResetDBs()
	ResetDBs()

	c := NewCtx(context.Background())

	t.Run("SQL without DBCONNECT", func(t *testing.T) {
		e := catchError(t, func() { SQL(c, "SELECT 1") })
		if e == nil || e.Kind != KindDatabase || !strings.Contains(e.Message, "DBCONNECT") {
			t.Fatalf("got %#v, want DATABASE error mentioning DBCONNECT", e)
		}
	})

	t.Run("SQLExec without DBCONNECT", func(t *testing.T) {
		e := catchError(t, func() { SQLExec(c, "CREATE TABLE x (a INT)") })
		if e == nil || e.Kind != KindDatabase {
			t.Fatalf("got %#v, want DATABASE error", e)
		}
	})

	t.Run("Tx on missing named connection", func(t *testing.T) {
		e := catchError(t, func() { Tx(c, "analytics", func() {}) })
		if e == nil || e.Kind != KindDatabase || !strings.Contains(e.Message, "ANALYTICS") {
			t.Fatalf("got %#v, want DATABASE error naming ANALYTICS", e)
		}
	})

	t.Run("DBConnect empty URL", func(t *testing.T) {
		e := catchError(t, func() { DBConnect(c, "", "") })
		if e == nil || e.Kind != KindDatabase {
			t.Fatalf("got %#v, want DATABASE error", e)
		}
	})

	t.Run("DBConnect unsupported scheme", func(t *testing.T) {
		e := catchError(t, func() { DBConnect(c, "mysql://user:pw@localhost/db", "") })
		if e == nil || e.Kind != KindDatabase {
			t.Fatalf("got %#v, want DATABASE error", e)
		}
		if strings.Contains(e.Message, "pw") {
			t.Errorf("error message leaked password: %s", e.Message)
		}
	})

	t.Run("DBConnect unreachable postgres redacts password", func(t *testing.T) {
		e := catchError(t, func() {
			DBConnect(c, "postgres://admin:topsecret@127.0.0.1:1/rip?connect_timeout=1", "")
		})
		if e == nil || e.Kind != KindDatabase {
			t.Fatalf("got %#v, want DATABASE error", e)
		}
		if strings.Contains(e.Message, "topsecret") {
			t.Errorf("error message leaked password: %s", e.Message)
		}
	})

	t.Run("SQL syntax error", func(t *testing.T) {
		DBConnect(c, "sqlite://:memory:", "")
		e := catchError(t, func() { SQL(c, "SELECT FROM WHERE") })
		if e == nil || e.Kind != KindDatabase {
			t.Fatalf("got %#v, want DATABASE error", e)
		}
	})
}

func TestShutdownClosesDBs(t *testing.T) {
	defer ResetDBs()
	ResetDBs()

	c := NewCtx(context.Background())
	DBConnect(c, "sqlite://:memory:", "")
	if got := Get(SQLOne(c, "SELECT 1 AS ok"), "ok"); got != int64(1) {
		t.Fatalf("pre-shutdown query got %#v, want 1", got)
	}

	// Trigger graceful shutdown via App.serveOn on a loopback listener.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // immediately shut down
	if err := c.App().serveOn(ctx, ln); err != nil {
		t.Fatalf("serveOn error: %v", err)
	}

	// After shutdown, the underlying SQLite DB must be closed.
	if e := catchError(t, func() { SQL(c, "SELECT 1") }); e == nil || e.Kind != KindDatabase {
		t.Fatalf("expected DATABASE error after shutdown closed DB, got %#v", e)
	}
}

func TestDecimalEqualHelperSanity(t *testing.T) {
	// Ensure decimal.Decimal returned from SQLite matches shopspring/decimal values.
	d1 := decimal.NewFromInt(42)
	d2 := DecimalOf(int64(42))
	if !d1.Equal(d2) {
		t.Fatalf("expected %v == %v", d1, d2)
	}
}

func TestDBCRUDHelpers(t *testing.T) {
	defer ResetDBs()
	ResetDBs()

	c := NewCtx(context.Background())
	DBConnect(c, "sqlite://:memory:", "")
	SQLExec(c, "CREATE TABLE users (id INTEGER PRIMARY KEY AUTOINCREMENT, name TEXT NOT NULL, email TEXT, balance NUMERIC(12,2) NOT NULL)")

	// DBInsert returns inserted row via RETURNING *.
	m1 := NewMap()
	m1.Set("name", "Ada")
	m1.Set("email", "ada@example.com")
	m1.Set("balance", DecimalOf("99.50"))
	u1 := DBInsert(c, "users", m1)
	if got := Get(u1, "name"); got != "Ada" {
		t.Fatalf("DBInsert name = %#v, want Ada", got)
	}
	if got := ToString(Get(u1, "balance")); got != "99.50" {
		t.Fatalf("DBInsert balance = %q, want 99.50", got)
	}
	id1 := Get(u1, "id")

	m2 := NewMap()
	m2.Set("name", "Grace")
	m2.Set("email", nil)
	m2.Set("balance", DecimalOf("200.00"))
	DBInsert(c, "users", m2)

	// DBFind with no filter, nil filter, and empty map returns all rows.
	if rows := DBFind(c, "users"); len(rows.Items) != 2 {
		t.Fatalf("DBFind all = %d, want 2", len(rows.Items))
	}
	if rows := DBFind(c, "users", nil); len(rows.Items) != 2 {
		t.Fatalf("DBFind nil = %d, want 2", len(rows.Items))
	}
	if rows := DBFind(c, "users", NewMap()); len(rows.Items) != 2 {
		t.Fatalf("DBFind empty map = %d, want 2", len(rows.Items))
	}

	// DBFind with NIL value generates IS NULL.
	noEmail := NewMap()
	noEmail.Set("email", nil)
	nullRows := DBFind(c, "users", noEmail)
	if len(nullRows.Items) != 1 || Get(nullRows.Items[0], "name") != "Grace" {
		t.Fatalf("DBFind IS NULL = %#v, want Grace", nullRows.Items)
	}

	// DBFindOne returns first match or nil.
	byID := NewMap()
	byID.Set("id", id1)
	found := DBFindOne(c, "users", byID)
	if Get(found, "name") != "Ada" {
		t.Fatalf("DBFindOne = %#v, want Ada", found)
	}
	missing := NewMap()
	missing.Set("name", "Nobody")
	if got := DBFindOne(c, "users", missing); got != nil {
		t.Fatalf("DBFindOne missing = %#v, want nil", got)
	}

	// DBUpdate updates matching rows and works inside TRANSACTION.
	patch := NewMap()
	patch.Set("balance", DecimalOf("150.25"))
	Tx(c, "", func() {
		if n := DBUpdate(c, "users", patch, byID); n != 1 {
			t.Fatalf("DBUpdate affected = %d, want 1", n)
		}
	})
	if got := ToString(Get(DBFindOne(c, "users", byID), "balance")); got != "150.25" {
		t.Fatalf("after DBUpdate balance = %q, want 150.25", got)
	}

	// Rollback works with DBDelete inside TRANSACTION.
	_ = catchError(t, func() {
		Tx(c, "", func() {
			DBDelete(c, "users", byID)
			panic(Errorf(KindUser, "rollback"))
		})
	})
	if DBFindOne(c, "users", byID) == nil {
		t.Fatalf("expected row to survive rolled-back DBDelete")
	}

	// DBDelete removes matching row.
	if n := DBDelete(c, "users", byID); n != 1 {
		t.Fatalf("DBDelete affected = %d, want 1", n)
	}
	if rows := DBFind(c, "users"); len(rows.Items) != 1 {
		t.Fatalf("after DBDelete rows = %d, want 1", len(rows.Items))
	}

	// Identifier validation blocks SQL injection in table and column names.
	t.Run("rejects unsafe table and column names", func(t *testing.T) {
		badTable := catchError(t, func() { DBFind(c, "users; DROP TABLE users") })
		if badTable == nil || badTable.Kind != KindDatabase {
			t.Fatalf("bad table got %#v, want DATABASE error", badTable)
		}
		badColMap := NewMap()
		badColMap.Set("id = 1 OR 1=1 --", int64(1))
		badCol := catchError(t, func() { DBFind(c, "users", badColMap) })
		if badCol == nil || badCol.Kind != KindDatabase {
			t.Fatalf("bad column got %#v, want DATABASE error", badCol)
		}
	})

	// Refuses empty data/where maps and non-map arguments.
	t.Run("rejects empty maps and wrong types", func(t *testing.T) {
		if e := catchError(t, func() { DBInsert(c, "users", NewMap()) }); e == nil || e.Kind != KindDatabase {
			t.Fatalf("empty DBInsert got %#v, want DATABASE error", e)
		}
		if e := catchError(t, func() { DBUpdate(c, "users", patch, NewMap()) }); e == nil || e.Kind != KindDatabase {
			t.Fatalf("empty DBUpdate where got %#v, want DATABASE error", e)
		}
		if e := catchError(t, func() { DBDelete(c, "users", NewMap()) }); e == nil || e.Kind != KindDatabase {
			t.Fatalf("empty DBDelete where got %#v, want DATABASE error", e)
		}
		if e := catchError(t, func() { DBInsert(c, "users", "nope") }); e == nil || e.Kind != KindTypeMismatch {
			t.Fatalf("non-map DBInsert got %#v, want TYPE MISMATCH", e)
		}
		if e := catchError(t, func() { DBFind(c, "users", int64(1)) }); e == nil || e.Kind != KindTypeMismatch {
			t.Fatalf("non-map DBFind got %#v, want TYPE MISMATCH", e)
		}
	})
}

func TestSQLArgsVariants(t *testing.T) {
	defer ResetDBs()
	ResetDBs()

	c := NewCtx(context.Background())
	DBConnect(c, "sqlite://:memory:", "")
	SQLExec(c, "CREATE TABLE orders (id INTEGER PRIMARY KEY AUTOINCREMENT, customer TEXT NOT NULL, status TEXT NOT NULL, total NUMERIC(12,2) NOT NULL)")

	n := SQLExecArgs(c, "INSERT INTO orders (customer, status, total) VALUES ($1, $2, $3)", ArrayOf("Ada", "paid", DecimalOf("120.50")))
	if n != 1 {
		t.Fatalf("SQLExecArgs affected = %d, want 1", n)
	}
	SQLExecArgs(c, "INSERT INTO orders (customer, status, total) VALUES ($1, $2, $3)", ArrayOf("Grace", "open", DecimalOf("75.00")))

	// Dynamic filter array with SQLArgs.
	rows := SQLArgs(c, "SELECT customer, status, total FROM orders WHERE customer = $1 AND status = $2", ArrayOf("Ada", "paid"))
	if len(rows.Items) != 1 {
		t.Fatalf("SQLArgs rows = %d, want 1", len(rows.Items))
	}
	if got := Get(rows.Items[0], "customer"); got != "Ada" {
		t.Errorf("customer = %#v, want Ada", got)
	}
	if got := ToString(Get(rows.Items[0], "total")); got != "120.50" {
		t.Errorf("total = %q, want 120.50", got)
	}

	// Empty array and nil both work as zero parameters.
	allEmpty := SQLArgs(c, "SELECT id FROM orders ORDER BY id", ArrayOf())
	if len(allEmpty.Items) != 2 {
		t.Fatalf("SQLArgs empty array rows = %d, want 2", len(allEmpty.Items))
	}
	allNil := SQLArgs(c, "SELECT id FROM orders ORDER BY id", nil)
	if len(allNil.Items) != 2 {
		t.Fatalf("SQLArgs nil rows = %d, want 2", len(allNil.Items))
	}

	// SQLOneArgs returns the first row or nil.
	one := SQLOneArgs(c, "SELECT customer FROM orders WHERE status = $1", ArrayOf("open"))
	if got := Get(one, "customer"); got != "Grace" {
		t.Errorf("SQLOneArgs customer = %#v, want Grace", got)
	}
	if none := SQLOneArgs(c, "SELECT customer FROM orders WHERE status = $1", ArrayOf("missing")); none != nil {
		t.Errorf("SQLOneArgs missing = %#v, want nil", none)
	}

	// Works inside TRANSACTION (commit + rollback).
	Tx(c, "", func() {
		SQLExecArgs(c, "UPDATE orders SET status = $1 WHERE customer = $2", ArrayOf("paid", "Grace"))
	})
	if got := Get(SQLOneArgs(c, "SELECT status FROM orders WHERE customer = $1", ArrayOf("Grace")), "status"); got != "paid" {
		t.Errorf("after tx commit status = %#v, want paid", got)
	}
	_ = catchError(t, func() {
		Tx(c, "", func() {
			SQLExecArgs(c, "UPDATE orders SET status = $1 WHERE customer = $2", ArrayOf("void", "Grace"))
			panic(Errorf(KindUser, "abort"))
		})
	})
	if got := Get(SQLOneArgs(c, "SELECT status FROM orders WHERE customer = $1", ArrayOf("Grace")), "status"); got != "paid" {
		t.Errorf("after tx rollback status = %#v, want paid", got)
	}

	// Non-array second argument raises TYPE MISMATCH.
	for _, tc := range []struct {
		name string
		fn   func()
	}{
		{"SQLArgs", func() { SQLArgs(c, "SELECT 1", "not-an-array") }},
		{"SQLOneArgs", func() { SQLOneArgs(c, "SELECT 1", int64(42)) }},
		{"SQLExecArgs", func() { SQLExecArgs(c, "SELECT 1", NewMap()) }},
	} {
		t.Run(tc.name+"_non_array", func(t *testing.T) {
			e := catchError(t, tc.fn)
			if e == nil || e.Kind != KindTypeMismatch {
				t.Fatalf("got %#v, want TYPE MISMATCH", e)
			}
		})
	}
}
