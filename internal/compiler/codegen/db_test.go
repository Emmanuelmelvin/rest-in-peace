package codegen_test

import (
	"strings"
	"testing"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/codegen"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/diag"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/token"
)

func dbConnect(line int, url ast.Expr, name string) ast.Stmt {
	return &ast.DBConnectStmt{At: at(line), URL: url, Name: strings.ToUpper(name)}
}

func txStmt(line int, name string, body ...ast.Stmt) ast.Stmt {
	return &ast.TransactionStmt{At: at(line), Name: strings.ToUpper(name), Body: body}
}

func TestDBCodegenShape(t *testing.T) {
	t.Run("structured DBCONNECT and TRANSACTION", func(t *testing.T) {
		p := structured("dbshape.bas",
			dbConnect(1, str("sqlite://:memory:"), ""),
			dbConnect(2, call("ENV", str("REPORTS_URL"), str("sqlite://:memory:")), "reports"),
			txStmt(3, "", do(4, call("SQLEXEC", str("SELECT 1")))),
			txStmt(6, "reports", do(7, call("SQLEXEC", str("SELECT 2")))),
		)
		files, ds := codegen.Generate(p, codegen.Options{})
		if ds.HasErrors() {
			t.Fatal(ds)
		}
		src := string(files[1].Src)
		for _, want := range []string{
			`rt.DBConnect(c, "sqlite://:memory:", "")`,
			`rt.DBConnect(c, rt.AsStr(rt.Env("REPORTS_URL", "sqlite://:memory:")), "REPORTS")`,
			`rt.Tx(c, "", func() {`,
			`rt.Tx(c, "REPORTS", func() {`,
		} {
			if !strings.Contains(src, want) {
				t.Errorf("generated source missing %q:\n%s", want, src)
			}
		}
	})

	t.Run("classic mode allows DBCONNECT and rejects TRANSACTION", func(t *testing.T) {
		okProg := classic("cdb.bas",
			ln(10, 1, dbConnect(1, str("sqlite://:memory:"), "")),
			ln(20, 2, do(2, call("SQLEXEC", str("CREATE TABLE t (x INT)")))),
			ln(30, 3, end(3)),
		)
		files, ds := codegen.Generate(okProg, codegen.Options{})
		if ds.HasErrors() {
			t.Fatalf("unexpected errors in classic DBCONNECT: %v", ds)
		}
		if !strings.Contains(string(files[1].Src), `rt.DBConnect(c, "sqlite://:memory:", "")`) {
			t.Errorf("classic generated source missing DBConnect:\n%s", files[1].Src)
		}

		badProg := classic("ctx.bas",
			ln(10, 1, txStmt(1, "", do(2, call("SQLEXEC", str("SELECT 1"))))),
		)
		files, ds = codegen.Generate(badProg, codegen.Options{})
		if files != nil || !ds.HasErrors() {
			t.Fatalf("expected error for TRANSACTION in classic mode, got files=%d ds=%v", len(files), ds)
		}
		found := false
		for _, d := range ds {
			if d.Kind == diag.Syntax && strings.Contains(d.Message, "structured mode") {
				found = true
			}
		}
		if !found {
			t.Errorf("expected Syntax diagnostic mentioning structured mode, got: %v", ds)
		}
	})
}

func TestDBEndToEnd(t *testing.T) {
	cases := []outCase{
		{
			name: "SQLite CRUD, DECIMAL round-trip, and LET visibility after TRANSACTION",
			prog: structured("inv.bas",
				dbConnect(1, str("sqlite://:memory:"), ""),
				do(2, call("SQLEXEC", str("CREATE TABLE invoices (id INTEGER PRIMARY KEY AUTOINCREMENT, customer TEXT NOT NULL, total NUMERIC(12,2) NOT NULL)"))),
				pr(3, call("CreateInv", str("Ada"), call("DECIMAL", str("215.00")))),
				function(10, "CreateInv", params(param("cust", "", nil), param("amt", "", nil)), "",
					txStmt(11, "",
						let(12, id("inv"), call("SQLONE", str("INSERT INTO invoices (customer, total) VALUES ($1, $2) RETURNING id, customer, total"), id("cust"), id("amt"))),
					),
					// inv assigned inside TRANSACTION must remain visible after END TRANSACTION.
					ret(15, bin(bin(mem(id("inv"), "customer"), token.Amp, str("=")), token.Amp, mem(id("inv"), "total"))),
				),
			),
			stdout: "Ada=215.00\n",
		},
		{
			name: "TRANSACTION commit, rollback via TRY/CATCH, and helper function",
			prog: structured("tx.bas",
				dbConnect(1, str("sqlite://:memory:"), ""),
				do(2, call("SQLEXEC", str("CREATE TABLE logs (msg TEXT NOT NULL)"))),
				txStmt(3, "",
					do(4, call("WriteLog", str("committed"))),
				),
				tryS(6, stmts(
					txStmt(7, "",
						do(8, call("WriteLog", str("rolled-back"))),
						throw(9, call("ERROR", str("boom"))),
					),
				), "e", stmts(
					pr(11, mem(id("e"), "kind"), str("@"), mem(id("e"), "line"), str(" "), mem(id("e"), "message")),
				)),
				let(13, id("rows"), call("SQL", str("SELECT msg FROM logs"))),
				pr(14, call("LEN", id("rows")), str(" "), mem(idx(id("rows"), num(0)), "msg")),
				sub(20, "WriteLog", params(param("m", "", nil)),
					do(21, call("SQLEXEC", str("INSERT INTO logs (msg) VALUES ($1)"), id("m"))),
				),
			),
			stdout: "USER@9 boom\n1 committed\n",
		},
		{
			name: "RETURN and EXIT inside TRANSACTION commit and propagate",
			prog: structured("txflow.bas",
				dbConnect(1, str("sqlite://:memory:"), ""),
				do(2, call("SQLEXEC", str("CREATE TABLE nums (n INTEGER NOT NULL)"))),
				pr(3, call("InsertAndReturn", num(42))),
				do(4, call("InsertAndSubReturn", num(7))),
				forS(5, "i", num(1), num(5), nil,
					txStmt(6, "",
						do(7, call("SQLEXEC", str("INSERT INTO nums (n) VALUES ($1)"), id("i"))),
						iff(8, bin(id("i"), token.Eq, num(2)), stmts(exit(8, token.FOR))),
					),
				),
				let(11, id("rows"), call("SQL", str("SELECT n FROM nums ORDER BY rowid"))),
				each(12, "r", id("rows"),
					pr(13, mem(id("r"), "n")),
				),
				function(20, "InsertAndReturn", params(param("v", "INTEGER", nil)), "INTEGER",
					txStmt(21, "",
						do(22, call("SQLEXEC", str("INSERT INTO nums (n) VALUES ($1)"), id("v"))),
						ret(23, bin(id("v"), token.Plus, num(1))),
					),
					ret(25, num(0)),
				),
				sub(30, "InsertAndSubReturn", params(param("v", "INTEGER", nil)),
					txStmt(31, "",
						do(32, call("SQLEXEC", str("INSERT INTO nums (n) VALUES ($1)"), id("v"))),
						ret(33, nil),
					),
					pr(35, str("UNREACHED")),
				),
			),
			stdout: "43\n42\n7\n1\n2\n",
		},
		{
			name: "named connection DBCONNECT AS reports and TRANSACTION ON reports",
			prog: structured("named.bas",
				dbConnect(1, str("sqlite://:memory:"), ""),
				dbConnect(2, str("sqlite://:memory:"), "reports"),
				do(3, call("SQLEXEC", str("CREATE TABLE t (v TEXT)"))),
				do(4, call("SQLEXEC", str("INSERT INTO t (v) VALUES ($1)"), str("main-db"))),
				txStmt(5, "reports",
					do(6, call("SQLEXEC", str("CREATE TABLE r (v TEXT)"))),
					do(7, call("SQLEXEC", str("INSERT INTO r (v) VALUES ($1)"), str("reports-db"))),
					pr(8, mem(call("SQLONE", str("SELECT v FROM r")), "v")),
				),
				pr(10, mem(call("SQLONE", str("SELECT v FROM t")), "v")),
			),
			stdout: "reports-db\nmain-db\n",
		},
		{
			name: "classic mode DBCONNECT and SQL",
			prog: classic("classicdb.bas",
				ln(10, 1, dbConnect(1, str("sqlite://:memory:"), "")),
				ln(20, 2, do(2, call("SQLEXEC", str("CREATE TABLE greetings (msg TEXT)")))),
				ln(30, 3, do(3, call("SQLEXEC", str("INSERT INTO greetings (msg) VALUES ($1)"), str("HELLO FROM 1975")))),
				ln(40, 4, let(4, id("R"), call("SQLONE", str("SELECT msg FROM greetings")))),
				ln(50, 5, pr(5, mem(id("R"), "msg"))),
				ln(60, 6, end(6)),
			),
			stdout: "HELLO FROM 1975\n",
		},
		{
			name: "DBINSERT, DBFIND, DBFINDONE, DBUPDATE, and DBDELETE helpers",
			prog: structured("crud.bas",
				dbConnect(1, str("sqlite://:memory:"), ""),
				do(2, call("SQLEXEC", str("CREATE TABLE tasks (id INTEGER PRIMARY KEY AUTOINCREMENT, title TEXT NOT NULL, done INT NOT NULL)"))),
				let(3, id("t1"), call("DBINSERT", str("tasks"), &ast.MapLit{
					At:     at(3),
					Keys:   []string{"title", "done"},
					Values: []ast.Expr{str("ship"), num(0)},
				})),
				do(4, call("DBINSERT", str("tasks"), &ast.MapLit{
					At:     at(4),
					Keys:   []string{"title", "done"},
					Values: []ast.Expr{str("demo"), num(1)},
				})),
				do(5, call("DBUPDATE", str("tasks"),
					&ast.MapLit{At: at(5), Keys: []string{"done"}, Values: []ast.Expr{num(1)}},
					&ast.MapLit{At: at(5), Keys: []string{"id"}, Values: []ast.Expr{mem(id("t1"), "id")}},
				)),
				let(6, id("all"), call("DBFIND", str("tasks"), &ast.MapLit{At: at(6), Keys: []string{"done"}, Values: []ast.Expr{num(1)}})),
				let(7, id("one"), call("DBFINDONE", str("tasks"), &ast.MapLit{At: at(7), Keys: []string{"id"}, Values: []ast.Expr{mem(id("t1"), "id")}})),
				let(8, id("del"), call("DBDELETE", str("tasks"), &ast.MapLit{At: at(8), Keys: []string{"title"}, Values: []ast.Expr{str("demo")}})),
				pr(9, call("LEN", id("all")), str(" "), mem(id("one"), "title"), str(" "), id("del")),
			),
			stdout: "2 ship 1\n",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			r := runProgram(t, tc.prog)
			if r.exit != 0 {
				t.Fatalf("exit %d, stdout=%q, stderr=%q", r.exit, r.stdout, r.stderr)
			}
			if r.stdout != tc.stdout {
				t.Errorf("stdout:\n%s\nwant:\n%s", r.stdout, tc.stdout)
			}
		})
	}
}
