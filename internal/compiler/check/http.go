package check

import (
	"fmt"
	"strings"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/diag"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/token"
)

// HTTP checks (Requirement 7): where ROUTE and SERVE may go, route paths,
// duplicate routes, and the shape of ROUTE … CALL handlers.

func (c *checker) checkRoute(st *ast.RouteStmt) {
	if c.inFunc != nil {
		c.errorf(st.At, diag.Syntax, "ROUTE goes in top-level code, not inside a FUNCTION or SUB")
	}
	if msg := routePathError(st.Path); msg != "" {
		c.errorf(st.At, diag.Syntax, "%s", msg)
	} else {
		key := strings.ToUpper(st.Method) + " " + routeShape(st.Path)
		if c.routes == nil {
			c.routes = map[string]token.Pos{}
		}
		if prev, dup := c.routes[key]; dup {
			c.errorf(st.At, diag.Syntax, "ROUTE %s %q is already defined at line %d", st.Method, st.Path, prev.Line)
		} else {
			c.routes[key] = st.At
		}
	}

	switch {
	case st.GosubLine != 0:
		if !c.classic {
			c.errorf(st.At, diag.Syntax, "ROUTE … GOSUB is only allowed in classic mode; in structured mode use ROUTE %s %q CALL FunctionName", st.Method, st.Path)
		} else {
			c.checkLineTarget(st.At, st.GosubLine)
		}
	case c.classic:
		c.errorf(st.At, diag.Syntax, "classic mode has no FUNCTIONs, so use ROUTE %s %q GOSUB <line>", st.Method, st.Path)
	case st.Handler != "":
		c.checkHandlerRef(st.At, st.Handler)
		if sym := c.modScope.lookup(st.Handler); sym != nil && sym.fn != nil && len(sym.fn.Params) != 1 {
			c.errorf(st.At, diag.Syntax, "%s handles ROUTE %s %q, so it takes one parameter, the request: FUNCTION %s(req)", sym.fn.Name, st.Method, st.Path, sym.fn.Name)
		}
	}
}

func (c *checker) checkServe(st *ast.ServeStmt) {
	if c.inFunc != nil {
		c.errorf(st.At, diag.Syntax, "SERVE goes in top-level code, not inside a FUNCTION or SUB")
	}
	if st.Port != nil {
		c.checkExpr(st.Port)
	}
}

// routePathError mirrors rt.RoutePattern, so a bad path fails at compile
// time: it starts with /, :name parameters are identifiers, and * only ends it.
func routePathError(path string) string {
	if !strings.HasPrefix(path, "/") {
		return fmt.Sprintf("a route path starts with /, like \"/hello\"; got %q", path)
	}
	segs := strings.Split(path[1:], "/")
	seen := map[string]bool{}
	for i, s := range segs {
		last := i == len(segs)-1
		switch {
		case s == "" && last, s == "*" && last:
		case strings.HasPrefix(s, ":"):
			name := s[1:]
			if !isParamName(name) {
				return fmt.Sprintf("%q is not a valid path parameter; use letters, digits and _, like :id", s)
			}
			if seen[name] || (name == "rest" && segs[len(segs)-1] == "*") {
				return fmt.Sprintf("the path parameter :%s appears twice in %q", name, path)
			}
			seen[name] = true
		case strings.Contains(s, "*"):
			return "* only works as the last part of a route path, like \"/files/*\""
		case strings.ContainsAny(s, "{}"):
			return "a route path cannot contain { or }; write path parameters as :name"
		case s == "":
			return fmt.Sprintf("the route path %q has an empty part (//)", path)
		}
	}
	return ""
}

// routeShape is a path with its parameter names erased, so /a/:id and
// /a/:key count as the same route.
func routeShape(path string) string {
	segs := strings.Split(path, "/")
	for i, s := range segs {
		if strings.HasPrefix(s, ":") {
			segs[i] = ":"
		}
	}
	return strings.Join(segs, "/")
}

func isParamName(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r == '_', r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}
