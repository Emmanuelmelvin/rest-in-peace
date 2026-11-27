' ENV with default, missing ENV -> NIL, and ENVREQUIRE when set (Requirement 10.1).
IMPORT os FROM GO "os"

PRINT ENV("RIP_BASH_UNSET_10", "fallback")
PRINT ENV("RIP_BASH_UNSET_10") = NIL

os.Setenv("RIP_BASH_SET_10", "alive")
PRINT ENV("RIP_BASH_SET_10")
PRINT ENV("RIP_BASH_SET_10", "ignored")
PRINT ENVREQUIRE("RIP_BASH_SET_10")
