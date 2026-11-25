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
	// TODO(task 3.3): embed migrations/*.sql in the binary and support `./app migrate`.
	bin, ok := compileAndBuild(p, entry, build.Options{Output: output, OS: *goos, Arch: *goarch, Release: true})
	if !ok {
		return 1
	}
	fmt.Printf("BUILT %s\nREADY.\n", bin)
	return 0
}
