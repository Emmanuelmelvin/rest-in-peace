// Command rip is the REST in Peace toolchain: rip new, dev, run, test, build, migrate, check.
package main

import (
	"os"

	"github.com/rip-basic/rip/internal/cli"
)

func main() {
	os.Exit(cli.Run(os.Args[1:]))
}
