IMPORT uuid FROM GO "github.com/google/uuid"
IMPORT { NewString } FROM GO "github.com/google/uuid"

id1 = uuid.NewString()
id2 = NewString()
PRINT LEN(id1)
PRINT LEN(id2)
parsed = uuid.Parse(id1)
PRINT parsed.String() = id1

TRY
  bad = uuid.Parse("invalid")
  PRINT bad
CATCH e
  PRINT e.kind
END TRY
