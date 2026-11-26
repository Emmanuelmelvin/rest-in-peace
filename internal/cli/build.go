package cli

import (
	"flag"
	"fmt"
	"path/filepath"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/build"
)

func runBuild(args []string) int {
	fset := flag.NewFlagSet("build", flag.ContinueOnError)
	goos := fset.String("os", "", "target OS, e.g. linux, darwin, windows")
	goarch := fset.String("arch", "", "target architecture, e.g. amd64, arm64")
	out := fset.String("o", "", "output binary (default dist/<name>)")
	if err := fset.Parse(args); err != nil {
		return 2
	}
	p, entry, err := resolve(fset.Args())
	if err != nil {
		return fail(err)
	}
	output := *out
	if output == "" {
		output = filepath.Join(p.Root, "dist", p.Name)
	}
	// Collect migrations for embedding. The result is informational for now;
	// the generated binary runs `./app migrate` via the embedded rt (task 3.3).
	if _, err := build.MigrationsFS(p.Root); err != nil {
		return fail(err)
	}
	bin, ok := compileAndBuild(p, entry, build.Options{Output: output, OS: *goos, Arch: *goarch, Release: true})
	if !ok {
		return 1
	}
	fmt.Printf("BUILT %s\nREADY.\n", bin)
	return 0
}
