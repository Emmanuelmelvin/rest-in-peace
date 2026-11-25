package rt

import "os"

// Env implements ENV("NAME" [, default]) (Requirement 10.1).
func Env(name string, def ...Value) Value {
	if v, ok := os.LookupEnv(name); ok {
		return v
	}
	if len(def) > 0 {
		return def[0]
	}
	return nil
}

// EnvRequire implements ENVREQUIRE("NAME"): startup stops if it is missing.
func EnvRequire(name string) string {
	v, ok := os.LookupEnv(name)
	if !ok || v == "" {
		panic(Errorf(KindUndefined, "environment variable %s is required — set it in .env for development, or in your host's settings", name))
	}
	return v
}
