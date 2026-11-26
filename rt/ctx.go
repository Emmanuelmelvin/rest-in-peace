package rt

import (
	"context"
	"time"
)

// DefaultMaxDepth is the recursion limit (Requirement 4.5).
const DefaultMaxDepth = 10000

// Ctx is passed as the first argument to every generated function
// (design §4.4). There is one per request, plus one for top-level code.
type Ctx struct {
	ctx      context.Context
	depth    int
	maxDepth int
	deadline time.Time // zero = none; set in playground (--guard) builds
	ticks    uint32
	gosub    []int // classic mode: this request's GOSUB return stack
	app      *App

	// Req is the current request, or nil outside a request.
	Req *Request

	// TODO(task 3.2): the active transaction, so SQL calls inside TRANSACTION use it.
}

// NewCtx returns a context for top-level code or a request.
func NewCtx(parent context.Context) *Ctx {
	if parent == nil {
		parent = context.Background()
	}
	return &Ctx{ctx: parent, maxDepth: DefaultMaxDepth}
}

// App returns the HTTP app that ROUTE and SERVE build. Every request's Ctx
// shares its program's app; a Ctx made with NewCtx gets a new one on first use.
func (c *Ctx) App() *App {
	if c.app == nil {
		c.app = NewApp("")
	}
	return c.app
}

// Context returns the Go context, which Go interop passes to context.Context parameters.
func (c *Ctx) Context() context.Context { return c.ctx }

// SetDeadline sets the request deadline that Guard enforces.
func (c *Ctx) SetDeadline(t time.Time) { c.deadline = t }

// Enter is called at the top of every generated function:
//
//	defer c.Enter()()
func (c *Ctx) Enter() func() {
	c.depth++
	if c.depth > c.maxDepth {
		c.depth--
		panic(Errorf(KindStackOverflow, "more than %d nested calls — is a function calling itself forever?", c.maxDepth))
	}
	return func() { c.depth-- }
}

// Guard is inserted at loop back-edges and GOTOs in --guard builds
// (design §4.12). It checks the clock every 1024 calls to stay cheap.
func (c *Ctx) Guard() {
	c.ticks++
	if c.ticks&1023 != 0 || c.deadline.IsZero() {
		return
	}
	if time.Now().After(c.deadline) {
		panic(&Error{Kind: KindEternalLoop, Status: 508, Message: "TRAPPED IN AN ETERNAL GOTO LOOP"})
	}
}

// GosubPush records where RETURN should jump back to (classic mode).
func (c *Ctx) GosubPush(returnID int) { c.gosub = append(c.gosub, returnID) }

// GosubPop implements RETURN (classic mode).
func (c *Ctx) GosubPop() int {
	n := len(c.gosub)
	if n == 0 {
		panic(Errorf(KindReturnWithoutGosub, "a RETURN ran with no GOSUB to return to"))
	}
	id := c.gosub[n-1]
	c.gosub = c.gosub[:n-1]
	return id
}
