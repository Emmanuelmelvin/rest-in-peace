package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnv(t *testing.T) {
	t.Setenv("RIP_ALREADY_SET", "real")
	path := filepath.Join(t.TempDir(), ".env")
	content := "# comment\nPORT=9090\nQUOTED=\"hello\"\nRIP_ALREADY_SET=fromfile\n\nNOEQUALS\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got := loadDotEnv(path)
	want := []string{"PORT=9090", "QUOTED=hello"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

func TestUnknownCommand(t *testing.T) {
	if code := Run([]string{"resurrect"}); code != 2 {
		t.Fatalf("exit code %d, want 2", code)
	}
}
