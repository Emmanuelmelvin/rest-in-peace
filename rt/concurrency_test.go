package rt

import (
	"bytes"
	"context"
	"math"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func catchConcurrencyError(fn func()) (err *Error) {
	defer func() {
		if r := recover(); r != nil {
			err = FromPanic(r)
		}
	}()
	fn()
	return nil
}

func TestSleep(t *testing.T) {
	t.Run("valid durations", func(t *testing.T) {
		cases := []struct {
			name   string
			ms     Value
			minDur time.Duration
		}{
			{"zero int", int64(0), 0},
			{"zero float", float64(0), 0},
			{"int 20ms", int64(20), 15 * time.Millisecond},
			{"float 15.5ms", float64(15.5), 10 * time.Millisecond},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				c := NewCtx(context.Background())
				start := time.Now()
				res := Sleep(c, tc.ms)
				elapsed := time.Since(start)
				if res != nil {
					t.Fatalf("Sleep(%v) = %v, want nil", tc.ms, res)
				}
				if elapsed < tc.minDur {
					t.Errorf("Sleep(%v) returned too early: %v < %v", tc.ms, elapsed, tc.minDur)
				}
			})
		}
	})

	t.Run("invalid arguments", func(t *testing.T) {
		cases := []struct {
			name string
			ms   Value
		}{
			{"negative int", int64(-1)},
			{"negative float", float64(-0.5)},
			{"NaN", math.NaN()},
			{"pos inf", math.Inf(1)},
			{"neg inf", math.Inf(-1)},
			{"string", "100"},
			{"nil", nil},
			{"bool", true},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				c := NewCtx(context.Background())
				err := catchConcurrencyError(func() { Sleep(c, tc.ms) })
				if err == nil {
					t.Fatalf("Sleep(%v) did not panic, want %s", tc.ms, KindTypeMismatch)
				}
				if err.Kind != KindTypeMismatch {
					t.Errorf("Sleep(%v) kind = %s, want %s", tc.ms, err.Kind, KindTypeMismatch)
				}
			})
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		c := NewCtx(ctx)
		go func() {
			time.Sleep(20 * time.Millisecond)
			cancel()
		}()
		start := time.Now()
		Sleep(c, int64(5000))
		if elapsed := time.Since(start); elapsed > time.Second {
			t.Errorf("Sleep did not unblock on context cancel; took %v", elapsed)
		}
	})

	t.Run("respects playground deadline", func(t *testing.T) {
		c := NewCtx(context.Background())
		c.SetDeadline(time.Now().Add(25 * time.Millisecond))
		err := catchConcurrencyError(func() { Sleep(c, int64(5000)) })
		if err == nil {
			t.Fatal("Sleep did not panic on deadline")
		}
		if err.Kind != KindEternalLoop {
			t.Errorf("kind = %s, want %s", err.Kind, KindEternalLoop)
		}
		if err.Status != 508 {
			t.Errorf("status = %d, want 508", err.Status)
		}
	})

	t.Run("already expired deadline", func(t *testing.T) {
		c := NewCtx(context.Background())
		c.SetDeadline(time.Now().Add(-10 * time.Millisecond))
		err := catchConcurrencyError(func() { Sleep(c, int64(0)) })
		if err == nil || err.Kind != KindEternalLoop {
			t.Fatalf("got err = %v, want %s", err, KindEternalLoop)
		}
	})
}

func TestSpawn(t *testing.T) {
	t.Run("runs detached with same app", func(t *testing.T) {
		parentCtx, cancel := context.WithCancel(context.Background())
		c := NewCtx(parentCtx)
		app := c.App()

		done := make(chan struct{})
		var gotApp *App
		var bgErr error
		Spawn(c, func(bg *Ctx) {
			time.Sleep(20 * time.Millisecond)
			gotApp = bg.app
			bgErr = bg.Context().Err()
			close(done)
		})
		cancel() // cancel parent immediately; background ctx must stay alive

		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("Spawn goroutine did not complete")
		}
		if gotApp != app {
			t.Errorf("Spawn background Ctx did not inherit *App")
		}
		if bgErr != nil {
			t.Errorf("Spawn background Ctx was canceled with parent: %v", bgErr)
		}
	})

	t.Run("recovers panic and logs", func(t *testing.T) {
		var mu sync.Mutex
		var buf bytes.Buffer
		w := &syncWriter{mu: &mu, buf: &buf}
		SetLogOutput(w)
		defer SetLogOutput(os.Stdout)

		done := make(chan struct{})
		c := NewCtx(context.Background())
		Spawn(c, func(bg *Ctx) {
			defer close(done)
			panic(Errorf(KindUser, "boom in spawn"))
		})

		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatal("Spawn goroutine did not finish")
		}
		// Give the outer recover defer a moment to write the log line.
		deadline := time.Now().Add(time.Second)
		for time.Now().Before(deadline) {
			mu.Lock()
			s := buf.String()
			mu.Unlock()
			if strings.Contains(s, "SPAWN PANIC") && strings.Contains(s, "boom in spawn") {
				return
			}
			time.Sleep(5 * time.Millisecond)
		}
		mu.Lock()
		defer mu.Unlock()
		t.Errorf("expected SPAWN PANIC in log output, got: %q", buf.String())
	})

	t.Run("blocked in playground", func(t *testing.T) {
		t.Setenv("RIP_PLAYGROUND", "1")
		c := NewCtx(context.Background())
		err := catchConcurrencyError(func() {
			Spawn(c, func(bg *Ctx) {})
		})
		if err == nil || err.Kind != KindNotAllowed {
			t.Fatalf("got err = %v, want %s", err, KindNotAllowed)
		}
	})
}

func TestEvery(t *testing.T) {
	t.Run("ticks and stops on shutdown", func(t *testing.T) {
		c := NewCtx(context.Background())
		app := c.App()
		var ticks atomic.Int32
		reached := make(chan struct{})
		var once sync.Once

		Every(c, int64(15), func(bg *Ctx) {
			if ticks.Add(1) >= 3 {
				once.Do(func() { close(reached) })
			}
		})

		select {
		case <-reached:
		case <-time.After(2 * time.Second):
			t.Fatalf("Every only ticked %d times, want >= 3", ticks.Load())
		}

		// Trigger registered shutdown hooks.
		app.shutdownMu.Lock()
		hooks := append([]func(){}, app.onShutdown...)
		app.shutdownMu.Unlock()
		for _, h := range hooks {
			h()
		}
		// Wait briefly for any in-flight tick, then verify ticks stop growing.
		time.Sleep(30 * time.Millisecond)
		afterStop := ticks.Load()
		time.Sleep(50 * time.Millisecond)
		if got := ticks.Load(); got != afterStop {
			t.Errorf("Every kept ticking after shutdown: before=%d, after=%d", afterStop, got)
		}
	})

	t.Run("recovers panic and keeps ticking", func(t *testing.T) {
		var mu sync.Mutex
		var buf bytes.Buffer
		SetLogOutput(&syncWriter{mu: &mu, buf: &buf})
		defer SetLogOutput(os.Stdout)

		c := NewCtx(context.Background())
		app := c.App()
		defer func() {
			app.shutdownMu.Lock()
			hooks := append([]func(){}, app.onShutdown...)
			app.shutdownMu.Unlock()
			for _, h := range hooks {
				h()
			}
		}()

		var count atomic.Int32
		done := make(chan struct{})
		var once sync.Once
		Every(c, int64(15), func(bg *Ctx) {
			n := count.Add(1)
			if n == 1 {
				panic(Errorf(KindUser, "tick 1 failed"))
			}
			if n >= 2 {
				once.Do(func() { close(done) })
			}
		})

		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Fatalf("Every stopped ticking after panic; count=%d", count.Load())
		}

		mu.Lock()
		out := buf.String()
		mu.Unlock()
		if !strings.Contains(out, "EVERY PANIC") || !strings.Contains(out, "tick 1 failed") {
			t.Errorf("expected EVERY PANIC log, got %q", out)
		}
	})

	t.Run("invalid intervals", func(t *testing.T) {
		cases := []struct {
			name string
			ms   Value
		}{
			{"zero", int64(0)},
			{"below minimum int", int64(9)},
			{"below minimum float", float64(9.9)},
			{"negative", int64(-10)},
			{"string", "100"},
			{"nil", nil},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				c := NewCtx(context.Background())
				err := catchConcurrencyError(func() {
					Every(c, tc.ms, func(bg *Ctx) {})
				})
				if err == nil || err.Kind != KindTypeMismatch {
					t.Fatalf("Every(%v) err = %v, want %s", tc.ms, err, KindTypeMismatch)
				}
			})
		}
	})

	t.Run("blocked in playground", func(t *testing.T) {
		t.Setenv("RIP_PLAYGROUND", "1")
		c := NewCtx(context.Background())
		err := catchConcurrencyError(func() {
			Every(c, int64(100), func(bg *Ctx) {})
		})
		if err == nil || err.Kind != KindNotAllowed {
			t.Fatalf("got err = %v, want %s", err, KindNotAllowed)
		}
	})
}

func TestParallel(t *testing.T) {
	t.Run("runs tasks concurrently", func(t *testing.T) {
		c := NewCtx(context.Background())
		ch1 := make(chan struct{})
		ch2 := make(chan struct{})
		var a, b int

		// Both tasks rendezvous through channels; this would deadlock if run sequentially.
		Parallel(c,
			func() {
				close(ch1)
				<-ch2
				a = 10
			},
			func() {
				<-ch1
				close(ch2)
				b = 20
			},
		)
		if a != 10 || b != 20 {
			t.Errorf("got a=%d b=%d, want 10, 20", a, b)
		}
	})

	t.Run("empty and nil tasks", func(t *testing.T) {
		c := NewCtx(context.Background())
		Parallel(c)
		Parallel(c, nil, nil)
	})

	t.Run("re-panics first error for TRY/CATCH", func(t *testing.T) {
		c := NewCtx(context.Background())
		var secondDone atomic.Bool
		var caught *Error

		func() {
			defer Recover(&caught)
			Parallel(c,
				func() {
					panic(Errorf(KindUser, "branch failed"))
				},
				func() {
					time.Sleep(15 * time.Millisecond)
					secondDone.Store(true)
				},
			)
		}()

		if !secondDone.Load() {
			t.Error("Parallel returned before sibling task completed")
		}
		if caught == nil {
			t.Fatal("TRY/CATCH did not catch panic from Parallel")
		}
		if caught.Kind != KindUser || caught.Message != "branch failed" {
			t.Errorf("caught = %+v, want USER 'branch failed'", caught)
		}
	})
}

type syncWriter struct {
	mu  *sync.Mutex
	buf *bytes.Buffer
}

func (w *syncWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.Write(p)
}
