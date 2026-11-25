// Package scaffold creates new projects for `rip new` (Requirement 13).
package scaffold

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// Templates are stored without leading dots, so go:embed picks them up and git
// doesn't treat them as config for this repository. They are renamed on write.
//
//go:embed templates
var templates embed.FS

var renames = map[string]string{
	"gitignore":   ".gitignore",
	"env.example": ".env.example",
	"kiro":        ".kiro",
}

// Create writes a new project into dir, which must not exist yet. Every
// "{{name}}" in the templates is replaced with name.
func Create(dir, name string) error {
	if _, err := os.Stat(dir); err == nil {
		return fmt.Errorf("?FILE EXISTS ERROR — %s already exists; pick another name or remove it first", dir)
	}
	return fs.WalkDir(templates, "templates", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(p, "templates"), "/")
		target := filepath.Join(dir, filepath.FromSlash(rename(rel)))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := templates.ReadFile(p)
		if err != nil {
			return err
		}
		data = []byte(strings.ReplaceAll(string(data), "{{name}}", name))
		return os.WriteFile(target, data, 0o644)
	})
}

func rename(rel string) string {
	if rel == "" {
		return rel
	}
	parts := strings.Split(rel, "/")
	for i, part := range parts {
		if r, ok := renames[part]; ok {
			parts[i] = r
		}
	}
	return path.Join(parts...)
}
