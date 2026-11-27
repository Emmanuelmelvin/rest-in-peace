package rt

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"math/big"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	_ "modernc.org/sqlite"
)

// Database access (Requirement 9). Two drivers sit behind one interface:
// pgx pools for postgres:// and database/sql + modernc.org/sqlite for sqlite://.
// Queries always use $1, $2… placeholders; the SQLite driver rewrites them to ?1, ?2…

// KindDatabase is the runtime error kind for database connection, query, and
// transaction failures.
const KindDatabase Kind = "DATABASE"

// DB is one open database (one DBCONNECT).
type DB interface {
	Query(c *Ctx, query string, args []Value) (*Array, error)
	Exec(c *Ctx, query string, args []Value) (int64, error)
	Close() error
}

// dbTx is an active transaction (root transaction or nested savepoint).
type dbTx interface {
	Query(c *Ctx, query string, args []Value) (*Array, error)
	Exec(c *Ctx, query string, args []Value) (int64, error)
	Begin(c *Ctx) (dbTx, error)
	Commit(c *Ctx) error
	Rollback(c *Ctx) error
}

type txBeginner interface {
	Begin(c *Ctx) (dbTx, error)
}

var (
	dbMu sync.RWMutex
	dbs  = map[string]DB{}
)

// normalizeDBName canonicalizes a connection name. Empty string and "default"
// (case-insensitive) both map to the default connection "".
func normalizeDBName(name string) string {
	n := strings.ToUpper(strings.TrimSpace(name))
	if n == "" || n == "DEFAULT" {
		return ""
	}
	return n
}

// ResetDBs closes and removes all registered database connections.
// Tests call this to isolate state between runs.
func ResetDBs() {
	dbMu.Lock()
	old := dbs
	dbs = map[string]DB{}
	dbMu.Unlock()
	for _, db := range old {
		_ = db.Close()
	}
}

// DBFor returns the registered database for name ("" for the default), or nil
// if no connection with that name is open.
func DBFor(name string) DB {
	norm := normalizeDBName(name)
	dbMu.RLock()
	db := dbs[norm]
	dbMu.RUnlock()
	return db
}

func mustGetDB(normName string) DB {
	dbMu.RLock()
	db := dbs[normName]
	dbMu.RUnlock()
	if db == nil {
		if normName == "" {
			panic(Errorf(KindDatabase, `no database connection open — call DBCONNECT first, e.g. DBCONNECT "sqlite://:memory:"`))
		}
		panic(Errorf(KindDatabase, "no database connection named %q — call DBCONNECT url AS %s first", normName, strings.ToLower(normName)))
	}
	return db
}

// DBConnect implements DBCONNECT url [AS name] (Requirement 9.1).
// Supported schemes: postgres://, postgresql://, and sqlite://.
// Each opened pool's Close is registered with c.App().OnShutdown (Requirement 14.5).
func DBConnect(c *Ctx, rawURL string, name string) {
	if testDBConnect(c, rawURL, name) { // rip test: open the test database instead (expect.go)
		return
	}
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		panic(Errorf(KindDatabase, `DBCONNECT needs a database URL like "postgres://…" or "sqlite://:memory:"`))
	}

	ctx := ctxOf(c)
	lower := strings.ToLower(trimmed)

	var (
		db  DB
		err error
	)
	switch {
	case strings.HasPrefix(lower, "postgres://"), strings.HasPrefix(lower, "postgresql://"):
		db, err = openPostgres(ctx, trimmed)
	case strings.HasPrefix(lower, "sqlite://"):
		db, err = openSQLite(ctx, trimmed)
	default:
		panic(Errorf(KindDatabase, "unsupported database URL %q — use postgres://… or sqlite://…", sanitizeURL(trimmed)))
	}
	if err != nil {
		panic(&Error{
			Kind:    KindDatabase,
			Message: fmt.Sprintf("could not connect to database (%s): %v", sanitizeURL(trimmed), err),
			Cause:   err,
		})
	}

	norm := normalizeDBName(name)
	dbMu.Lock()
	prev := dbs[norm]
	dbs[norm] = db
	dbMu.Unlock()
	if prev != nil {
		_ = prev.Close()
	}

	if c != nil {
		c.App().OnShutdown(func() {
			_ = db.Close()
		})
	}
}

// SQL implements SQL(query, args…), which returns an array of row maps
// preserving column order (Requirement 9.2).
func SQL(c *Ctx, query string, args ...Value) *Array {
	var (
		rows *Array
		err  error
	)
	if tx := c.activeTx(); tx != nil {
		rows, err = tx.Query(c, query, args)
	} else {
		rows, err = mustGetDB("").Query(c, query, args)
	}
	if err != nil {
		panic(&Error{
			Kind:    KindDatabase,
			Message: fmt.Sprintf("SQL query failed: %v", err),
			Cause:   err,
		})
	}
	return rows
}

// SQLOne implements SQLONE(query, args…), which returns the first row map or NIL
// when the query returns no rows (Requirement 9.2).
func SQLOne(c *Ctx, query string, args ...Value) Value {
	rows := SQL(c, query, args...)
	if rows == nil || len(rows.Items) == 0 {
		return nil
	}
	return rows.Items[0]
}

// SQLExec implements SQLEXEC(query, args…), which returns the affected row count
// (Requirement 9.2).
func SQLExec(c *Ctx, query string, args ...Value) int64 {
	var (
		n   int64
		err error
	)
	if tx := c.activeTx(); tx != nil {
		n, err = tx.Exec(c, query, args)
	} else {
		n, err = mustGetDB("").Exec(c, query, args)
	}
	if err != nil {
		panic(&Error{
			Kind:    KindDatabase,
			Message: fmt.Sprintf("SQLEXEC failed: %v", err),
			Cause:   err,
		})
	}
	return n
}

// quoteSQLIdent validates a table or column identifier against
// ^[A-Za-z_][A-Za-z0-9_]*$ and returns it double-quoted so user input can never
// inject SQL through table or column names.
func quoteSQLIdent(fnName, kind, name string) string {
	if name == "" {
		panic(Errorf(KindDatabase, "%s: %s name cannot be empty", fnName, kind))
	}
	for i := 0; i < len(name); i++ {
		b := name[i]
		isLetter := (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || b == '_'
		isDigit := b >= '0' && b <= '9'
		if i == 0 && !isLetter {
			panic(Errorf(KindDatabase, "%s: invalid %s name %q — use letters, digits, and underscores only", fnName, kind, name))
		}
		if i > 0 && !isLetter && !isDigit {
			panic(Errorf(KindDatabase, "%s: invalid %s name %q — use letters, digits, and underscores only", fnName, kind, name))
		}
	}
	return `"` + name + `"`
}

func requireNonEmptyMap(fnName, role string, v Value) *Map {
	m, ok := v.(*Map)
	if !ok || m == nil {
		panic(Errorf(KindTypeMismatch, `%s expects a MAP for %s, got %s`, fnName, role, TypeName(v)))
	}
	if m.Len() == 0 {
		panic(Errorf(KindDatabase, "%s: %s map cannot be empty", fnName, role))
	}
	return m
}

func optionalWhereMap(fnName string, where []Value) *Map {
	if len(where) == 0 || where[0] == nil {
		return nil
	}
	m, ok := where[0].(*Map)
	if !ok {
		panic(Errorf(KindTypeMismatch, "%s expects a MAP for filter conditions, got %s", fnName, TypeName(where[0])))
	}
	if m == nil || m.Len() == 0 {
		return nil
	}
	return m
}

func buildWhereClause(fnName string, where *Map, startIdx int) (string, []Value) {
	if where == nil || where.Len() == 0 {
		return "", nil
	}
	keys := where.Keys()
	parts := make([]string, 0, len(keys))
	args := make([]Value, 0, len(keys))
	idx := startIdx
	for _, k := range keys {
		col := quoteSQLIdent(fnName, "column", k)
		v, _ := where.Get(k)
		if v == nil {
			parts = append(parts, col+" IS NULL")
			continue
		}
		parts = append(parts, fmt.Sprintf("%s = $%d", col, idx))
		args = append(args, v)
		idx++
	}
	return " WHERE " + strings.Join(parts, " AND "), args
}

// DBInsert implements DBINSERT(table, dataMap), inserting one row and returning
// the inserted row map via RETURNING *.
func DBInsert(c *Ctx, table string, data Value) Value {
	tbl := quoteSQLIdent("DBINSERT", "table", table)
	m := requireNonEmptyMap("DBINSERT", "row data", data)
	keys := m.Keys()
	cols := make([]string, len(keys))
	placeholders := make([]string, len(keys))
	args := make([]Value, len(keys))
	for i, k := range keys {
		cols[i] = quoteSQLIdent("DBINSERT", "column", k)
		placeholders[i] = "$" + strconv.Itoa(i+1)
		v, _ := m.Get(k)
		args[i] = v
	}
	q := "INSERT INTO " + tbl + " (" + strings.Join(cols, ", ") + ") VALUES (" + strings.Join(placeholders, ", ") + ") RETURNING *"
	return SQLOne(c, q, args...)
}

// DBFind implements DBFIND(table [, whereMap]), returning an ARRAY of matching
// row maps.
func DBFind(c *Ctx, table string, where ...Value) *Array {
	tbl := quoteSQLIdent("DBFIND", "table", table)
	wm := optionalWhereMap("DBFIND", where)
	clause, args := buildWhereClause("DBFIND", wm, 1)
	return SQL(c, "SELECT * FROM "+tbl+clause, args...)
}

// DBFindOne implements DBFINDONE(table [, whereMap]), returning the first
// matching row map or NIL when no row matches.
func DBFindOne(c *Ctx, table string, where ...Value) Value {
	tbl := quoteSQLIdent("DBFINDONE", "table", table)
	wm := optionalWhereMap("DBFINDONE", where)
	clause, args := buildWhereClause("DBFINDONE", wm, 1)
	return SQLOne(c, "SELECT * FROM "+tbl+clause+" LIMIT 1", args...)
}

// DBUpdate implements DBUPDATE(table, dataMap, whereMap), updating matching
// rows and returning the affected row count. An empty whereMap is rejected to
// prevent accidental full-table updates.
func DBUpdate(c *Ctx, table string, data Value, where Value) int64 {
	tbl := quoteSQLIdent("DBUPDATE", "table", table)
	dm := requireNonEmptyMap("DBUPDATE", "update data", data)
	wm := requireNonEmptyMap("DBUPDATE", "where filter", where)

	dkeys := dm.Keys()
	sets := make([]string, len(dkeys))
	args := make([]Value, 0, len(dkeys)+wm.Len())
	for i, k := range dkeys {
		col := quoteSQLIdent("DBUPDATE", "column", k)
		sets[i] = fmt.Sprintf("%s = $%d", col, i+1)
		v, _ := dm.Get(k)
		args = append(args, v)
	}
	clause, wargs := buildWhereClause("DBUPDATE", wm, len(dkeys)+1)
	args = append(args, wargs...)
	q := "UPDATE " + tbl + " SET " + strings.Join(sets, ", ") + clause
	return SQLExec(c, q, args...)
}

// DBDelete implements DBDELETE(table, whereMap), deleting matching rows and
// returning the affected row count. An empty whereMap is rejected to prevent
// accidental full-table deletes.
func DBDelete(c *Ctx, table string, where Value) int64 {
	tbl := quoteSQLIdent("DBDELETE", "table", table)
	wm := requireNonEmptyMap("DBDELETE", "where filter", where)
	clause, args := buildWhereClause("DBDELETE", wm, 1)
	return SQLExec(c, "DELETE FROM "+tbl+clause, args...)
}

func unpackSQLArgs(fnName string, params Value) []Value {
	if params == nil {
		return nil
	}
	arr, ok := params.(*Array)
	if !ok {
		panic(Errorf(KindTypeMismatch, "%s expects an ARRAY of parameters as its second argument, got %s", fnName, TypeName(params)))
	}
	if arr == nil {
		return nil
	}
	return arr.Items
}

// SQLArgs implements SQLARGS(query, paramsArray), running SQL with a dynamic
// ARRAY of bind parameters (for dynamic WHERE clauses and reusable helpers).
func SQLArgs(c *Ctx, query string, params Value) *Array {
	return SQL(c, query, unpackSQLArgs("SQLARGS", params)...)
}

// SQLOneArgs implements SQLONEARGS(query, paramsArray), running SQLONE with a
// dynamic ARRAY of bind parameters.
func SQLOneArgs(c *Ctx, query string, params Value) Value {
	return SQLOne(c, query, unpackSQLArgs("SQLONEARGS", params)...)
}

// SQLExecArgs implements SQLEXECARGS(query, paramsArray), running SQLEXEC with
// a dynamic ARRAY of bind parameters.
func SQLExecArgs(c *Ctx, query string, params Value) int64 {
	return SQLExec(c, query, unpackSQLArgs("SQLEXECARGS", params)...)
}

// Tx implements TRANSACTION [ON name] … END TRANSACTION (Requirement 9.5).
// The transaction is bound to c so every SQL, SQLOne, and SQLExec call made
// while body runs — including inside helper functions — uses it.
// It commits when body returns normally or panics with *Respond (Requirement 6.7),
// and rolls back on any other panic, then re-panics.
func Tx(c *Ctx, name string, body func()) {
	if c == nil {
		c = NewCtx(context.Background())
	}
	norm := normalizeDBName(name)

	var (
		tx  dbTx
		err error
	)
	if existing := c.activeTxFor(norm); existing != nil {
		tx, err = existing.Begin(c)
	} else {
		db := mustGetDB(norm)
		beginner, ok := db.(txBeginner)
		if !ok {
			panic(Errorf(KindDatabase, "database driver %T does not support transactions", db))
		}
		tx, err = beginner.Begin(c)
	}
	if err != nil {
		panic(&Error{
			Kind:    KindDatabase,
			Message: fmt.Sprintf("could not begin transaction: %v", err),
			Cause:   err,
		})
	}

	restore := c.pushTx(norm, tx)
	defer func() {
		restore()
		r := recover()
		if r == nil {
			if cerr := tx.Commit(c); cerr != nil {
				panic(&Error{
					Kind:    KindDatabase,
					Message: fmt.Sprintf("could not commit transaction: %v", cerr),
					Cause:   cerr,
				})
			}
			return
		}
		if _, isRespond := r.(*Respond); isRespond {
			if cerr := tx.Commit(c); cerr != nil {
				panic(&Error{
					Kind:    KindDatabase,
					Message: fmt.Sprintf("could not commit transaction before RESPOND: %v", cerr),
					Cause:   cerr,
				})
			}
			panic(r)
		}
		// Preserve the BASIC source location inside body before re-panicking.
		if e, ok := r.(*Error); ok && e.File == "" {
			e.File, e.Line = basLocation()
		}
		_ = tx.Rollback(c)
		panic(r)
	}()

	body()
}

// ---- Context & URL helpers ----

func ctxOf(c *Ctx) context.Context {
	if c != nil && c.ctx != nil {
		return c.ctx
	}
	return context.Background()
}

func rollbackCtx() context.Context {
	// Use an uncanceled background context so rollback still executes if the
	// request context was canceled.
	return context.Background()
}

// sanitizeURL redacts any password in rawURL so error messages never leak
// database credentials (security.md).
func sanitizeURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || u.User == nil {
		return rawURL
	}
	if _, hasPass := u.User.Password(); !hasPass {
		return rawURL
	}
	u.User = url.UserPassword(u.User.Username(), "REDACTED")
	return u.String()
}

// ---- PostgreSQL driver (pgxpool) ----

type pgQuerier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}

type pgDB struct {
	pool      *pgxpool.Pool
	closeOnce sync.Once
}

func openPostgres(ctx context.Context, rawURL string) (DB, error) {
	cfg, err := pgxpool.ParseConfig(rawURL)
	if err != nil {
		return nil, err
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}
	return &pgDB{pool: pool}, nil
}

func (d *pgDB) Query(c *Ctx, query string, args []Value) (*Array, error) {
	return pgQuery(ctxOf(c), d.pool, query, args)
}

func (d *pgDB) Exec(c *Ctx, query string, args []Value) (int64, error) {
	return pgExec(ctxOf(c), d.pool, query, args)
}

func (d *pgDB) Begin(c *Ctx) (dbTx, error) {
	tx, err := d.pool.Begin(ctxOf(c))
	if err != nil {
		return nil, err
	}
	return &pgTx{tx: tx}, nil
}

func (d *pgDB) Close() error {
	d.closeOnce.Do(func() {
		d.pool.Close()
	})
	return nil
}

type pgTx struct {
	tx pgx.Tx
}

func (t *pgTx) Query(c *Ctx, query string, args []Value) (*Array, error) {
	return pgQuery(ctxOf(c), t.tx, query, args)
}

func (t *pgTx) Exec(c *Ctx, query string, args []Value) (int64, error) {
	return pgExec(ctxOf(c), t.tx, query, args)
}

func (t *pgTx) Begin(c *Ctx) (dbTx, error) {
	sub, err := t.tx.Begin(ctxOf(c))
	if err != nil {
		return nil, err
	}
	return &pgTx{tx: sub}, nil
}

func (t *pgTx) Commit(c *Ctx) error {
	return t.tx.Commit(ctxOf(c))
}

func (t *pgTx) Rollback(_ *Ctx) error {
	return t.tx.Rollback(rollbackCtx())
}

func pgQuery(ctx context.Context, q pgQuerier, query string, args []Value) (*Array, error) {
	pgArgs, err := toPGArgs(args)
	if err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, query, pgArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	fds := rows.FieldDescriptions()
	cols := make([]string, len(fds))
	for i, fd := range fds {
		cols[i] = string(fd.Name)
	}

	out := ArrayOf()
	for rows.Next() {
		vals, err := rows.Values()
		if err != nil {
			return nil, err
		}
		row := NewMap()
		for i, col := range cols {
			row.Set(col, pgValueToRT(vals[i]))
		}
		out.Items = append(out.Items, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func pgExec(ctx context.Context, q pgQuerier, query string, args []Value) (int64, error) {
	pgArgs, err := toPGArgs(args)
	if err != nil {
		return 0, err
	}
	tag, err := q.Exec(ctx, query, pgArgs...)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func toPGArgs(args []Value) ([]any, error) {
	if len(args) == 0 {
		return nil, nil
	}
	out := make([]any, len(args))
	for i, a := range args {
		switch x := a.(type) {
		case Decimal:
			out[i] = pgtype.Numeric{
				Int:   x.Coefficient(),
				Exp:   x.Exponent(),
				Valid: true,
			}
		case *Map, *Array:
			b, err := marshalValue(x)
			if err != nil {
				return nil, err
			}
			out[i] = string(b)
		default:
			out[i] = a
		}
	}
	return out, nil
}

// pgValueToRT converts a value returned by pgx rows.Values() into a BASIC rt.Value.
// Postgres NUMERIC/DECIMAL values decode to rt.Decimal (never float64, Requirement 9.7).
func pgValueToRT(v any) Value {
	switch x := v.(type) {
	case nil:
		return nil
	case bool, int64, float64, string, time.Time, Decimal:
		return x
	case int:
		return int64(x)
	case int8:
		return int64(x)
	case int16:
		return int64(x)
	case int32:
		return int64(x)
	case uint8:
		return int64(x)
	case uint16:
		return int64(x)
	case uint32:
		return int64(x)
	case uint64:
		if x <= math.MaxInt64 {
			return int64(x)
		}
		return float64(x)
	case float32:
		return float64(x)
	case []byte:
		return string(x)
	case [16]byte:
		return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", x[0:4], x[4:6], x[6:8], x[8:10], x[10:16])
	case pgtype.Numeric:
		return pgNumericToDecimal(x)
	case pgtype.Text:
		if !x.Valid {
			return nil
		}
		return x.String
	case pgtype.Int8:
		if !x.Valid {
			return nil
		}
		return x.Int64
	case pgtype.Int4:
		if !x.Valid {
			return nil
		}
		return int64(x.Int32)
	case pgtype.Int2:
		if !x.Valid {
			return nil
		}
		return int64(x.Int16)
	case pgtype.Bool:
		if !x.Valid {
			return nil
		}
		return x.Bool
	case pgtype.Float8:
		if !x.Valid {
			return nil
		}
		return x.Float64
	case pgtype.Float4:
		if !x.Valid {
			return nil
		}
		return float64(x.Float32)
	case pgtype.Timestamp:
		if !x.Valid {
			return nil
		}
		return x.Time
	case pgtype.Timestamptz:
		if !x.Valid {
			return nil
		}
		return x.Time
	case pgtype.Date:
		if !x.Valid {
			return nil
		}
		return x.Time
	case map[string]any:
		m := NewMap()
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			m.Set(k, pgValueToRT(x[k]))
		}
		return m
	case []any:
		arr := ArrayOf()
		arr.Items = make([]Value, len(x))
		for i, it := range x {
			arr.Items[i] = pgValueToRT(it)
		}
		return arr
	default:
		return x
	}
}

func pgNumericToDecimal(n pgtype.Numeric) Value {
	if !n.Valid {
		return nil
	}
	if n.NaN {
		return math.NaN()
	}
	switch n.InfinityModifier {
	case pgtype.Infinity:
		return math.Inf(1)
	case pgtype.NegativeInfinity:
		return math.Inf(-1)
	}
	if n.Int == nil {
		return decimal.New(0, n.Exp)
	}
	return decimal.NewFromBigInt(new(big.Int).Set(n.Int), n.Exp)
}

// ---- SQLite driver (database/sql + modernc.org/sqlite) ----

type sqlQuerier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

type sqliteDB struct {
	db        *sql.DB
	closeOnce sync.Once
	closeErr  error
}

// parseSQLiteDSN strips the sqlite:// scheme and normalizes the DSN for
// modernc.org/sqlite. It returns the DSN and whether it is an in-memory DB.
func parseSQLiteDSN(rawURL string) (dsn string, inMemory bool, err error) {
	if len(rawURL) < len("sqlite://") || !strings.EqualFold(rawURL[:len("sqlite://")], "sqlite://") {
		return "", false, fmt.Errorf("expected sqlite:// URL")
	}
	rest := rawURL[len("sqlite://"):]
	if rest == "" {
		return "", false, fmt.Errorf("missing path after sqlite:// — use sqlite://:memory: or sqlite://./app.db")
	}
	if rest == ":memory:" || strings.HasPrefix(rest, ":memory:?") {
		return rest, true, nil
	}
	// Handle Windows absolute file URIs like sqlite:///C:/path/to/db.sqlite.
	if len(rest) >= 3 && rest[0] == '/' && isASCIIAlpha(rest[1]) && rest[2] == ':' {
		rest = rest[1:]
	}
	return rest, false, nil
}

func isASCIIAlpha(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

func openSQLite(ctx context.Context, rawURL string) (DB, error) {
	dsn, inMemory, err := parseSQLiteDSN(rawURL)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if inMemory {
		// In-memory SQLite creates a distinct empty DB per connection, so pin
		// to one connection to share state across queries and transactions.
		db.SetMaxOpenConns(1)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, err
	}
	// Best-effort pragmas for foreign key enforcement and lock retries.
	_, _ = db.ExecContext(ctx, "PRAGMA foreign_keys = ON")
	_, _ = db.ExecContext(ctx, "PRAGMA busy_timeout = 5000")
	return &sqliteDB{db: db}, nil
}

func (d *sqliteDB) Query(c *Ctx, query string, args []Value) (*Array, error) {
	return sqliteQuery(ctxOf(c), d.db, query, args)
}

func (d *sqliteDB) Exec(c *Ctx, query string, args []Value) (int64, error) {
	return sqliteExec(ctxOf(c), d.db, query, args)
}

func (d *sqliteDB) Begin(c *Ctx) (dbTx, error) {
	tx, err := d.db.BeginTx(ctxOf(c), nil)
	if err != nil {
		return nil, err
	}
	return &sqliteTx{tx: tx}, nil
}

func (d *sqliteDB) Close() error {
	d.closeOnce.Do(func() {
		d.closeErr = d.db.Close()
	})
	return d.closeErr
}

type sqliteTx struct {
	tx      *sql.Tx
	spDepth int
	spName  string // non-empty for nested savepoint transactions
}

func (t *sqliteTx) Query(c *Ctx, query string, args []Value) (*Array, error) {
	return sqliteQuery(ctxOf(c), t.tx, query, args)
}

func (t *sqliteTx) Exec(c *Ctx, query string, args []Value) (int64, error) {
	return sqliteExec(ctxOf(c), t.tx, query, args)
}

func (t *sqliteTx) Begin(c *Ctx) (dbTx, error) {
	nextDepth := t.spDepth + 1
	spName := fmt.Sprintf("rip_sp_%d", nextDepth)
	if _, err := t.tx.ExecContext(ctxOf(c), "SAVEPOINT "+spName); err != nil {
		return nil, err
	}
	return &sqliteTx{tx: t.tx, spDepth: nextDepth, spName: spName}, nil
}

func (t *sqliteTx) Commit(c *Ctx) error {
	if t.spName != "" {
		_, err := t.tx.ExecContext(ctxOf(c), "RELEASE SAVEPOINT "+t.spName)
		return err
	}
	return t.tx.Commit()
}

func (t *sqliteTx) Rollback(_ *Ctx) error {
	if t.spName != "" {
		ctx := rollbackCtx()
		_, err := t.tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT "+t.spName)
		_, _ = t.tx.ExecContext(ctx, "RELEASE SAVEPOINT "+t.spName)
		return err
	}
	return t.tx.Rollback()
}

func sqliteQuery(ctx context.Context, q sqlQuerier, query string, args []Value) (*Array, error) {
	rewritten := rewritePlaceholders(query)
	sqlArgs, err := toSQLiteArgs(args)
	if err != nil {
		return nil, err
	}
	rows, err := q.QueryContext(ctx, rewritten, sqlArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	colTypes, err := rows.ColumnTypes()
	if err != nil {
		return nil, err
	}
	type colMeta struct {
		isDec    bool
		scale    int32
		hasScale bool
	}
	metas := make([]colMeta, len(cols))
	for i := range cols {
		if i < len(colTypes) {
			isDec, scale, hasScale := parseDecimalColumnType(colTypes[i].DatabaseTypeName())
			metas[i] = colMeta{isDec: isDec, scale: scale, hasScale: hasScale}
		}
	}

	out := ArrayOf()
	scanDest := make([]any, len(cols))
	scanPtrs := make([]any, len(cols))
	for i := range cols {
		scanPtrs[i] = &scanDest[i]
	}

	for rows.Next() {
		for i := range scanDest {
			scanDest[i] = nil
		}
		if err := rows.Scan(scanPtrs...); err != nil {
			return nil, err
		}
		row := NewMap()
		for i, col := range cols {
			m := metas[i]
			row.Set(col, sqliteValueToRT(scanDest[i], m.isDec, m.scale, m.hasScale))
		}
		out.Items = append(out.Items, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func sqliteExec(ctx context.Context, q sqlQuerier, query string, args []Value) (int64, error) {
	rewritten := rewritePlaceholders(query)
	sqlArgs, err := toSQLiteArgs(args)
	if err != nil {
		return 0, err
	}
	res, err := q.ExecContext(ctx, rewritten, sqlArgs...)
	if err != nil {
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, nil
	}
	return n, nil
}

func toSQLiteArgs(args []Value) ([]any, error) {
	if len(args) == 0 {
		return nil, nil
	}
	out := make([]any, len(args))
	for i, a := range args {
		switch x := a.(type) {
		case Decimal:
			// Store DECIMALs as exact scale-preserving TEXT (design §4.9).
			out[i] = formatDecimal(x)
		case *Map, *Array:
			b, err := marshalValue(x)
			if err != nil {
				return nil, err
			}
			out[i] = string(b)
		default:
			out[i] = a
		}
	}
	return out, nil
}

func sqliteValueToRT(v any, isDec bool, scale int32, hasScale bool) Value {
	if v == nil {
		return nil
	}
	if isDec {
		switch x := v.(type) {
		case Decimal:
			return applyDecimalScale(x, scale, hasScale)
		case string:
			if d, err := decimal.NewFromString(strings.TrimSpace(x)); err == nil {
				return applyDecimalScale(d, scale, hasScale)
			}
			return x
		case []byte:
			s := strings.TrimSpace(string(x))
			if d, err := decimal.NewFromString(s); err == nil {
				return applyDecimalScale(d, scale, hasScale)
			}
			return string(x)
		case int64:
			return applyDecimalScale(decimal.NewFromInt(x), scale, hasScale)
		case int32:
			return applyDecimalScale(decimal.NewFromInt(int64(x)), scale, hasScale)
		case int:
			return applyDecimalScale(decimal.NewFromInt(int64(x)), scale, hasScale)
		case float64:
			return applyDecimalScale(decimal.NewFromFloat(x), scale, hasScale)
		case float32:
			return applyDecimalScale(decimal.NewFromFloat(float64(x)), scale, hasScale)
		}
	}
	switch x := v.(type) {
	case bool, int64, float64, string, time.Time, Decimal:
		return x
	case int:
		return int64(x)
	case int32:
		return int64(x)
	case float32:
		return float64(x)
	case []byte:
		return string(x)
	default:
		return x
	}
}

// parseDecimalColumnType reports whether a SQLite DatabaseTypeName represents
// a NUMERIC or DECIMAL column, and extracts the declared scale when present
// (e.g. "NUMERIC(12,2)" → true, 2, true).
func parseDecimalColumnType(dbType string) (isDec bool, scale int32, hasScale bool) {
	upper := strings.ToUpper(strings.TrimSpace(dbType))
	if upper == "" {
		return false, 0, false
	}
	base := upper
	params := ""
	if i := strings.IndexByte(upper, '('); i >= 0 {
		base = strings.TrimSpace(upper[:i])
		if j := strings.IndexByte(upper[i+1:], ')'); j >= 0 {
			params = upper[i+1 : i+1+j]
		}
	}
	if base != "NUMERIC" && base != "DECIMAL" {
		return false, 0, false
	}
	if params != "" {
		parts := strings.Split(params, ",")
		if len(parts) == 2 {
			if n, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil && n >= 0 && n <= int(maxDecimalScale) {
				return true, int32(n), true
			}
		}
	}
	return true, 0, false
}

func applyDecimalScale(d Decimal, scale int32, hasScale bool) Decimal {
	if !hasScale || scale <= 0 {
		return d
	}
	if d.Exponent() > -scale {
		if padded, err := decimal.NewFromString(d.StringFixed(scale)); err == nil {
			return padded
		}
	}
	return d
}

// rewritePlaceholders rewrites Postgres-style positional placeholders ($1, $2, …)
// to SQLite numbered placeholders (?1, ?2, …), skipping single-quoted string
// literals, quoted identifiers ("…", `…`, […]), line comments (-- …), and
// block comments (/* … */).
func rewritePlaceholders(query string) string {
	if !strings.ContainsRune(query, '$') {
		return query
	}
	var b strings.Builder
	b.Grow(len(query))
	n := len(query)
	for i := 0; i < n; {
		ch := query[i]
		switch ch {
		case '\'', '"', '`':
			// Quoted string or identifier with doubled-quote escaping.
			quote := ch
			b.WriteByte(ch)
			i++
			for i < n {
				c := query[i]
				b.WriteByte(c)
				i++
				if c == quote {
					if i < n && query[i] == quote {
						b.WriteByte(query[i])
						i++
						continue
					}
					break
				}
			}
		case '[':
			// Bracket-quoted identifier in SQLite.
			b.WriteByte(ch)
			i++
			for i < n {
				c := query[i]
				b.WriteByte(c)
				i++
				if c == ']' {
					break
				}
			}
		case '-':
			if i+1 < n && query[i+1] == '-' {
				// Line comment: copy until newline or EOF.
				b.WriteString("--")
				i += 2
				for i < n {
					c := query[i]
					b.WriteByte(c)
					i++
					if c == '\n' {
						break
					}
				}
				continue
			}
			b.WriteByte(ch)
			i++
		case '/':
			if i+1 < n && query[i+1] == '*' {
				// Block comment: copy until */ or EOF.
				b.WriteString("/*")
				i += 2
				for i < n {
					if query[i] == '*' && i+1 < n && query[i+1] == '/' {
						b.WriteString("*/")
						i += 2
						break
					}
					b.WriteByte(query[i])
					i++
				}
				continue
			}
			b.WriteByte(ch)
			i++
		case '$':
			// Rewrite $N (N >= 1) only when not preceded by an identifier character
			// and not immediately followed by an identifier letter or underscore.
			prevIdent := i > 0 && isSQLIdentChar(query[i-1])
			if !prevIdent && i+1 < n && query[i+1] >= '1' && query[i+1] <= '9' {
				j := i + 1
				for j < n && query[j] >= '0' && query[j] <= '9' {
					j++
				}
				if j == n || !isSQLIdentStart(query[j]) {
					b.WriteByte('?')
					b.WriteString(query[i+1 : j])
					i = j
					continue
				}
			}
			b.WriteByte(ch)
			i++
		default:
			b.WriteByte(ch)
			i++
		}
	}
	return b.String()
}

func isSQLIdentStart(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || b == '_'
}

func isSQLIdentChar(b byte) bool {
	return isSQLIdentStart(b) || (b >= '0' && b <= '9')
}
