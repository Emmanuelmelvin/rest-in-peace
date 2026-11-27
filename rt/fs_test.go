package rt

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func catchFSError(t *testing.T, fn func()) *Error {
	t.Helper()
	var got *Error
	func() {
		defer func() {
			r := recover()
			if r == nil {
				return
			}
			e, ok := r.(*Error)
			if !ok {
				t.Fatalf("expected *rt.Error panic, got %T: %v", r, r)
			}
			got = e
		}()
		fn()
	}()
	return got
}

func TestFSReadWriteAppendDelete(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "hello.txt")

	if FileExists(file) {
		t.Fatalf("expected %q not to exist initially", file)
	}

	if got := WriteFile(file, "hello\n"); got != nil {
		t.Fatalf("WriteFile returned %v, want nil", got)
	}
	if !FileExists(file) {
		t.Fatalf("expected %q to exist after WriteFile", file)
	}
	if got := ReadFile(file); got != "hello\n" {
		t.Fatalf("ReadFile = %q, want %q", got, "hello\n")
	}

	// Overwrite with UTF-8 content
	if got := WriteFile(file, "héllo 🌍\n"); got != nil {
		t.Fatalf("WriteFile overwrite returned %v, want nil", got)
	}
	if got := ReadFile(file); got != "héllo 🌍\n" {
		t.Fatalf("ReadFile after overwrite = %q, want %q", got, "héllo 🌍\n")
	}

	// Append to existing file
	if got := AppendFile(file, "line 2\n"); got != nil {
		t.Fatalf("AppendFile returned %v, want nil", got)
	}
	if got := ReadFile(file); got != "héllo 🌍\nline 2\n" {
		t.Fatalf("ReadFile after AppendFile = %q, want %q", got, "héllo 🌍\nline 2\n")
	}

	// Append creates file if absent
	appended := filepath.Join(dir, "new.txt")
	if got := AppendFile(appended, "created"); got != nil {
		t.Fatalf("AppendFile create returned %v, want nil", got)
	}
	if got := ReadFile(appended); got != "created" {
		t.Fatalf("ReadFile(new.txt) = %q, want %q", got, "created")
	}

	// DeleteFile
	if got := DeleteFile(file); got != nil {
		t.Fatalf("DeleteFile returned %v, want nil", got)
	}
	if FileExists(file) {
		t.Fatalf("expected %q not to exist after DeleteFile", file)
	}
}

func TestFSErrors(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "does-not-exist.txt")
	missingNested := filepath.Join(dir, "no-such-dir", "file.txt")

	tests := []struct {
		name string
		fn   func()
	}{
		{"ReadFile missing", func() { ReadFile(missing) }},
		{"WriteFile missing parent", func() { WriteFile(missingNested, "x") }},
		{"AppendFile missing parent", func() { AppendFile(missingNested, "x") }},
		{"DeleteFile missing", func() { DeleteFile(missing) }},
		{"ReadDir missing", func() { ReadDir(missing) }},
		{"FileStat missing", func() { FileStat(missing) }},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := catchFSError(t, tc.fn)
			if err == nil {
				t.Fatalf("expected panic, got none")
			}
			if err.Kind != KindInternal {
				t.Fatalf("error kind = %q, want %q (msg: %s)", err.Kind, KindInternal, err.Message)
			}
		})
	}
}

func TestFSMkdirAndReadDir(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "a", "b", "c")

	if got := Mkdir(nested); got != nil {
		t.Fatalf("Mkdir returned %v, want nil", got)
	}
	if !FileExists(nested) {
		t.Fatalf("expected %q to exist after Mkdir", nested)
	}
	// Idempotent on existing dir
	if got := Mkdir(nested); got != nil {
		t.Fatalf("Mkdir (existing) returned %v, want nil", got)
	}

	// Empty dir
	empty := ReadDir(nested)
	if empty.Len() != 0 {
		t.Fatalf("ReadDir(empty) len = %d, want 0", empty.Len())
	}

	// Populated dir: entries must be sorted by name
	WriteFile(filepath.Join(nested, "zeta.txt"), "z")
	WriteFile(filepath.Join(nested, "alpha.txt"), "a")
	Mkdir(filepath.Join(nested, "middle"))

	entries := ReadDir(nested)
	want := []string{"alpha.txt", "middle", "zeta.txt"}
	if entries.Len() != len(want) {
		t.Fatalf("ReadDir len = %d, want %d", entries.Len(), len(want))
	}
	for i, w := range want {
		if got, ok := entries.Items[i].(string); !ok || got != w {
			t.Fatalf("entries[%d] = %#v, want %q", i, entries.Items[i], w)
		}
	}
}

func TestFSFileStat(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "data.bin")
	payload := "1234567890"
	WriteFile(file, payload)

	st := FileStat(file)
	name, _ := st.Get("name")
	size, _ := st.Get("size")
	isDir, _ := st.Get("isDir")
	modTime, _ := st.Get("modTime")

	if name != "data.bin" {
		t.Errorf("name = %#v, want %q", name, "data.bin")
	}
	if size != int64(len(payload)) {
		t.Errorf("size = %#v, want %d", size, len(payload))
	}
	if isDir != false {
		t.Errorf("isDir = %#v, want false", isDir)
	}
	modStr, ok := modTime.(string)
	if !ok {
		t.Fatalf("modTime = %#v, want string", modTime)
	}
	parsed, err := time.Parse(time.RFC3339, modStr)
	if err != nil {
		t.Errorf("modTime %q is not valid RFC3339: %v", modStr, err)
	}
	if !strings.HasSuffix(modStr, "Z") {
		t.Errorf("modTime %q expected UTC RFC3339 ending in Z (parsed %v)", modStr, parsed)
	}

	// Directory stat
	sub := filepath.Join(dir, "sub")
	Mkdir(sub)
	dst := FileStat(sub)
	if dName, _ := dst.Get("name"); dName != "sub" {
		t.Errorf("dir name = %#v, want %q", dName, "sub")
	}
	if dIsDir, _ := dst.Get("isDir"); dIsDir != true {
		t.Errorf("dir isDir = %#v, want true", dIsDir)
	}
}

func TestFSPlaygroundGuard(t *testing.T) {
	t.Setenv("RIP_PLAYGROUND", "1")
	dir := t.TempDir()
	file := filepath.Join(dir, "blocked.txt")

	blocked := []struct {
		name string
		fn   func()
	}{
		{"READFILE", func() { ReadFile(file) }},
		{"WRITEFILE", func() { WriteFile(file, "x") }},
		{"APPENDFILE", func() { AppendFile(file, "x") }},
		{"FILEEXISTS", func() { FileExists(file) }},
		{"DELETEFILE", func() { DeleteFile(file) }},
		{"MKDIR", func() { Mkdir(filepath.Join(dir, "sub")) }},
		{"READDIR", func() { ReadDir(dir) }},
		{"FILESTAT", func() { FileStat(dir) }},
	}

	for _, tc := range blocked {
		t.Run(tc.name, func(t *testing.T) {
			err := catchFSError(t, tc.fn)
			if err == nil {
				t.Fatalf("expected %s to panic in playground", tc.name)
			}
			if err.Kind != KindNotAllowed {
				t.Fatalf("%s kind = %q, want %q", tc.name, err.Kind, KindNotAllowed)
			}
			if !strings.Contains(err.Message, "playground") {
				t.Fatalf("%s message %q should mention playground", tc.name, err.Message)
			}
		})
	}

	// Path helpers must remain allowed in the playground.
	joined := PathJoin("a", "b", "c.txt")
	wantJoined := filepath.Join("a", "b", "c.txt")
	if joined != wantJoined {
		t.Fatalf("PathJoin in playground = %q, want %q", joined, wantJoined)
	}
	if got := DirName(joined); got != filepath.Dir(joined) {
		t.Fatalf("DirName in playground = %q, want %q", got, filepath.Dir(joined))
	}
	if got := BaseName(joined); got != "c.txt" {
		t.Fatalf("BaseName in playground = %q, want %q", got, "c.txt")
	}
	if got := ExtName(joined); got != ".txt" {
		t.Fatalf("ExtName in playground = %q, want %q", got, ".txt")
	}
}

func TestPathHelpers(t *testing.T) {
	t.Run("PathJoin", func(t *testing.T) {
		tests := []struct {
			name  string
			parts []Value
			want  string
		}{
			{"empty", nil, ""},
			{"single", []Value{"foo"}, filepath.Join("foo")},
			{"multiple", []Value{"a", "b", "c.rip"}, filepath.Join("a", "b", "c.rip")},
			{"clean dotdot", []Value{"a", "b", "..", "c.rip"}, filepath.Join("a", "c.rip")},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				if got := PathJoin(tc.parts...); got != tc.want {
					t.Errorf("PathJoin(%v) = %q, want %q", tc.parts, got, tc.want)
				}
			})
		}
	})

	t.Run("PathJoin type mismatch", func(t *testing.T) {
		badArgs := []Value{int64(42), true, nil, NewMap()}
		for _, arg := range badArgs {
			err := catchFSError(t, func() { PathJoin("a", arg) })
			if err == nil {
				t.Fatalf("PathJoin(\"a\", %#v) expected panic, got none", arg)
			}
			if err.Kind != KindTypeMismatch {
				t.Fatalf("PathJoin(\"a\", %#v) kind = %q, want %q", arg, err.Kind, KindTypeMismatch)
			}
		}
	})

	t.Run("DirName BaseName ExtName", func(t *testing.T) {
		p := filepath.Join("dir", "sub", "archive.tar.gz")
		if got, want := DirName(p), filepath.Join("dir", "sub"); got != want {
			t.Errorf("DirName(%q) = %q, want %q", p, got, want)
		}
		if got := BaseName(p); got != "archive.tar.gz" {
			t.Errorf("BaseName(%q) = %q, want %q", p, got, "archive.tar.gz")
		}
		if got := ExtName(p); got != ".gz" {
			t.Errorf("ExtName(%q) = %q, want %q", p, got, ".gz")
		}
		if got := ExtName("README"); got != "" {
			t.Errorf("ExtName(\"README\") = %q, want \"\"", got)
		}
	})
}
