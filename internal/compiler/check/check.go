// Package check resolves names across files and produces semantic errors and warnings.
package check

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/ast"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/diag"
	"github.com/Emmanuelmelvin/rest-in-peace/internal/compiler/token"
)

// builtins are the v1 built-in functions available in every file (language.md).
var builtins = map[string]bool{
	"LEN": true, "UPPER": true, "LOWER": true, "TRIM": true,
	"LEFT": true, "RIGHT": true, "MID": true,
	"STR": true, "VAL": true, "INT": true, "ABS": true, "ROUND": true,
	"DECIMAL": true, "NOW": true,
	"ENV": true, "ENVREQUIRE": true,
	"SQL": true, "SQLONE": true, "SQLEXEC": true,
	"FETCH": true, "REQUEST": true,
	"ERROR": true, "HTTPERROR": true, "NEW": true,
	// Built-in middleware.
	"CORS": true, "RATELIMIT": true, "REQUESTLOG": true, "APIKEY": true, "BEARERJWT": true,
}

// classicBuiltins are HTTP request helpers available only in classic mode.
var classicBuiltins = map[string]string{
	"PARAM":  "req.params",
	"QUERY":  "req.query",
	"HEADER": "req.headers",
	"BODY":   "req.body",
}

// validTypes are the type names accepted after AS (language.md).
// ANY is normalized to "" by the parser.
var validTypes = map[string]bool{
	"": true, "NUMBER": true, "INTEGER": true, "STRING": true, "BOOLEAN": true,
	"MAP": true, "ARRAY": true, "DECIMAL": true, "DATE": true, "NIL": true,
}

var validTypeList = []string{
	"NUMBER", "INTEGER", "STRING", "BOOLEAN", "MAP", "ARRAY", "DECIMAL", "DATE", "NIL", "ANY",
}

var typeAliases = map[string]string{
	"INT":    "INTEGER",
	"STR":    "STRING",
	"BOOL":   "BOOLEAN",
	"FLOAT":  "NUMBER",
	"DOUBLE": "NUMBER",
	"LIST":   "ARRAY",
	"DICT":   "MAP",
	"OBJECT": "MAP",
	"NULL":   "NIL",
}

var logLevels = map[string]bool{
	"DEBUG": true, "INFO": true, "WARN": true, "ERROR": true,
}

// Check validates a whole program.
func Check(p *ast.Program) diag.List {
	if p == nil {
		return nil
	}
	var all diag.List
	for _, f := range p.Files {
		if f == nil {
			continue
		}
		c := &checker{
			file:     f,
			modScope: newScope(nil),
		}
		c.checkFile()
		sort.SliceStable(c.diags, func(i, j int) bool {
			if c.diags[i].Pos.Line != c.diags[j].Pos.Line {
				return c.diags[i].Pos.Line < c.diags[j].Pos.Line
			}
			return c.diags[i].Pos.Col < c.diags[j].Pos.Col
		})
		all = append(all, c.diags...)
	}
	// TODO(task 3.4): IMPORT/EXPORT resolution across files; using a name that isn't exported.
	// TODO(task 3.6): ?INJECTION RISK WARNING for SQL strings built from req values.
	// TODO(task 4.3): ?SHARED STATE WARNING for module variables assigned in handlers.
	return all
}

type symKind int

const (
	symVar symKind = iota
	symParam
	symConst
	symFunc
	symSub
	symImport
)

type symbol struct {
	name     string
	kind     symKind
	pos      token.Pos
	declared bool // true for DIM, CONST, PARAM, FUNCTION, SUB, IMPORT
	fn       *ast.FuncDecl
}

type scope struct {
	parent *scope
	syms   map[string]*symbol
}

func newScope(parent *scope) *scope {
	return &scope{parent: parent, syms: map[string]*symbol{}}
}

func (s *scope) lookup(name string) *symbol {
	for cur := s; cur != nil; cur = cur.parent {
		if sym, ok := cur.syms[name]; ok {
			return sym
		}
	}
	return nil
}

func (s *scope) lookupLocal(name string) *symbol {
	return s.syms[name]
}

func (s *scope) define(sym *symbol) {
	s.syms[sym.name] = sym
}

type checker struct {
	file     *ast.File
	diags    diag.List
	modScope *scope
	curScope *scope
	inFunc   *ast.FuncDecl
	inTest   bool
	loops    []token.Kind
	lineSet  map[int]bool
	hasWild  bool
	classic  bool
}

func (c *checker) errorf(pos token.Pos, kind diag.Kind, format string, args ...any) {
	c.diags = append(c.diags, diag.Errorf(pos, kind, format, args...))
}

func (c *checker) checkFile() {
	c.curScope = c.modScope
	c.registerImports()

	if c.file.Mode == token.Classic {
		c.classic = true
		c.checkClassic()
		return
	}
	c.checkStructured()
}

func (c *checker) registerImports() {
	for _, imp := range c.file.Imports {
		if !imp.FromGo && len(imp.Names) == 0 {
			c.hasWild = true
		}
		if imp.Alias != "" {
			c.modScope.define(&symbol{name: strings.ToUpper(imp.Alias), kind: symImport, pos: imp.At, declared: true})
		}
		for _, n := range imp.Names {
			c.modScope.define(&symbol{name: strings.ToUpper(n), kind: symImport, pos: imp.At, declared: true})
		}
	}
}

func (c *checker) checkStructured() {
	// Pass 1: register top-level FUNCTION and SUB declarations for forward calls.
	for _, s := range c.file.Body {
		fn, ok := s.(*ast.FuncDecl)
		if !ok {
			continue
		}
		c.declareFunc(fn)
	}

	// Pass 2: check top-level statements in source order.
	for _, s := range c.file.Body {
		switch s.(type) {
		case *ast.FuncDecl, *ast.TestDecl:
			continue
		}
		c.checkStmt(s)
	}

	// Pass 3: check FUNCTION, SUB and TEST bodies.
	for _, s := range c.file.Body {
		switch d := s.(type) {
		case *ast.FuncDecl:
			c.checkFuncBody(d)
		case *ast.TestDecl:
			c.checkTestBody(d)
		}
	}
}

func (c *checker) checkClassic() {
	c.lineSet = make(map[int]bool, len(c.file.Lines))
	for _, ln := range c.file.Lines {
		c.lineSet[ln.Number] = true
	}

	// Pass 1: collect all classic-mode globals and declarations so GOTO/GOSUB
	// control flow can assign a variable on a later line before reading it.
	for _, ln := range c.file.Lines {
		c.collectClassicDecls(ln.Stmts)
	}

	// Pass 2: check all statements and expressions.
	for _, ln := range c.file.Lines {
		for _, s := range ln.Stmts {
			c.checkStmt(s)
		}
	}
}

func (c *checker) collectClassicDecls(stmts []ast.Stmt) {
	for _, s := range stmts {
		switch st := s.(type) {
		case *ast.FuncDecl:
			c.declareFunc(st)
		case *ast.ConstDecl:
			if prev := c.modScope.lookupLocal(st.Name); prev != nil && prev.declared {
				c.errorf(st.At, diag.Syntax, "%s is already declared at %s", st.Name, prev.pos)
			} else {
				c.modScope.define(&symbol{name: st.Name, kind: symConst, pos: st.At, declared: true})
			}
		case *ast.DimStmt:
			if prev := c.modScope.lookupLocal(st.Name); prev != nil && prev.declared {
				c.errorf(st.At, diag.Syntax, "%s is already declared at %s", st.Name, prev.pos)
			} else {
				c.modScope.define(&symbol{name: st.Name, kind: symVar, pos: st.At, declared: true})
			}
		case *ast.LetStmt:
			if id, ok := st.Target.(*ast.Ident); ok {
				if c.modScope.lookupLocal(id.Name) == nil {
					c.modScope.define(&symbol{name: id.Name, kind: symVar, pos: st.At})
				}
			}
		case *ast.IfStmt:
			c.collectClassicDecls(st.Then)
			c.collectClassicDecls(st.Else)
		case *ast.ForStmt:
			if c.modScope.lookupLocal(st.Var) == nil {
				c.modScope.define(&symbol{name: st.Var, kind: symVar, pos: st.At})
			}
			c.collectClassicDecls(st.Body)
		case *ast.ForEachStmt:
			if c.modScope.lookupLocal(st.Var) == nil {
				c.modScope.define(&symbol{name: st.Var, kind: symVar, pos: st.At})
			}
			c.collectClassicDecls(st.Body)
		case *ast.WhileStmt:
			c.collectClassicDecls(st.Body)
		case *ast.DoStmt:
			c.collectClassicDecls(st.Body)
		case *ast.SelectStmt:
			for _, sc := range st.Cases {
				c.collectClassicDecls(sc.Body)
			}
			c.collectClassicDecls(st.Else)
		case *ast.TryStmt:
			c.collectClassicDecls(st.Body)
			c.collectClassicDecls(st.Catch)
			c.collectClassicDecls(st.Finally)
		case *ast.TransactionStmt:
			c.collectClassicDecls(st.Body)
		}
	}
}

func (c *checker) declareFunc(fn *ast.FuncDecl) {
	if fn.Name == "" {
		return
	}
	if prev := c.modScope.lookupLocal(fn.Name); prev != nil && prev.declared {
		c.errorf(fn.At, diag.Syntax, "%s is already declared at %s", fn.Name, prev.pos)
		return
	}
	kind := symFunc
	if fn.IsSub {
		kind = symSub
	}
	c.modScope.define(&symbol{name: fn.Name, kind: kind, pos: fn.At, declared: true, fn: fn})
}

func (c *checker) checkFuncBody(fn *ast.FuncDecl) {
	c.checkType(fn.At, fn.Result)

	local := newScope(c.modScope)
	firstDefault := ""
	for _, prm := range fn.Params {
		c.checkType(fn.At, prm.Type)
		if prm.Default != nil {
			// Evaluate default expression in module scope.
			prev := c.curScope
			c.curScope = c.modScope
			c.checkExpr(prm.Default)
			c.curScope = prev
			if firstDefault == "" {
				firstDefault = prm.Name
			}
		} else if firstDefault != "" {
			c.errorf(fn.At, diag.Syntax, "parameter %s has no default, but follows parameter %s which has one", prm.Name, firstDefault)
		}
		if prev := local.lookupLocal(prm.Name); prev != nil {
			c.errorf(fn.At, diag.Syntax, "duplicate parameter %s in %s", prm.Name, fn.Name)
			continue
		}
		local.define(&symbol{name: prm.Name, kind: symParam, pos: fn.At, declared: true})
	}

	prevScope, prevFunc, prevLoops := c.curScope, c.inFunc, c.loops
	c.curScope = local
	c.inFunc = fn
	c.loops = nil
	for _, s := range fn.Body {
		c.checkStmt(s)
	}
	c.curScope, c.inFunc, c.loops = prevScope, prevFunc, prevLoops
}

func (c *checker) checkTestBody(td *ast.TestDecl) {
	prevScope, prevFunc, prevTest, prevLoops := c.curScope, c.inFunc, c.inTest, c.loops
	c.curScope = newScope(c.modScope)
	c.inFunc = nil
	c.inTest = true
	c.loops = nil
	for _, s := range td.Body {
		c.checkStmt(s)
	}
	c.curScope, c.inFunc, c.inTest, c.loops = prevScope, prevFunc, prevTest, prevLoops
}

func (c *checker) checkType(pos token.Pos, t ast.TypeRef) {
	if validTypes[t.Name] {
		return
	}
	if hint, ok := typeAliases[t.Name]; ok {
		c.errorf(pos, diag.Undefined, "unknown type %s; did you mean %s?", t.Name, hint)
		return
	}
	if hint := suggest(t.Name, validTypeList); hint != "" {
		c.errorf(pos, diag.Undefined, "unknown type %s; did you mean %s?", t.Name, hint)
		return
	}
	c.errorf(pos, diag.Undefined, "unknown type %s; valid types are NUMBER, INTEGER, STRING, BOOLEAN, MAP, ARRAY, DECIMAL, DATE, NIL or ANY", t.Name)
}

func (c *checker) checkStmt(s ast.Stmt) {
	if s == nil {
		return
	}
	switch st := s.(type) {
	case *ast.FuncDecl:
		// Can appear here only if parsed inside classic mode.
		c.checkFuncBody(st)

	case *ast.ConstDecl:
		c.checkExpr(st.Value)
		if !c.classic {
			if prev := c.curScope.lookupLocal(st.Name); prev != nil {
				c.errorf(st.At, diag.Syntax, "%s is already declared at %s", st.Name, prev.pos)
			} else {
				c.curScope.define(&symbol{name: st.Name, kind: symConst, pos: st.At, declared: true})
			}
		}

	case *ast.DimStmt:
		c.checkType(st.At, st.Type)
		if st.Value != nil {
			c.checkExpr(st.Value)
		}
		if !c.classic {
			if prev := c.curScope.lookupLocal(st.Name); prev != nil {
				if prev.kind == symParam {
					c.errorf(st.At, diag.Syntax, "%s is already a parameter of this function", st.Name)
				} else {
					c.errorf(st.At, diag.Syntax, "%s is already declared at %s", st.Name, prev.pos)
				}
			} else {
				c.curScope.define(&symbol{name: st.Name, kind: symVar, pos: st.At, declared: true})
			}
		}

	case *ast.LetStmt:
		c.checkExpr(st.Value)
		c.checkAssignTarget(st.At, st.Target)

	case *ast.PrintStmt:
		for _, a := range st.Args {
			c.checkExpr(a)
		}

	case *ast.ExprStmt:
		c.checkCallOrExpr(st.X, true)

	case *ast.IfStmt:
		c.checkExpr(st.Cond)
		for _, b := range st.Then {
			c.checkStmt(b)
		}
		for _, b := range st.Else {
			c.checkStmt(b)
		}

	case *ast.ForStmt:
		c.checkExpr(st.From)
		c.checkExpr(st.To)
		if st.Step != nil {
			c.checkExpr(st.Step)
		}
		c.checkLoopVar(st.At, st.Var)
		c.loops = append(c.loops, token.FOR)
		for _, b := range st.Body {
			c.checkStmt(b)
		}
		c.loops = c.loops[:len(c.loops)-1]

	case *ast.ForEachStmt:
		c.checkExpr(st.In)
		c.checkLoopVar(st.At, st.Var)
		c.loops = append(c.loops, token.FOR)
		for _, b := range st.Body {
			c.checkStmt(b)
		}
		c.loops = c.loops[:len(c.loops)-1]

	case *ast.WhileStmt:
		c.checkExpr(st.Cond)
		c.loops = append(c.loops, token.WHILE)
		for _, b := range st.Body {
			c.checkStmt(b)
		}
		c.loops = c.loops[:len(c.loops)-1]

	case *ast.DoStmt:
		c.loops = append(c.loops, token.DO)
		for _, b := range st.Body {
			c.checkStmt(b)
		}
		c.loops = c.loops[:len(c.loops)-1]
		if st.Until != nil {
			c.checkExpr(st.Until)
		}

	case *ast.SelectStmt:
		c.checkExpr(st.X)
		for _, sc := range st.Cases {
			for _, v := range sc.Values {
				c.checkExpr(v)
			}
			for _, b := range sc.Body {
				c.checkStmt(b)
			}
		}
		for _, b := range st.Else {
			c.checkStmt(b)
		}

	case *ast.ExitStmt:
		if !c.inLoop(st.Loop) {
			c.errorf(st.At, diag.Syntax, "EXIT %s can only be used inside a %s loop", st.Loop, st.Loop)
		}

	case *ast.EndStmt:
		// Always valid.

	case *ast.ReturnStmt:
		if st.Value != nil {
			c.checkExpr(st.Value)
		}
		if c.classic {
			if st.Value != nil {
				c.errorf(st.At, diag.Syntax, "RETURN in classic mode returns from GOSUB and takes no value")
			}
		} else if c.inFunc == nil {
			c.errorf(st.At, diag.Syntax, "RETURN can only be used inside a FUNCTION or SUB")
		} else if c.inFunc.IsSub && st.Value != nil {
			c.errorf(st.At, diag.Syntax, "SUB %s returns nothing; remove the value after RETURN or make it a FUNCTION", c.inFunc.Name)
		}

	case *ast.ThrowStmt:
		c.checkExpr(st.Value)

	case *ast.TryStmt:
		for _, b := range st.Body {
			c.checkStmt(b)
		}
		if st.CatchVar != "" || len(st.Catch) > 0 {
			prev := c.curScope
			catchScope := newScope(prev)
			if st.CatchVar != "" {
				catchScope.define(&symbol{name: st.CatchVar, kind: symVar, pos: st.At, declared: true})
			}
			c.curScope = catchScope
			for _, b := range st.Catch {
				c.checkStmt(b)
			}
			c.curScope = prev
		}
		for _, b := range st.Finally {
			c.checkStmt(b)
		}

	case *ast.RouteStmt:
		if st.GosubLine != 0 {
			if !c.classic {
				c.errorf(st.At, diag.Syntax, "ROUTE … GOSUB is only allowed in classic mode; in structured mode use ROUTE %s %q CALL FunctionName", st.Method, st.Path)
			} else {
				c.checkLineTarget(st.At, st.GosubLine)
			}
		} else if st.Handler != "" {
			c.checkHandlerRef(st.At, st.Handler)
		}

	case *ast.GroupStmt:
		for _, r := range st.Routes {
			c.checkStmt(r)
		}

	case *ast.UseStmt:
		c.checkExpr(st.Fn)

	case *ast.RespondStmt:
		c.checkExpr(st.Status)
		c.checkExpr(st.Value)
		if st.Headers != nil {
			c.checkExpr(st.Headers)
		}

	case *ast.ServeStmt:
		if st.Port != nil {
			c.checkExpr(st.Port)
		}

	case *ast.DBConnectStmt:
		c.checkExpr(st.URL)

	case *ast.TransactionStmt:
		for _, b := range st.Body {
			c.checkStmt(b)
		}

	case *ast.LogStmt:
		if id, ok := st.Level.(*ast.Ident); ok && logLevels[id.Name] && c.curScope.lookup(id.Name) == nil {
			// Bare log level name such as LOG INFO, "msg".
		} else {
			c.checkExpr(st.Level)
		}
		c.checkExpr(st.Msg)
		if st.Fields != nil {
			c.checkExpr(st.Fields)
		}

	case *ast.ExpectStmt:
		c.checkExpr(st.Cond)
		if st.Msg != nil {
			c.checkExpr(st.Msg)
		}

	case *ast.GotoStmt:
		if !c.classic {
			c.errorf(st.At, diag.Syntax, "GOTO is only allowed in classic mode (files with line numbers); use IF, FOR, WHILE or FUNCTION instead")
			return
		}
		c.checkLineTarget(st.At, st.Line)

	case *ast.GosubStmt:
		if !c.classic {
			c.errorf(st.At, diag.Syntax, "GOSUB is only allowed in classic mode (files with line numbers); use FUNCTION or SUB instead")
			return
		}
		c.checkLineTarget(st.At, st.Line)

	case *ast.OnGotoStmt:
		c.checkExpr(st.X)
		if !c.classic {
			c.errorf(st.At, diag.Syntax, "ON … GOTO is only allowed in classic mode (files with line numbers); use SELECT CASE instead")
			return
		}
		for _, ln := range st.Lines {
			c.checkLineTarget(st.At, ln)
		}
	}
}

func (c *checker) checkAssignTarget(pos token.Pos, target ast.Expr) {
	switch t := target.(type) {
	case *ast.Ident:
		c.checkVarAssign(pos, t.Name)
	case *ast.MemberExpr:
		c.checkExpr(t.X)
	case *ast.IndexExpr:
		c.checkExpr(t.X)
		c.checkExpr(t.Index)
	default:
		c.checkExpr(target)
	}
}

func (c *checker) checkVarAssign(pos token.Pos, name string) {
	if sym := c.curScope.lookup(name); sym != nil {
		switch sym.kind {
		case symConst:
			c.errorf(pos, diag.Syntax, "cannot reassign CONST %s; declare it with DIM or LET if it needs to change", name)
			return
		case symFunc, symSub:
			c.errorf(pos, diag.Syntax, "cannot assign to %s because it is a %s", name, kindWord(sym.kind))
			return
		}
	}
	if !c.classic && c.curScope.lookupLocal(name) == nil {
		// In structured mode, assigning inside a function/sub creates a local variable (Requirement 4.3).
		c.curScope.define(&symbol{name: name, kind: symVar, pos: pos})
	}
}

func (c *checker) checkLoopVar(pos token.Pos, name string) {
	if name == "" {
		return
	}
	c.checkVarAssign(pos, name)
}

func (c *checker) checkLineTarget(pos token.Pos, line int) {
	if c.lineSet[line] {
		return
	}
	c.errorf(pos, diag.Undefined, "line %d does not exist", line)
}

func (c *checker) checkHandlerRef(pos token.Pos, name string) {
	if sym := c.modScope.lookup(name); sym != nil {
		switch sym.kind {
		case symFunc, symSub, symImport:
			return
		default:
			c.errorf(pos, diag.TypeMismatch, "%s is not a function; ROUTE CALL expects a FUNCTION name", name)
			return
		}
	}
	if c.hasWild {
		return
	}
	if hint := suggest(name, c.callableNames()); hint != "" {
		c.errorf(pos, diag.Undefined, "no function named %s; did you mean %s?", name, hint)
		return
	}
	c.errorf(pos, diag.Undefined, "no function named %s", name)
}

func (c *checker) inLoop(k token.Kind) bool {
	for i := len(c.loops) - 1; i >= 0; i-- {
		if c.loops[i] == k {
			return true
		}
	}
	return false
}

func (c *checker) checkExpr(e ast.Expr) {
	c.checkCallOrExpr(e, false)
}

func (c *checker) checkCallOrExpr(e ast.Expr, asStmt bool) {
	if e == nil {
		return
	}
	switch ex := e.(type) {
	case *ast.Ident:
		c.checkIdentRef(ex)
	case *ast.NumberLit, *ast.StringLit, *ast.BoolLit, *ast.NilLit:
		// Always valid.
	case *ast.ArrayLit:
		for _, item := range ex.Items {
			c.checkExpr(item)
		}
	case *ast.MapLit:
		for _, v := range ex.Values {
			c.checkExpr(v)
		}
	case *ast.MemberExpr:
		c.checkExpr(ex.X)
	case *ast.IndexExpr:
		c.checkExpr(ex.X)
		c.checkExpr(ex.Index)
	case *ast.UnaryExpr:
		c.checkExpr(ex.X)
	case *ast.BinaryExpr:
		c.checkExpr(ex.X)
		c.checkExpr(ex.Y)
	case *ast.CallExpr:
		c.checkCall(ex, asStmt)
	}
}

func (c *checker) checkIdentRef(id *ast.Ident) {
	if sym := c.curScope.lookup(id.Name); sym != nil {
		return
	}
	if builtins[id.Name] {
		return
	}
	if c.classic {
		if _, ok := classicBuiltins[id.Name]; ok {
			return
		}
	}
	if c.hasWild {
		return
	}
	if hint := suggest(id.Name, c.valueNames()); hint != "" {
		c.errorf(id.At, diag.Undefined, "%s is not defined; did you mean %s?", id.Name, hint)
		return
	}
	c.errorf(id.At, diag.Undefined, "%s is not defined", id.Name)
}

func (c *checker) checkCall(call *ast.CallExpr, asStmt bool) {
	for _, a := range call.Args {
		c.checkExpr(a)
	}
	id, ok := call.Fn.(*ast.Ident)
	if !ok {
		c.checkExpr(call.Fn)
		return
	}

	if sym := c.curScope.lookup(id.Name); sym != nil {
		switch sym.kind {
		case symFunc, symSub:
			if sym.kind == symSub && !asStmt {
				c.errorf(call.At, diag.TypeMismatch, "SUB %s returns no value; call it as a statement or make it a FUNCTION", id.Name)
			}
			c.checkArity(call, sym.fn)
		case symConst:
			c.errorf(call.At, diag.TypeMismatch, "%s is a CONST, not a function", id.Name)
		}
		return
	}
	if builtins[id.Name] {
		return
	}
	if alt, ok := classicBuiltins[id.Name]; ok {
		if c.classic {
			return
		}
		c.errorf(call.At, diag.Undefined, "%s is only available in classic mode; in structured mode read %s", id.Name, alt)
		return
	}
	if c.hasWild {
		return
	}
	if hint := suggest(id.Name, c.callableNames()); hint != "" {
		c.errorf(call.At, diag.Undefined, "no function named %s; did you mean %s?", id.Name, hint)
		return
	}
	c.errorf(call.At, diag.Undefined, "no function named %s", id.Name)
}

func (c *checker) checkArity(call *ast.CallExpr, fn *ast.FuncDecl) {
	if fn == nil {
		return
	}
	minArgs := 0
	for _, p := range fn.Params {
		if p.Default == nil {
			minArgs++
		}
	}
	maxArgs := len(fn.Params)
	got := len(call.Args)
	if got >= minArgs && got <= maxArgs {
		return
	}
	if minArgs == maxArgs {
		c.errorf(call.At, diag.Syntax, "%s expects %s, got %d", fn.Name, pluralArgs(minArgs), got)
		return
	}
	if got < minArgs {
		c.errorf(call.At, diag.Syntax, "%s expects at least %s, got %d", fn.Name, pluralArgs(minArgs), got)
		return
	}
	c.errorf(call.At, diag.Syntax, "%s expects at most %s, got %d", fn.Name, pluralArgs(maxArgs), got)
}

func pluralArgs(n int) string {
	if n == 1 {
		return "1 argument"
	}
	return fmt.Sprintf("%d arguments", n)
}

func kindWord(k symKind) string {
	switch k {
	case symFunc:
		return "FUNCTION"
	case symSub:
		return "SUB"
	case symConst:
		return "CONST"
	default:
		return "variable"
	}
}

func (c *checker) callableNames() []string {
	seen := map[string]bool{}
	var out []string
	add := func(name string) {
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	for s := c.curScope; s != nil; s = s.parent {
		for name, sym := range s.syms {
			if sym.kind == symFunc || sym.kind == symSub || sym.kind == symImport {
				add(name)
			}
		}
	}
	for name := range builtins {
		add(name)
	}
	if c.classic {
		for name := range classicBuiltins {
			add(name)
		}
	}
	return out
}

func (c *checker) valueNames() []string {
	seen := map[string]bool{}
	var out []string
	add := func(name string) {
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	for s := c.curScope; s != nil; s = s.parent {
		for name := range s.syms {
			add(name)
		}
	}
	return out
}

// suggest returns the closest candidate to target by Levenshtein edit distance,
// or "" if no candidate is close enough.
func suggest(target string, candidates []string) string {
	n := len(target)
	if n <= 1 {
		return ""
	}
	maxDist := 1
	switch {
	case n >= 7:
		maxDist = 3
	case n >= 4:
		maxDist = 2
	}
	best, bestDist := "", maxDist+1
	for _, cand := range candidates {
		if cand == target {
			continue
		}
		d := editDistance(target, cand)
		if d <= maxDist && (d < bestDist || (d == bestDist && cand < best)) {
			best = cand
			bestDist = d
		}
	}
	return best
}

func editDistance(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			del := prev[j] + 1
			ins := curr[j-1] + 1
			sub := prev[j-1] + cost
			m := del
			if ins < m {
				m = ins
			}
			if sub < m {
				m = sub
			}
			curr[j] = m
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}
