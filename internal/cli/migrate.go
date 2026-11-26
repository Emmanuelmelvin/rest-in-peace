package cli

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"

	_ "modernc.org/sqlite" // register "sqlite" driver
)

// runMigrate implements `rip migrate` (Requirement 9.6).
func runMigrate(args []string) int {
	// Resolve the project root the same way other commands do, but tolerate a
	// missing main.bas so `rip migrate` works in projects that have no entry
	// file yet (e.g. a pure-migrations bootstrap).
	var root string
	p, _, err := resolve(args)
	if err != nil {
		cwd, cwdErr := filepath.Abs(".")
		if cwdErr != nil {
			return fail(err)
		}
		root = cwd
	} else {
		root = p.Root
	}

	// Load .env (dev-only, same as rip run).
	for _, kv := range loadDotEnv(filepath.Join(root, ".env")) {
		k, v, _ := strings.Cut(kv, "=")
		if err := os.Setenv(k, v); err != nil {
			return fail(fmt.Errorf("?CONFIG ERROR — could not apply .env key %s: %v", k, err))
		}
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		fmt.Fprintln(os.Stderr, "?CONFIG ERROR — DATABASE_URL is not set; set it in .env or the environment")
		return 1
	}

	migrationsDir := filepath.Join(root, "migrations")
	files, err := collectMigrations(migrationsDir)
	if err != nil {
		return fail(err)
	}
	if len(files) == 0 {
		fmt.Println("No .sql files in migrations/ — nothing to do.")
		return 0
	}

	switch {
	case strings.HasPrefix(dbURL, "postgres://"), strings.HasPrefix(dbURL, "postgresql://"):
		return migratePostgres(dbURL, migrationsDir, files)
	case strings.HasPrefix(dbURL, "sqlite://"):
		return migrateSQLite(dbURL, migrationsDir, files)
	default:
		fmt.Fprintf(os.Stderr, "?CONFIG ERROR — unsupported DATABASE_URL scheme (got %q); use postgres:// or sqlite://\n", dbURL)
		return 1
	}
}

// collectMigrations finds *.sql files inside dir, sorted by filename.
// Returns nil, nil when dir does not exist or contains no .sql files.
func collectMigrations(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("?FILE NOT FOUND ERROR — cannot read migrations directory %s: %v", dir, err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".sql") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// ── Postgres ──────────────────────────────────────────────────────────────────

func migratePostgres(dbURL, dir string, files []string) int {
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "?CONFIG ERROR — cannot connect to Postgres: %v\n", err)
		return 1
	}
	defer conn.Close(ctx)

	const createTable = `
CREATE TABLE IF NOT EXISTS _rip_migrations (
	name       TEXT        PRIMARY KEY,
	applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
)`
	if _, err := conn.Exec(ctx, createTable); err != nil {
		fmt.Fprintf(os.Stderr, "?MIGRATE ERROR — cannot create _rip_migrations table: %v\n", err)
		return 1
	}

	applied, err := pgxApplied(ctx, conn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "?MIGRATE ERROR — cannot read _rip_migrations: %v\n", err)
		return 1
	}

	for _, name := range files {
		if applied[name] {
			fmt.Printf("  SKIP   %s (already applied)\n", name)
			continue
		}
		if err := pgxApplyOne(ctx, conn, dir, name); err != nil {
			fmt.Fprintf(os.Stderr, "?MIGRATE ERROR IN %s — %v\n", name, err)
			return 1
		}
		fmt.Printf("  APPLY  %s\n", name)
	}
	fmt.Println("READY.")
	return 0
}

func pgxApplied(ctx context.Context, conn *pgx.Conn) (map[string]bool, error) {
	rows, err := conn.Query(ctx, `SELECT name FROM _rip_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out[name] = true
	}
	return out, rows.Err()
}

func pgxApplyOne(ctx context.Context, conn *pgx.Conn, dir, name string) error {
	src, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return fmt.Errorf("cannot read file: %w", err)
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("cannot begin transaction: %w", err)
	}
	if _, err := tx.Exec(ctx, string(src)); err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("migration SQL failed: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO _rip_migrations (name) VALUES ($1)`, name); err != nil {
		_ = tx.Rollback(ctx)
		return fmt.Errorf("cannot record migration: %w", err)
	}
	return tx.Commit(ctx)
}

// ── SQLite ────────────────────────────────────────────────────────────────────

// sqliteDSN converts a sqlite:// URL to a DSN for database/sql + modernc.org/sqlite.
//
//	sqlite://:memory:        → :memory:
//	sqlite:///abs/path.db    → /abs/path.db
//	sqlite://./rel.db        → ./rel.db
//	sqlite://rel.db          → rel.db
func sqliteDSN(rawURL string) string {
	rest := strings.TrimPrefix(rawURL, "sqlite://")
	// All other forms pass through as-is; sqlite:///abs → /abs because we
	// stripped "sqlite://" so "rest" is "/abs".
	return rest
}

func migrateSQLite(dbURL, dir string, files []string) int {
	dsn := sqliteDSN(dbURL)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "?CONFIG ERROR — cannot open SQLite database %q: %v\n", dsn, err)
		return 1
	}
	defer db.Close()
	db.SetMaxOpenConns(1) // SQLite requires serialised writes

	if err := sqliteEnsureTable(db); err != nil {
		fmt.Fprintf(os.Stderr, "?MIGRATE ERROR — cannot create _rip_migrations table: %v\n", err)
		return 1
	}

	applied, err := sqliteApplied(db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "?MIGRATE ERROR — cannot read _rip_migrations: %v\n", err)
		return 1
	}

	for _, name := range files {
		if applied[name] {
			fmt.Printf("  SKIP   %s (already applied)\n", name)
			continue
		}
		if err := sqliteApplyOne(db, dir, name); err != nil {
			fmt.Fprintf(os.Stderr, "?MIGRATE ERROR IN %s — %v\n", name, err)
			return 1
		}
		fmt.Printf("  APPLY  %s\n", name)
	}
	fmt.Println("READY.")
	return 0
}

func sqliteEnsureTable(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS _rip_migrations (
	name       TEXT     PRIMARY KEY,
	applied_at DATETIME NOT NULL DEFAULT (datetime('now'))
)`)
	return err
}

func sqliteApplied(db *sql.DB) (map[string]bool, error) {
	rows, err := db.Query(`SELECT name FROM _rip_migrations`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out[name] = true
	}
	return out, rows.Err()
}

func sqliteApplyOne(db *sql.DB, dir, name string) error {
	src, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return fmt.Errorf("cannot read file: %w", err)
	}
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("cannot begin transaction: %w", err)
	}
	// Split the script into individual statements because modernc.org/sqlite's
	// database/sql driver does not support multi-statement Exec in one call.
	for _, stmt := range splitSQL(string(src)) {
		if _, err := tx.Exec(stmt); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("migration SQL failed: %w", err)
		}
	}
	if _, err := tx.Exec(`INSERT INTO _rip_migrations (name) VALUES (?)`, name); err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("cannot record migration: %w", err)
	}
	return tx.Commit()
}

// splitSQL splits a SQL script into individual non-empty statements.
// It understands "--" line comments and splits on ";". This covers DDL
// migrations produced by rip and common third-party migration files.
func splitSQL(src string) []string {
	var out []string
	var buf strings.Builder
	for _, line := range strings.Split(src, "\n") {
		// Drop line comments before checking for semicolons.
		bare := line
		if i := strings.Index(line, "--"); i >= 0 {
			bare = line[:i]
		}
		if strings.TrimSpace(bare) == "" {
			continue
		}
		buf.WriteString(line)
		buf.WriteByte('\n')
		if strings.ContainsRune(bare, ';') {
			if s := strings.TrimSpace(buf.String()); s != "" {
				out = append(out, s)
			}
			buf.Reset()
		}
	}
	if s := strings.TrimSpace(buf.String()); s != "" {
		out = append(out, s)
	}
	return out
}
