package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/rip-basic/rip/internal/build"
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
	// TODO(task 4.1): watch *.bas (and .env, migrations/), rebuild on change, and
	// restart the program, keeping the last good build running when a build fails.
	fmt.Fprintln(os.Stderr, "rip dev: watch mode is not built yet (task 4.1); running once")
	return runRun(args)
}
