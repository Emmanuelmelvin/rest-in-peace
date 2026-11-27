package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/build"
)

func runRun(args []string) int {
	p, entry, err := resolve(args)
	if err != nil {
		return fail(err)
	}
	bin, ok := compileAndBuild(p, entry, build.Options{})
	if !ok {
		return 1
	}
	return execProgram(bin, loadDotEnv(filepath.Join(p.Root, ".env")))
}

func runDev(args []string) int {
	p, entry, err := resolve(args)
	if err != nil {
		return fail(err)
	}
	if _, err := build.LookGo(); err != nil {
		return fail(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	envPath := filepath.Join(p.Root, ".env")
	cfg := watcherConfig{
		root:         p.Root,
		pollInterval: defaultPollInterval,
		debounce:     defaultDebounce,
		stderr:       os.Stderr,
		build: func(ctx context.Context, gen int) (string, bool) {
			res, ok := compile(entry, false)
			if !ok {
				return "", false
			}
			dir := filepath.Join(p.Root, ".rip", "build")
			if err := build.Prepare(dir, res.Files); err != nil {
				fmt.Fprintln(os.Stderr, err)
				return "", false
			}
			out := filepath.Join(p.Root, ".rip", "bin", fmt.Sprintf("%s-dev-%d", p.Name, gen))
			bin, err := build.Build(ctx, dir, build.Options{Output: out})
			if err != nil {
				if ctx.Err() == nil {
					fmt.Fprintln(os.Stderr, err)
				}
				return "", false
			}
			return bin, true
		},
		start: startOSProcess,
		loadEnv: func() []string {
			return loadDotEnv(envPath)
		},
	}

	return runWatchLoop(ctx, cfg)
}
