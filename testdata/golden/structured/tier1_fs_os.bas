' Tier 1: File system, path & OS built-ins
PRINT BASENAME(PATHJOIN("a", "b", "hello.bas"))
PRINT EXTNAME("invoice.pdf")
PRINT BASENAME(DIRNAME(PATHJOIN("a", "b", "c.txt")))

PRINT LEN(PLATFORM()) > 0
PRINT CPUS() >= 1
PRINT PID() > 0
PRINT LEN(CWD()) > 0

dir = PATHJOIN(CWD(), ".rip_tier1_golden_tmp_" & UUID())
MKDIR(dir)
f = PATHJOIN(dir, "note.txt")
WRITEFILE(f, "HELLO")
APPENDFILE(f, " WORLD")
PRINT FILEEXISTS(f)
PRINT READFILE(f)

st = FILESTAT(f)
PRINT st.name
PRINT st.size
PRINT st.isDir

entries = READDIR(dir)
PRINT LEN(entries)
PRINT entries[0]

DELETEFILE(f)
DELETEFILE(dir)
PRINT FILEEXISTS(f)
