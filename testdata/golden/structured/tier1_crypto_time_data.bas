' Tier 1: Crypto, Date/Time & Core Data utilities
PRINT LEN(UUID())
PRINT LEN(RANDOMHEX(8))
PRINT HASH("sha256", "hello")
PRINT HMAC("sha256", "secret", "msg")

pwHash = HASHPASSWORD("s3cr3t!")
PRINT VERIFYPASSWORD("s3cr3t!", pwHash)
PRINT VERIFYPASSWORD("wrong", pwHash)

enc = BASE64ENCODE("REST in Peace")
PRINT enc
PRINT BASE64DECODE(enc)

PRINT LEN(NOW()) >= 20
PRINT UNIXTIME() > 1700000000
PRINT UNIXMS() > 1700000000000
ts = PARSEDATE("2026-11-27T12:34:56Z")
PRINT FORMATDATE(ts, "DATE")
PRINT FORMATDATE(ts, "TIME")

obj = JSONPARSE("{\"b\":1,\"a\":[2,3]}")
PRINT JSONSTRINGIFY(obj)
PRINT JOIN(KEYS(obj), ",")
PRINT LEN(VALUES(obj))

parts = SPLIT("one,two,three", ",")
PRINT LEN(parts)
PRINT JOIN(parts, "-")
PRINT REPLACE("foo bar foo", "foo", "baz")
PRINT CONTAINS("hello world", "world")
PRINT CONTAINS(parts, "two")
PRINT CONTAINS(obj, "a")
PRINT STARTSWITH("rip-basic", "rip")
PRINT ENDSWITH("rip-basic", "basic")
PRINT INDEXOF("hello", "ll")
PRINT INDEXOF(parts, "three")

arr = [3, 1, 2]
PRINT PUSH(arr, 4)
PRINT POP(arr)
PRINT JOIN(SORT(arr), ",")
PRINT JOIN(SLICE(arr, 0, 2), ",")

PRINT REGEXMATCH("^[a-z]+-[0-9]+$", "inv-42")
PRINT REGEXREPLACE("[0-9]+", "item 123 and 456", "X")
