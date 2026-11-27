package rt

import "testing"

func TestCleanBasFile(t *testing.T) {
	cases := []struct {
		name string
		file string
		root string
		want string
	}{
		{"matching unix buildRoot", "/tmp/proj/.rip/test/tests/fail.test.bas", "/tmp/proj/.rip/test/", "tests/fail.test.bas"},
		{"matching windows buildRoot", `C:\Users\dev\.rip\test\tests\fail.test.bas`, "C:/Users/dev/.rip/test/", "tests/fail.test.bas"},
		{"matching trimpath module root", "ripapp/tests/fail.test.bas", "ripapp/", "tests/fail.test.bas"},
		{"relative path on linux without trimpath", "tests/fail.test.bas", "/tmp/proj/.rip/test/", "tests/fail.test.bas"},
		{"unmatched unix absolute path", "/other/dir/app.bas", "/tmp/proj/.rip/test/", "app.bas"},
		{"unmatched windows absolute path", `D:\other\dir\app.bas`, "C:/Users/dev/.rip/test/", "app.bas"},
		{"plain filename", "main.bas", "/tmp/proj/.rip/test/", "main.bas"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := cleanBasFile(tc.file, tc.root); got != tc.want {
				t.Errorf("cleanBasFile(%q, %q) = %q, want %q", tc.file, tc.root, got, tc.want)
			}
		})
	}
}
