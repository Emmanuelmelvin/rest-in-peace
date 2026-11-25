// Command playground runs the hosted REST in Peace playground (Requirement 15).
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/rip-basic/rip/internal/playground"
)

func main() {
	addr := flag.String("addr", ":8090", "listen address")
	web := flag.String("web", "playground/web/dist", "built front end directory; empty to disable")
	data := flag.String("data", filepath.Join(os.TempDir(), "rip-playground"), "data directory for programs")
	flag.Parse()

	cfg := playground.Config{WebDir: *web, DataDir: *data, Limits: playground.DefaultLimits()}
	if cfg.WebDir != "" {
		if _, err := os.Stat(cfg.WebDir); err != nil {
			log.Printf("no front end at %s (run `npm run build` in playground/web); serving the API only", cfg.WebDir)
			cfg.WebDir = ""
		}
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		log.Fatal(err)
	}
	log.Printf("READY. playground listening on %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, playground.New(cfg).Handler()))
}
