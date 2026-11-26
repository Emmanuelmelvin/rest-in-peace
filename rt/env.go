package rt

import "os"

// Env implements ENV("NAME" [, default]) (Requirement 10.1).
//
// Returns the environment variable value when it is set (even if empty),
// the first element of def when the variable is absent, or NIL.
func Env(name string, def ...Value) Value {
	if v, ok := os.LookupEnv(name); ok {
		return v
	}
	if len(def) > 0 {
		return def[0]
	}
	return nil
}

// EnvRequire implements ENVREQUIRE("NAME") (Requirement 10.1).
//
// Returns the variable's value when it is set and non-empty, or panics with a
// clear BASIC-style message so the program stops at startup instead of
// misbehaving at request time.
func EnvRequire(name string) string {
	if v, ok := os.LookupEnv(name); ok && v != "" {
		return v
	}
	panic(Errorf(KindUndefined,
		"required environment variable %s is not set — add it to .env for local development, or to your host's environment settings",
		name,
	))
}
