package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/Emmanuelmelvin/rest-in-peace/internal/scaffold"
)

var validName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]*$`)

func runNew(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(os.Stderr, "?SYNTAX ERROR — usage: rip new <name>")
		return 2
	}
	dir := args[0]
	name := filepath.Base(dir)
	if !validName.MatchString(name) {
		fmt.Fprintf(os.Stderr, "?SYNTAX ERROR — %q is not a valid project name; use letters, digits, - and _\n", name)
		return 2
	}
	if err := scaffold.Create(dir, name); err != nil {
		return fail(err)
	}
	fmt.Printf("CREATED %s\n\n  cd %s\n  rip dev\n\nREADY.\n", dir, dir)
	return 0
}
