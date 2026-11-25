package cli

import (
	"fmt"
	"os"
)

func runMigrate(args []string) int {
	// TODO(task 3.3): connect to DATABASE_URL, create _rip_migrations if needed, and
	// apply migrations/*.sql in name order, each in its own transaction and once only.
	fmt.Fprintln(os.Stderr, "?NOT IMPLEMENTED ERROR — rip migrate is not built yet (task 3.3 in .kiro/specs/rip-basic/tasks.md)")
	return 1
}
