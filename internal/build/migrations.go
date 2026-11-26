package build

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// MigrationsFS returns a map of filename → SQL content for every *.sql file
// found in the migrations/ subdirectory of projectRoot, sorted by filename.
// It returns nil, nil when the directory does not exist or contains no .sql files.
//
// This is used by rip build (task 3.3) to embed migrations into the compiled
// binary so that `./app migrate` can apply them without the source tree.
func MigrationsFS(projectRoot string) (map[string][]byte, error) {
	dir := filepath.Join(projectRoot, "migrations")
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := make(map[string][]byte)
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".sql") {
			data, err := os.ReadFile(filepath.Join(dir, e.Name()))
			if err != nil {
				return nil, err
			}
			out[e.Name()] = data
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// SortedMigrationNames returns the keys of m in lexicographic order.
func SortedMigrationNames(m map[string][]byte) []string {
	names := make([]string, 0, len(m))
	for k := range m {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
