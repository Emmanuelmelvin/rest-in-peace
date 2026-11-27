' ENVREQUIRE on a missing variable stops startup with ?UNDEFINED ERROR (Requirement 10.1).
LET secret = ENVREQUIRE("RIP_BASH_DEFINITELY_UNSET_999")
PRINT secret
