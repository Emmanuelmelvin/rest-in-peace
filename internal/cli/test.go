package cli

import (
	"fmt"
	"os"
)

func runTest(args []string) int {
	// TODO(task 4.2): find tests/*.test.bas, compile them together with the entry
	// file (SERVE does not bind a port), swap DBCONNECT for SQLite :memory: (or
	// TEST_DATABASE_URL), load .env.test, run each TEST, and report failures as
	// ?EXPECT FAILED ERROR IN file:line. Exit non-zero if any test fails.
	fmt.Fprintln(os.Stderr, "?NOT IMPLEMENTED ERROR — rip test is not built yet (task 4.2 in .kiro/specs/rip-basic/tasks.md)")
	return 1
}
