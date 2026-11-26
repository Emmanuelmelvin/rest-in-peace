package check

import (
	"strings"
	"testing"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/diag"
)

// TestSQLInjectionWarning covers Requirement 9.4: warn when SQL/SQLONE/SQLEXEC
// receive a query string built with + or & from request data.
func TestSQLInjectionWarning(t *testing.T) {
	cases := []struct {
		name     string
		src      string
		wantWarn bool // true → expect exactly one InjectionRisk warning
		wantLine int  // only checked when wantWarn is true
	}{
		// ----- structured mode: direct req member concatenation -----
		{
			name: "sql_plus_req_params",
			src: `FUNCTION Search(req)
  LET rows = SQL("SELECT * FROM t WHERE name = '" & req.params.name & "'")
  RETURN rows
END FUNCTION
`,
			wantWarn: true,
			wantLine: 2,
		},
		{
			name: "sql_plus_req_query",
			src: `FUNCTION List(req)
  LET rows = SQL("SELECT * FROM t ORDER BY " + req.query.sort)
  RETURN rows
END FUNCTION
`,
			wantWarn: true,
			wantLine: 2,
		},
		{
			name: "sql_amp_req_body_field",
			src: `FUNCTION Create(req)
  LET rows = SQL("INSERT INTO t (name) VALUES ('" & req.body.name & "')")
  RETURN rows
END FUNCTION
`,
			wantWarn: true,
			wantLine: 2,
		},
		{
			name: "sql_amp_req_headers",
			src: `FUNCTION HeaderSearch(req)
  LET rows = SQL("SELECT * FROM t WHERE token = '" & req.headers.authorization & "'")
  RETURN rows
END FUNCTION
`,
			wantWarn: true,
			wantLine: 2,
		},
		{
			name: "sqlone_plus_req",
			src: `FUNCTION GetOne(req)
  LET row = SQLONE("SELECT * FROM t WHERE id = " + req.params.id)
  RETURN row
END FUNCTION
`,
			wantWarn: true,
			wantLine: 2,
		},
		{
			name: "sqlexec_amp_req",
			src: `FUNCTION Delete(req)
  SQLEXEC("DELETE FROM t WHERE id = " & req.params.id)
  RETURN NIL
END FUNCTION
`,
			wantWarn: true,
			wantLine: 2,
		},
		// ----- structured mode: req index access -----
		{
			name: "sql_req_index_access",
			src: `FUNCTION Search(req)
  LET rows = SQL("SELECT * FROM t WHERE id = " & req.params["id"])
  RETURN rows
END FUNCTION
`,
			wantWarn: true,
			wantLine: 2,
		},
		{
			name: "sql_req_body_index",
			src: `FUNCTION Create(req)
  LET rows = SQL("INSERT INTO t VALUES ('" & req.body["name"] & "')")
  RETURN rows
END FUNCTION
`,
			wantWarn: true,
			wantLine: 2,
		},
		// ----- structured mode: transitive variable assignment -----
		{
			name: "transitive_let",
			src: `FUNCTION Search(req)
  LET name = req.params.name
  LET rows = SQL("SELECT * FROM t WHERE name = '" & name & "'")
  RETURN rows
END FUNCTION
`,
			wantWarn: true,
			wantLine: 3,
		},
		{
			name: "transitive_dim",
			src: `FUNCTION Search(req)
  DIM id AS STRING = req.params.id
  LET rows = SQL("SELECT * FROM t WHERE id = " & id)
  RETURN rows
END FUNCTION
`,
			wantWarn: true,
			wantLine: 3,
		},
		{
			name: "transitive_two_hops",
			src: `FUNCTION Search(req)
  LET a = req.query.q
  LET b = a
  LET rows = SQL("SELECT * FROM t WHERE q = '" & b & "'")
  RETURN rows
END FUNCTION
`,
			wantWarn: true,
			wantLine: 4,
		},
		// ----- structured mode: string built-ins propagate taint -----
		{
			name: "trim_propagates_taint",
			src: `FUNCTION Search(req)
  LET name = TRIM(req.params.name)
  LET rows = SQL("SELECT * FROM t WHERE name = '" & name & "'")
  RETURN rows
END FUNCTION
`,
			wantWarn: true,
			wantLine: 3,
		},
		{
			name: "upper_propagates_taint",
			src: `FUNCTION Search(req)
  LET rows = SQL("SELECT * FROM t WHERE code = '" & UPPER(req.params.code) & "'")
  RETURN rows
END FUNCTION
`,
			wantWarn: true,
			wantLine: 2,
		},
		{
			name: "lower_propagates_taint",
			src: `FUNCTION Search(req)
  LET rows = SQL("SELECT * FROM t WHERE email = '" & LOWER(req.body.email) & "'")
  RETURN rows
END FUNCTION
`,
			wantWarn: true,
			wantLine: 2,
		},
		{
			name: "left_propagates_taint",
			src: `FUNCTION Search(req)
  LET prefix = LEFT(req.params.name, 3)
  LET rows = SQL("SELECT * FROM t WHERE prefix = '" & prefix & "'")
  RETURN rows
END FUNCTION
`,
			wantWarn: true,
			wantLine: 3,
		},
		{
			name: "right_propagates_taint",
			src: `FUNCTION Search(req)
  LET rows = SQL("SELECT * FROM t WHERE sfx = '" & RIGHT(req.params.name, 3) & "'")
  RETURN rows
END FUNCTION
`,
			wantWarn: true,
			wantLine: 2,
		},
		{
			name: "mid_propagates_taint",
			src: `FUNCTION Search(req)
  LET rows = SQL("SELECT * FROM t WHERE sub = '" & MID(req.params.name, 2, 4) & "'")
  RETURN rows
END FUNCTION
`,
			wantWarn: true,
			wantLine: 2,
		},
		{
			name: "str_propagates_taint",
			src: `FUNCTION Search(req)
  LET rows = SQL("SELECT * FROM t WHERE n = " & STR(req.params.n))
  RETURN rows
END FUNCTION
`,
			wantWarn: true,
			wantLine: 2,
		},
		// ----- structured mode: nested concat expression -----
		{
			name: "nested_concat_expression",
			src: `FUNCTION Search(req)
  LET rows = SQL("SELECT * FROM t WHERE a = '" & req.params.a & "' AND b = '" & req.params.b & "'")
  RETURN rows
END FUNCTION
`,
			wantWarn: true,
			wantLine: 2,
		},
		// ----- classic mode: PARAM / QUERY / HEADER / BODY -----
		{
			name:     "classic_param_concat",
			src:      "10 LET ROWS = SQL(\"SELECT * FROM t WHERE id = \" & PARAM(\"id\"))\n20 END\n",
			wantWarn: true,
			wantLine: 1,
		},
		{
			name:     "classic_query_concat",
			src:      "10 LET ROWS = SQL(\"SELECT * FROM t ORDER BY \" + QUERY(\"sort\"))\n20 END\n",
			wantWarn: true,
			wantLine: 1,
		},
		{
			name:     "classic_header_concat",
			src:      "10 LET ROWS = SQLONE(\"SELECT * FROM t WHERE tok = '\" & HEADER(\"X-Token\") & \"'\")\n20 END\n",
			wantWarn: true,
			wantLine: 1,
		},
		{
			name:     "classic_body_concat",
			src:      "10 SQLEXEC(\"INSERT INTO t (v) VALUES ('\" & BODY() & \"')\")\n20 END\n",
			wantWarn: true,
			wantLine: 1,
		},
		{
			name:     "classic_transitive_param",
			src:      "10 LET ID = PARAM(\"id\")\n20 LET ROWS = SQL(\"SELECT * FROM t WHERE id = \" & ID)\n30 END\n",
			wantWarn: true,
			wantLine: 2,
		},
		// ----- safe: parameterized placeholders -----
		{
			name: "safe_placeholder_structured",
			src: `FUNCTION GetUser(req)
  LET row = SQL("SELECT * FROM t WHERE id = $1", req.params.id)
  RETURN row
END FUNCTION
`,
			wantWarn: false,
		},
		{
			name: "safe_placeholder_sqlone",
			src: `FUNCTION GetUser(req)
  LET row = SQLONE("SELECT * FROM t WHERE id = $1 AND name = $2", req.params.id, req.params.name)
  RETURN row
END FUNCTION
`,
			wantWarn: false,
		},
		{
			name: "safe_placeholder_sqlexec",
			src: `FUNCTION Delete(req)
  SQLEXEC("DELETE FROM t WHERE id = $1", req.params.id)
  RETURN NIL
END FUNCTION
`,
			wantWarn: false,
		},
		{
			name:     "safe_placeholder_classic",
			src:      "10 LET ROWS = SQL(\"SELECT * FROM t WHERE id = $1\", PARAM(\"id\"))\n20 END\n",
			wantWarn: false,
		},
		// ----- safe: literal-only concatenation -----
		{
			name: "safe_literal_concat",
			src: `FUNCTION GetAll(req)
  LET table = "invoices"
  LET rows = SQL("SELECT * FROM " & table)
  RETURN rows
END FUNCTION
`,
			wantWarn: false,
		},
		{
			name: "safe_literal_plus_concat",
			src: `FUNCTION GetAll(req)
  LET rows = SQL("SELECT * FROM " + "invoices" + " WHERE active = TRUE")
  RETURN rows
END FUNCTION
`,
			wantWarn: false,
		},
		// ----- safe: const concatenation -----
		{
			name: "safe_const_concat",
			src: `CONST TABLE = "invoices"
FUNCTION GetAll(req)
  LET rows = SQL("SELECT * FROM " & TABLE & " ORDER BY id")
  RETURN rows
END FUNCTION
`,
			wantWarn: false,
		},
		// ----- safe: req used only as parameter, not in query -----
		{
			name: "safe_req_as_sql_param",
			src: `FUNCTION GetUser(req)
  LET row = SQL("SELECT * FROM users WHERE id = $1", req.params.id)
  RETURN row
END FUNCTION
`,
			wantWarn: false,
		},
		// ----- safe: tainted variable used as SQL parameter (not in query) -----
		{
			name: "safe_tainted_var_as_sql_param",
			src: `FUNCTION GetUser(req)
  LET id = req.params.id
  LET row = SQL("SELECT * FROM users WHERE id = $1", id)
  RETURN row
END FUNCTION
`,
			wantWarn: false,
		},
		// ----- safe: req not involved in SQL at all -----
		{
			name: "safe_no_sql",
			src: `FUNCTION Hello(req)
  RETURN "hello, " & req.params.name
END FUNCTION
`,
			wantWarn: false,
		},
		// ----- safe: SQL called without arguments -----
		{
			name: "safe_sql_no_args",
			src: `FUNCTION All(req)
  LET rows = SQL("SELECT 1")
  RETURN rows
END FUNCTION
`,
			wantWarn: false,
		},
		// ----- safe: req tainted but only concatenated in a non-SQL context -----
		{
			name: "safe_concat_in_non_sql",
			src: `FUNCTION Greet(req)
  LET msg = "Hello, " & req.params.name
  RETURN msg
END FUNCTION
`,
			wantWarn: false,
		},
		// ----- safe: SQL query is a plain variable (not a concat) -----
		{
			name: "safe_query_in_variable",
			src: `FUNCTION All(req)
  LET q = "SELECT * FROM t"
  LET rows = SQL(q)
  RETURN rows
END FUNCTION
`,
			wantWarn: false,
		},
		// ----- safe: tainted variable as the entire query (not concat) -----
		{
			name: "safe_tainted_var_as_whole_query",
			src: `FUNCTION Dynamic(req)
  LET q = req.params.query
  LET rows = SQL(q)
  RETURN rows
END FUNCTION
`,
			// A bare tainted variable as the full query is not a concatenation,
			// so we don't warn. The user has presumably put a full query there.
			wantWarn: false,
		},
		// ----- structured mode: handler with explicit req parameter name -----
		{
			name: "explicit_req_param_name",
			src: `FUNCTION Handle(request)
  LET rows = SQL("SELECT * FROM t WHERE id = " + request.params.id)
  RETURN rows
END FUNCTION
`,
			wantWarn: true,
			wantLine: 2,
		},
		// ----- structured mode: req taint through nested function call -----
		{
			name:     "classic_sqlone_param",
			src:      "10 LET ROW = SQLONE(\"SELECT * FROM t WHERE id = $1\", PARAM(\"id\"))\n20 END\n",
			wantWarn: false,
		},
		// ----- multiple SQL calls: only tainted one warns -----
		{
			name: "mixed_safe_and_unsafe",
			src: `FUNCTION Mixed(req)
  LET safe = SQL("SELECT * FROM t WHERE id = $1", req.params.id)
  LET unsafe = SQL("SELECT * FROM t WHERE name = '" & req.params.name & "'")
  RETURN safe
END FUNCTION
`,
			wantWarn: true,
			wantLine: 3,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ds := checkSrc(t, tc.name+".bas", tc.src)

			var warnings []diag.Diagnostic
			for _, d := range ds {
				if d.Kind == diag.InjectionRisk {
					warnings = append(warnings, d)
				}
			}

			if tc.wantWarn {
				if len(warnings) == 0 {
					t.Fatalf("expected InjectionRisk warning, got none (all diags: %v)", ds)
				}
				found := false
				for _, w := range warnings {
					if w.Pos.Line == tc.wantLine {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("expected warning on line %d, got warnings: %v", tc.wantLine, warnings)
				}
				// Verify the warning message contains the required hint.
				for _, w := range warnings {
					if w.Pos.Line == tc.wantLine && !strings.Contains(w.Message, "use $1 placeholders") {
						t.Errorf("warning message %q missing 'use $1 placeholders'", w.Message)
					}
				}
			} else {
				if len(warnings) > 0 {
					t.Errorf("expected no InjectionRisk warning, got: %v", warnings)
				}
			}

			// Warnings must never make compilation fail.
			if ds.HasErrors() {
				// Only fail if the errors are not from check issues unrelated to injection.
				var injErrors []diag.Diagnostic
				for _, d := range ds {
					if d.Kind == diag.InjectionRisk && d.Severity == diag.Error {
						injErrors = append(injErrors, d)
					}
				}
				if len(injErrors) > 0 {
					t.Errorf("injection check emitted errors (not warnings): %v", injErrors)
				}
			}
		})
	}
}

// TestSQLInjectionWarningFormat verifies the exact string format of the warning.
func TestSQLInjectionWarningFormat(t *testing.T) {
	src := `FUNCTION Search(req)
  LET rows = SQL("SELECT * FROM t WHERE id = " & req.params.id)
  RETURN rows
END FUNCTION
`
	ds := checkSrc(t, "fmt_test.bas", src)
	var w *diag.Diagnostic
	for i := range ds {
		if ds[i].Kind == diag.InjectionRisk {
			w = &ds[i]
			break
		}
	}
	if w == nil {
		t.Fatal("expected InjectionRisk warning, got none")
	}
	got := w.String()
	want := "?INJECTION RISK WARNING IN fmt_test.bas:2 — use $1 placeholders"
	if got != want {
		t.Errorf("warning format:\n  got:  %q\n  want: %q", got, want)
	}
	if w.Severity != diag.Warning {
		t.Errorf("expected Warning severity, got %v", w.Severity)
	}
}

// TestSQLInjectionNoErrors verifies that injection warnings never set HasErrors.
func TestSQLInjectionNoErrors(t *testing.T) {
	srcs := []string{
		// structured
		`FUNCTION F(req)
  LET rows = SQL("SELECT * FROM t WHERE id = " & req.params.id)
  RETURN rows
END FUNCTION
`,
		// classic
		"10 LET ROWS = SQL(\"SELECT * FROM t WHERE id = \" & PARAM(\"id\"))\n20 END\n",
	}
	for _, src := range srcs {
		ds := checkSrc(t, "noerr.bas", src)
		if ds.HasErrors() {
			var errs []diag.Diagnostic
			for _, d := range ds {
				if d.Severity == diag.Error {
					errs = append(errs, d)
				}
			}
			t.Errorf("HasErrors() = true; errors: %v", errs)
		}
	}
}
