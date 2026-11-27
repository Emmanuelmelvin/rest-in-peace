package rt

import (
	"context"
	"math"
	"sync"
	"time"
)

// Sleep implements SLEEP(ms). It pauses for ms milliseconds (which may be an
// INTEGER or NUMBER >= 0), respecting request cancellation (c.Context().Done())
// and any playground deadline set on c. Returns NIL.
func Sleep(c *Ctx, ms Value) Value {
	msFloat, ok := toMillis(ms)
	if !ok {
		panic(Errorf(KindTypeMismatch, "SLEEP needs a non-negative NUMBER of milliseconds, got %s %s", TypeName(ms), ToString(ms)))
	}
	dur := time.Duration(msFloat * float64(time.Millisecond))

	var ctxDone <-chan struct{}
	var hasDeadline bool
	var deadline time.Time
	if c != nil {
		if ctx := c.Context(); ctx != nil {
			ctxDone = ctx.Done()
		}
		if !c.deadline.IsZero() {
			hasDeadline = true
			deadline = c.deadline
		}
	}

	if hasDeadline {
		rem := time.Until(deadline)
		if rem <= 0 {
			panic(&Error{Kind: KindEternalLoop, Status: 508, Message: "SLEEP exceeded the playground time limit"})
		}
		if dur == 0 {
			return nil
		}
		sleepTimer := time.NewTimer(dur)
		defer sleepTimer.Stop()
		deadlineTimer := time.NewTimer(rem)
		defer deadlineTimer.Stop()
		select {
		case <-sleepTimer.C:
			return nil
		case <-deadlineTimer.C:
			panic(&Error{Kind: KindEternalLoop, Status: 508, Message: "SLEEP exceeded the playground time limit"})
		case <-ctxDone:
			return nil
		}
	}

	if dur == 0 {
		return nil
	}
	timer := time.NewTimer(dur)
	defer timer.Stop()
	select {
	case <-timer.C:
	case <-ctxDone:
	}
	return nil
}

// Spawn implements SPAWN Fn(args...). It launches fn in a background goroutine
// with a detached *Ctx attached to the same *App. Any panic inside fn is
// recovered and logged via Log("ERROR", "SPAWN PANIC", ...).
// SPAWN is disallowed in the hosted playground.
func Spawn(c *Ctx, fn func(*Ctx)) {
	if isPlayground() {
		panic(Errorf(KindNotAllowed, "SPAWN is not allowed in the playground — use the rip CLI"))
	}
	if fn == nil {
		return
	}
	bg := NewCtx(context.Background())
	if c != nil {
		bg.app = c.app
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				e := FromPanic(r)
				Log(bg, "ERROR", "SPAWN PANIC", MapOf("error", e.Error(), "kind", string(e.Kind)))
			}
		}()
		fn(bg)
	}()
}

// Every implements EVERY ms CALL Fn. It starts a background ticker that invokes
// fn with a fresh detached *Ctx every ms milliseconds (minimum 10ms) until the
// app shuts down. Any panic inside a tick is recovered and logged via Log.
// EVERY is disallowed in the hosted playground.
func Every(c *Ctx, ms Value, fn func(*Ctx)) {
	if isPlayground() {
		panic(Errorf(KindNotAllowed, "EVERY is not allowed in the playground — use the rip CLI"))
	}
	msFloat, ok := toMillis(ms)
	if !ok || msFloat < 10 {
		panic(Errorf(KindTypeMismatch, "EVERY needs an interval of at least 10 milliseconds, got %s %s", TypeName(ms), ToString(ms)))
	}
	if fn == nil {
		return
	}
	dur := time.Duration(msFloat * float64(time.Millisecond))

	stop := make(chan struct{})
	var stopOnce sync.Once
	var app *App
	if c != nil {
		app = c.App()
		app.OnShutdown(func() {
			stopOnce.Do(func() { close(stop) })
		})
	}

	go func() {
		ticker := time.NewTicker(dur)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				runEveryTick(app, fn)
			}
		}
	}()
}

func runEveryTick(app *App, fn func(*Ctx)) {
	bg := NewCtx(context.Background())
	bg.app = app
	defer func() {
		if r := recover(); r != nil {
			e := FromPanic(r)
			Log(bg, "ERROR", "EVERY PANIC", MapOf("error", e.Error(), "kind", string(e.Kind)))
		}
	}()
	fn(bg)
}

// Parallel implements PARALLEL ... END PARALLEL. It runs every task in its own
// goroutine and waits for all of them to complete. If one or more tasks panic,
// Parallel re-panics the first recovered panic after all tasks finish so
// enclosing TRY/CATCH blocks catch it naturally.
func Parallel(c *Ctx, tasks ...func()) {
	if len(tasks) == 0 {
		return
	}
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		firstErr any
		hasPanic bool
	)
	for _, task := range tasks {
		if task == nil {
			continue
		}
		wg.Add(1)
		go func(fn func()) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					mu.Lock()
					if !hasPanic {
						firstErr = r
						hasPanic = true
					}
					mu.Unlock()
				}
			}()
			fn()
		}(task)
	}
	wg.Wait()
	if hasPanic {
		panic(firstErr)
	}
}

func toMillis(v Value) (float64, bool) {
	switch x := v.(type) {
	case int64:
		if x < 0 {
			return 0, false
		}
		return float64(x), true
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) || x < 0 {
			return 0, false
		}
		return x, true
	}
	return 0, false
}
