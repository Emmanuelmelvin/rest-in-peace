package scaffold

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "shop")
	if err := Create(dir, "shop"); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{
		"main.bas", "rip.json", ".env.example", ".gitignore", "Dockerfile",
		"migrations/001_init.sql", "tests/main.test.bas", ".kiro/steering/rip.md",
	} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(f))); err != nil {
			t.Errorf("missing %s: %v", f, err)
		}
	}
	cfg, err := os.ReadFile(filepath.Join(dir, "rip.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cfg), `"shop"`) {
		t.Errorf("rip.json does not contain the project name: %s", cfg)
	}
	if err := Create(dir, "shop"); err == nil {
		t.Error("creating over an existing directory should fail")
	}
}
