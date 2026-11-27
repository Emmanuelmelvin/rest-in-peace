package check

import (
	"strings"
	"testing"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/diag"
)

// TestSharedStateWarning covers Requirement 14.4: warn when a module-level
// variable is assigned inside a handler function in a file that serves HTTP.
func TestSharedStateWarning(t *testing.T) {
	cases := []struct {
		name     string
		src      string
		wantWarn bool
		wantLine int
		wantSub  string // substring expected in the hint
	}{
		// ---- must warn ----
		{
			name: "let_assign_module_var_in_handler",
			src: `
counter = 0
ROUTE GET "/count" CALL Handler
FUNCTION Handler(req)
  counter = counter + 1
  RETURN counter
END FUNCTION
`,
			wantWarn: true,
			wantLine: 5,
			wantSub:  "SHARED DIM",
		},
		{
			name: "let_inside_if_inside_handler",
			src: `
visits = 0
ROUTE GET "/" CALL Home
FUNCTION Home(req)
  IF TRUE THEN
    visits = visits + 1
  END IF
  RETURN visits
END FUNCTION
`,
			wantWarn: true,
			wantLine: 6,
			wantSub:  "SHARED DIM",
		},
		{
			name: "let_inside_for_inside_handler",
			src: `
total = 0
ROUTE POST "/tally" CALL Tally
FUNCTION Tally(req)
  FOR i = 1 TO 3
    total = total + i
  NEXT
  RETURN total
END FUNCTION
`,
			wantWarn: true,
			wantLine: 6,
			wantSub:  "SHARED DIM",
		},
		{
			name: "let_inside_while_inside_handler",
			src: `
state = 0
ROUTE GET "/loop" CALL RunLoop
FUNCTION RunLoop(req)
  WHILE FALSE
    state = 1
  WEND
  RETURN state
END FUNCTION
`,
			wantWarn: true,
			wantLine: 6,
			wantSub:  "SHARED DIM",
		},
		{
			name: "let_inside_try_inside_handler",
			src: `
flag = 0
ROUTE GET "/t" CALL T
FUNCTION T(req)
  TRY
    flag = 1
  CATCH e
  END TRY
  RETURN flag
END FUNCTION
`,
			wantWarn: true,
			wantLine: 6,
			wantSub:  "SHARED DIM",
		},
		{
			name: "sub_handler_warns_too",
			src: `
count = 0
ROUTE POST "/inc" CALL Inc
SUB Inc(req)
  count = count + 1
END SUB
`,
			wantWarn: true,
			wantLine: 5,
			wantSub:  "SHARED DIM",
		},
		{
			name: "map_index_mutation_warns",
			src: `
cache = {}
ROUTE POST "/set" CALL SetCache
FUNCTION SetCache(req)
  cache["k"] = "v"
  RETURN cache
END FUNCTION
`,
			wantWarn: true,
			wantLine: 5,
			wantSub:  "SHARED DIM",
		},
		{
			name: "map_member_mutation_warns",
			src: `
cache = {}
ROUTE POST "/set" CALL SetCache
FUNCTION SetCache(req)
  cache.last = "v"
  RETURN cache
END FUNCTION
`,
			wantWarn: true,
			wantLine: 5,
			wantSub:  "SHARED DIM",
		},
		{
			name: "helper_function_called_from_handler_warns",
			src: `
hits = 0
ROUTE GET "/h" CALL Handle
FUNCTION Handle(req)
  Bump()
  RETURN hits
END FUNCTION
SUB Bump()
  hits = hits + 1
END SUB
`,
			wantWarn: true,
			wantLine: 9,
			wantSub:  "SHARED DIM",
		},
		{
			name: "route_group_warns",
			src: `
hits = 0
ROUTE GROUP "/api"
  ROUTE GET "/h" CALL Handle
END GROUP
FUNCTION Handle(req)
  hits = hits + 1
  RETURN hits
END FUNCTION
`,
			wantWarn: true,
			wantLine: 7,
			wantSub:  "SHARED DIM",
		},
		{
			name: "use_middleware_warns",
			src: `
hits = 0
USE Track
SUB Track(req, nxt)
  hits = hits + 1
  nxt()
END SUB
`,
			wantWarn: true,
			wantLine: 5,
			wantSub:  "SHARED DIM",
		},
		// ---- must NOT warn ----
		{
			name: "shared_dim_suppresses_warning",
			src: `
SHARED DIM counter AS INTEGER
ROUTE GET "/count" CALL Handler
FUNCTION Handler(req)
  counter = counter + 1
  RETURN counter
END FUNCTION
`,
			wantWarn: false,
		},
		{
			name: "shared_dim_map_mutation_suppresses_warning",
			src: `
SHARED DIM cache AS MAP = {}
ROUTE POST "/set" CALL SetCache
FUNCTION SetCache(req)
  cache["k"] = "v"
  cache.last = "v"
  RETURN cache
END FUNCTION
`,
			wantWarn: false,
		},
		{
			name: "local_let_inside_function",
			src: `
ROUTE GET "/" CALL Home
FUNCTION Home(req)
  local = 1
  RETURN local
END FUNCTION
`,
			// 'local' is not in modVars (never assigned at module level), so no warn.
			wantWarn: false,
		},
		{
			name: "assignment_at_top_level_not_in_function",
			src: `
counter = 0
counter = counter + 1
ROUTE GET "/c" CALL Get
FUNCTION Get(req)
  RETURN counter
END FUNCTION
`,
			// Top-level assignment is fine; reading it is fine; only assigning
			// inside the function should warn, and here the function only reads.
			wantWarn: false,
		},
		{
			name: "no_routes_no_warning",
			src: `
state = 0
FUNCTION Bump()
  state = state + 1
END FUNCTION
`,
			// File has no ROUTE/USE — not a server file, no warning.
			wantWarn: false,
		},
		{
			name:     "classic_mode_no_warning",
			src:      "10 LET X = 0\n20 X = X + 1\n30 END\n",
			wantWarn: false,
		},
		{
			name: "dim_inside_function_is_local",
			src: `
ROUTE GET "/" CALL H
FUNCTION H(req)
  DIM x AS INTEGER = 0
  x = x + 1
  RETURN x
END FUNCTION
`,
			wantWarn: false,
		},
		{
			name: "dim_inside_function_shadowing_module_var",
			src: `
x = 0
ROUTE GET "/" CALL H
FUNCTION H(req)
  DIM x AS INTEGER = 1
  x = x + 1
  RETURN x
END FUNCTION
`,
			wantWarn: false,
		},
		{
			name: "for_loop_var_shadowing_module_var",
			src: `
i = 0
ROUTE GET "/" CALL H
FUNCTION H(req)
  FOR i = 1 TO 3
    i = i + 0
  NEXT
  RETURN 1
END FUNCTION
`,
			wantWarn: false,
		},
		{
			name: "param_name_shadowing_module_var",
			src: `
counter = 0
ROUTE POST "/set" CALL Set
FUNCTION Set(counter)
  counter = counter + 1
  RETURN counter
END FUNCTION
`,
			// 'counter' is a parameter here — it's local, not the module var.
			wantWarn: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := tc.src
			if !strings.HasSuffix(src, "\n") {
				src += "\n"
			}
			ds := checkSrc(t, tc.name+".bas", src)

			var warnings []diag.Diagnostic
			for _, d := range ds {
				if d.Kind == diag.SharedState {
					warnings = append(warnings, d)
				}
			}

			if tc.wantWarn {
				if len(warnings) == 0 {
					t.Fatalf("expected SharedState warning, got none\nall diags: %v", ds)
				}
				found := false
				for _, w := range warnings {
					if w.Pos.Line == tc.wantLine {
						found = true
						if tc.wantSub != "" && !strings.Contains(w.Message, tc.wantSub) {
							t.Errorf("warning on line %d: message %q missing %q", tc.wantLine, w.Message, tc.wantSub)
						}
						break
					}
				}
				if !found {
					t.Errorf("expected warning on line %d, got warnings at: %v", tc.wantLine, lineNumbers(warnings))
				}
			} else {
				if len(warnings) > 0 {
					t.Errorf("expected no SharedState warning, got: %v", warnings)
				}
			}

			// Warnings must never make compilation fail.
			for _, d := range ds {
				if d.Kind == diag.SharedState && d.Severity == diag.Error {
					t.Errorf("SharedState emitted as an error: %v", d)
				}
			}
		})
	}
}

// TestSharedStateWarningFormat verifies the exact string format.
func TestSharedStateWarningFormat(t *testing.T) {
	src := `
count = 0
ROUTE GET "/c" CALL Get
FUNCTION Get(req)
  count = count + 1
  RETURN count
END FUNCTION
`
	ds := checkSrc(t, "fmt.bas", src)
	var w *diag.Diagnostic
	for i := range ds {
		if ds[i].Kind == diag.SharedState {
			w = &ds[i]
			break
		}
	}
	if w == nil {
		t.Fatal("expected SharedState warning, got none")
	}
	got := w.String()
	if !strings.HasPrefix(got, "?SHARED STATE WARNING IN fmt.bas:") {
		t.Errorf("unexpected format: %q", got)
	}
	if w.Severity != diag.Warning {
		t.Errorf("expected Warning severity, got %v", w.Severity)
	}
	if !strings.Contains(w.Message, "SHARED DIM") {
		t.Errorf("hint %q does not mention SHARED DIM", w.Message)
	}
}

func lineNumbers(ds []diag.Diagnostic) []int {
	lines := make([]int, len(ds))
	for i, d := range ds {
		lines[i] = d.Pos.Line
	}
	return lines
}
