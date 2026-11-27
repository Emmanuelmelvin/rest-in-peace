' UTF-8 strings/keys, case-insensitive keywords/vars/functions,
' and case-sensitive map keys & member access (Requirement 1.1, 1.4, 1.5).
REM Standalone REM comment
MyVal = "café"
pRiNt MYVAL & " 👻"

LET obj = {"Name": "Ada", "name": "ada", "café": 42}
PRINT obj.Name
PRINT obj.name
PRINT obj["café"]

fUnCtIoN AddOne(x)
  rEtUrN X + 1
eNd fUnCtIoN

PRINT addone(41) ' inline comment
