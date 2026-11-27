package rt

import (
	"errors"
	"os"
	"path/filepath"
	"time"
)

// checkFSAllowed blocks file-system I/O inside the hosted playground
// (RIP_PLAYGROUND=1).
func checkFSAllowed(name string) {
	if isPlayground() {
		panic(Errorf(KindNotAllowed, "%s is not allowed in the playground — use the rip CLI", name))
	}
}

// ReadFile implements READFILE(path): reads the entire file as a STRING.
func ReadFile(path string) string {
	checkFSAllowed("READFILE")
	b, err := os.ReadFile(path)
	if err != nil {
		panic(Errorf(KindInternal, "READFILE %q: %v", path, err))
	}
	return string(b)
}

// WriteFile implements WRITEFILE(path, data): writes data to path (mode 0644),
// creating or truncating the file.
func WriteFile(path string, data string) Value {
	checkFSAllowed("WRITEFILE")
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		panic(Errorf(KindInternal, "WRITEFILE %q: %v", path, err))
	}
	return nil
}

// AppendFile implements APPENDFILE(path, data): appends data to path, creating
// the file (mode 0644) if it does not exist.
func AppendFile(path string, data string) Value {
	checkFSAllowed("APPENDFILE")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		panic(Errorf(KindInternal, "APPENDFILE %q: %v", path, err))
	}
	if _, err := f.WriteString(data); err != nil {
		_ = f.Close()
		panic(Errorf(KindInternal, "APPENDFILE %q: %v", path, err))
	}
	if err := f.Close(); err != nil {
		panic(Errorf(KindInternal, "APPENDFILE %q: %v", path, err))
	}
	return nil
}

// FileExists implements FILEEXISTS(path): reports whether path exists.
func FileExists(path string) bool {
	checkFSAllowed("FILEEXISTS")
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	if errors.Is(err, os.ErrNotExist) {
		return false
	}
	panic(Errorf(KindInternal, "FILEEXISTS %q: %v", path, err))
}

// DeleteFile implements DELETEFILE(path): removes a file or empty directory.
func DeleteFile(path string) Value {
	checkFSAllowed("DELETEFILE")
	if err := os.Remove(path); err != nil {
		panic(Errorf(KindInternal, "DELETEFILE %q: %v", path, err))
	}
	return nil
}

// Mkdir implements MKDIR(path): creates a directory and any missing parents
// (mode 0755).
func Mkdir(path string) Value {
	checkFSAllowed("MKDIR")
	if err := os.MkdirAll(path, 0o755); err != nil {
		panic(Errorf(KindInternal, "MKDIR %q: %v", path, err))
	}
	return nil
}

// ReadDir implements READDIR(path): returns an ARRAY of entry names sorted
// alphabetically.
func ReadDir(path string) *Array {
	checkFSAllowed("READDIR")
	entries, err := os.ReadDir(path)
	if err != nil {
		panic(Errorf(KindInternal, "READDIR %q: %v", path, err))
	}
	items := make([]Value, len(entries))
	for i, entry := range entries {
		items[i] = entry.Name()
	}
	return &Array{Items: items}
}

// FileStat implements FILESTAT(path): returns a MAP with "name", "size",
// "isDir", and "modTime" (UTC RFC3339).
func FileStat(path string) *Map {
	checkFSAllowed("FILESTAT")
	info, err := os.Stat(path)
	if err != nil {
		panic(Errorf(KindInternal, "FILESTAT %q: %v", path, err))
	}
	return MapOf(
		"name", info.Name(),
		"size", info.Size(),
		"isDir", info.IsDir(),
		"modTime", info.ModTime().UTC().Format(time.RFC3339),
	)
}

// PathJoin implements PATHJOIN(parts...): joins path segments using the OS
// separator. Pure string operation; allowed in the playground.
func PathJoin(parts ...Value) string {
	strs := make([]string, len(parts))
	for i, p := range parts {
		s, ok := p.(string)
		if !ok {
			panic(Errorf(KindTypeMismatch, "PATHJOIN needs STRING arguments, got %s", TypeName(p)))
		}
		strs[i] = s
	}
	return filepath.Join(strs...)
}

// DirName implements DIRNAME(p): returns all but the last element of path p.
func DirName(p string) string {
	return filepath.Dir(p)
}

// BaseName implements BASENAME(p): returns the last element of path p.
func BaseName(p string) string {
	return filepath.Base(p)
}

// ExtName implements EXTNAME(p): returns the file name extension used by p.
func ExtName(p string) string {
	return filepath.Ext(p)
}
