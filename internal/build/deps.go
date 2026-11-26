package build

// The CLI links every module that the embedded rt imports, so that the
// module's version appears in the CLI's build info and runtimeRequires pins
// exactly that version in each program's go.mod (design §4.1).
// TestGoModPinsRuntimeImports fails if an rt import is missing here.
import (
	_ "github.com/golang-jwt/jwt/v5"    // rt/middleware.go: BEARERJWT (task 2.7)
	_ "github.com/jackc/pgx/v5/pgxpool" // rt/db.go: DBCONNECT postgres:// (task 3.1)
	_ "github.com/shopspring/decimal"   // rt/decimal.go: DECIMAL (task 2.5)
	_ "modernc.org/sqlite"              // rt/db.go: DBCONNECT sqlite:// (task 3.1)
)
