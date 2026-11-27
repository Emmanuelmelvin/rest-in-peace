// Command playground runs the hosted REST in Peace playground (Requirement 15).
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/playground"
)

func main() {
	addr := flag.String("addr", ":8090", "listen address")
	web := flag.String("web", "playground/web/dist", "built front end directory; empty to disable")
	data := flag.String("data", filepath.Join(os.TempDir(), "rip-playground"), "data directory for programs")
	warm := flag.Bool("warm", false, "build one sample program to fill the Go caches, then exit")
	flag.Parse()

	if err := os.MkdirAll(*data, 0o755); err != nil {
		log.Fatal(err)
	}
	if *warm {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		if err := playground.Warm(ctx, *data); err != nil {
			log.Fatal(err)
		}
		log.Print("READY. the Go caches are warm")
		return
	}

	cfg := playground.Config{WebDir: *web, DataDir: *data, Limits: playground.DefaultLimits()}
	if cfg.WebDir != "" {
		if _, err := os.Stat(cfg.WebDir); err != nil {
			log.Printf("no front end at %s (run `npm run build` in playground/web); serving the API only", cfg.WebDir)
			cfg.WebDir = ""
		}
	}
	pg := playground.New(cfg)
	defer pg.Close()

	// No WriteTimeout: log streams stay open. Proxied requests have their
	// own deadline (the watchdog).
	srv := &http.Server{Addr: *addr, Handler: pg.Handler(), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 2 * time.Minute}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()

	log.Printf("READY. playground listening on %s", *addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Print(err)
	}
}
