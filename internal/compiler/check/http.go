package check

import (
	"fmt"
	"strings"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/diag"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/token"
)

// HTTP checks (Requirements 7 and 8): where ROUTE, ROUTE GROUP, USE and SERVE
// may go, route paths, duplicate routes, middleware scopes and handler shapes.

// middlewareBuiltins maps each built-in middleware factory to its [min, max]
// argument count (Requirement 8.3).
var middlewareBuiltins = map[string][2]int{
	"APIKEY":     {1, 1},
	"CORS":       {0, 1},
	"REQUESTLOG": {0, 0},
	"RATELIMIT":  {1, 1},
	"BEARERJWT":  {1, 1},
}

func (c *checker) checkRoute(st *ast.RouteStmt) {
	c.checkRouteWithPath(st, st.Path)
}

func (c *checker) checkRouteWithPath(st *ast.RouteStmt, fullPath string) {
	if c.inFunc != nil {
		c.errorf(st.At, diag.Syntax, "ROUTE goes in top-level code, not inside a FUNCTION or SUB")
	}
	if msg := routePathError(fullPath); msg != "" {
		c.errorf(st.At, diag.Syntax, "%s", msg)
	} else {
		key := strings.ToUpper(st.Method) + " " + routeShape(fullPath)
		if c.routes == nil {
			c.routes = map[string]token.Pos{}
		}
		if prev, dup := c.routes[key]; dup {
			c.errorf(st.At, diag.Syntax, "ROUTE %s %q is already defined at line %d", st.Method, fullPath, prev.Line)
		} else {
			c.routes[key] = st.At
		}
	}

	switch {
	case st.GosubLine != 0:
		if !c.classic {
			c.errorf(st.At, diag.Syntax, "ROUTE … GOSUB is only allowed in classic mode; in structured mode use ROUTE %s %q CALL FunctionName", st.Method, fullPath)
		} else {
			c.checkLineTarget(st.At, st.GosubLine)
		}
	case c.classic:
		c.errorf(st.At, diag.Syntax, "classic mode has no FUNCTIONs, so use ROUTE %s %q GOSUB <line>", st.Method, fullPath)
	case st.Handler != "":
		c.checkHandlerRef(st.At, st.Handler)
		if sym := c.modScope.lookup(st.Handler); sym != nil && sym.fn != nil && len(sym.fn.Params) != 1 {
			c.errorf(st.At, diag.Syntax, "%s handles ROUTE %s %q, so it takes one parameter, the request: FUNCTION %s(req)", sym.fn.Name, st.Method, fullPath, sym.fn.Name)
		}
	}
}

func (c *checker) checkGroup(st *ast.GroupStmt) {
	if c.inFunc != nil {
		c.errorf(st.At, diag.Syntax, "ROUTE GROUP goes in top-level code, not inside a FUNCTION or SUB")
	}
	if msg := groupPrefixError(st.Prefix); msg != "" {
		c.errorf(st.At, diag.Syntax, "%s", msg)
		return
	}
	for _, r := range st.Routes {
		if msg := routePathError(r.Path); msg != "" {
			c.errorf(r.At, diag.Syntax, "%s", msg)
			continue
		}
		c.checkRouteWithPath(r, joinRoutePath(st.Prefix, r.Path))
	}
}

func (c *checker) checkUse(st *ast.UseStmt) {
	if c.classic {
		c.errorf(st.At, diag.Syntax, "USE middleware only works in structured mode; remove the line numbers to use it")
		return
	}
	if c.inFunc != nil {
		c.errorf(st.At, diag.Syntax, "USE goes in top-level code, not inside a FUNCTION or SUB")
	}
	if st.Scope != "" {
		if msg := scopePathError(st.Scope); msg != "" {
			c.errorf(st.At, diag.Syntax, "%s", msg)
		}
	}
	switch fn := st.Fn.(type) {
	case *ast.Ident:
		if _, ok := middlewareBuiltins[fn.Name]; ok {
			c.errorf(fn.At, diag.Syntax, "%s is a built-in middleware; call it with parentheses, like USE %s()", fn.Name, fn.Name)
			return
		}
		c.checkHandlerRef(fn.At, fn.Name)
		if sym := c.modScope.lookup(fn.Name); sym != nil && sym.fn != nil && len(sym.fn.Params) != 1 {
			c.errorf(fn.At, diag.Syntax, "middleware %s takes one parameter, the request: FUNCTION %s(req) or SUB %s(req)", sym.fn.Name, sym.fn.Name, sym.fn.Name)
		}
	case *ast.CallExpr:
		id, ok := fn.Fn.(*ast.Ident)
		if !ok {
			c.errorf(fn.Pos(), diag.Syntax, "USE expects a middleware function name (USE Auth) or built-in middleware (USE CORS(), USE APIKEY(\"KEY\"))")
			return
		}
		arity, ok := middlewareBuiltins[id.Name]
		if !ok {
			c.errorf(id.At, diag.Syntax, "USE %s(…) is not a built-in middleware; pass a FUNCTION/SUB by name without parentheses (USE %s), or use APIKEY, CORS, REQUESTLOG, RATELIMIT or BEARERJWT", id.Name, id.Name)
			return
		}
		if len(fn.Args) < arity[0] || len(fn.Args) > arity[1] {
			if arity[0] == arity[1] {
				c.errorf(id.At, diag.Syntax, "%s takes %d argument(s), got %d", id.Name, arity[0], len(fn.Args))
			} else {
				c.errorf(id.At, diag.Syntax, "%s takes %d to %d argument(s), got %d", id.Name, arity[0], arity[1], len(fn.Args))
			}
		}
		for _, a := range fn.Args {
			c.checkExpr(a)
		}
	default:
		c.errorf(st.At, diag.Syntax, "USE expects a middleware function name or built-in middleware call")
	}
}

func groupPrefixError(prefix string) string {
	if !strings.HasPrefix(prefix, "/") {
		return fmt.Sprintf("a ROUTE GROUP prefix starts with /, like \"/v1\"; got %q", prefix)
	}
	if strings.Contains(prefix, "*") {
		return "a ROUTE GROUP prefix cannot contain *; put * at the end of a ROUTE path inside the group"
	}
	if strings.ContainsAny(prefix, "{}") {
		return "a ROUTE GROUP prefix cannot contain { or }; write path parameters as :name"
	}
	if strings.Contains(prefix, "//") {
		return fmt.Sprintf("the ROUTE GROUP prefix %q has an empty part (//)", prefix)
	}
	return routePathError(prefix)
}

func scopePathError(scope string) string {
	if !strings.HasPrefix(scope, "/") {
		return fmt.Sprintf("a USE scope starts with /, like \"/admin/*\"; got %q", scope)
	}
	if strings.ContainsAny(scope, "{}") {
		return fmt.Sprintf("a USE scope cannot contain { or }; got %q", scope)
	}
	if strings.Contains(scope, "//") {
		return fmt.Sprintf("the USE scope %q has an empty part (//)", scope)
	}
	trimmed := strings.TrimSuffix(scope, "/*")
	if strings.Contains(trimmed, "*") {
		return "* only works at the end of a USE scope, like \"/admin/*\""
	}
	return ""
}

func joinRoutePath(prefix, path string) string {
	p := strings.TrimRight(prefix, "/")
	if p == "" {
		return path
	}
	if path == "/" {
		return p
	}
	return p + path
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
