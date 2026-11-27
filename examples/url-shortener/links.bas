' links.bas — URL validation, custom slug checks, and short-code generation.
IMPORT url FROM GO "net/url"
IMPORT regexp FROM GO "regexp"
IMPORT strings FROM GO "strings"
IMPORT { NewString } FROM GO "github.com/google/uuid"

EXPORT FUNCTION ValidateURL(raw) AS STRING
  IF raw = NIL THEN
    THROW HTTPERROR(422, "URL IS REQUIRED")
  END IF

  TRY
    DIM rawStr AS STRING = raw
  CATCH e
    THROW HTTPERROR(422, "URL MUST BE A STRING")
  END TRY

  LET cleaned = TRIM(raw)
  IF LEN(cleaned) = 0 THEN
    THROW HTTPERROR(422, "URL IS REQUIRED")
  END IF

  TRY
    LET parsed = url.ParseRequestURI(cleaned)
    IF parsed.Scheme <> "http" AND parsed.Scheme <> "https" THEN
      THROW HTTPERROR(422, "URL MUST USE HTTP OR HTTPS")
    END IF
    IF parsed.Host = "" THEN
      THROW HTTPERROR(422, "URL MUST INCLUDE A HOST")
    END IF
  CATCH e
    IF e.kind = "HTTP" THEN THROW e
    THROW HTTPERROR(422, "INVALID URL FORMAT")
  END TRY

  RETURN cleaned
END FUNCTION

EXPORT FUNCTION ValidateCode(raw) AS STRING
  TRY
    DIM rawStr AS STRING = raw
  CATCH e
    THROW HTTPERROR(422, "CUSTOM CODE MUST BE A STRING")
  END TRY

  LET code = TRIM(raw)
  IF LEN(code) < 3 OR LEN(code) > 32 THEN
    THROW HTTPERROR(422, "CUSTOM CODE MUST BE 3 TO 32 CHARACTERS")
  END IF

  LET lower = strings.ToLower(code)
  IF lower = "api" OR lower = "_health" THEN
    THROW HTTPERROR(422, "CUSTOM CODE IS RESERVED")
  END IF

  LET ok = regexp.MatchString("^[a-zA-Z0-9_-]+$", code)
  IF NOT ok THEN
    THROW HTTPERROR(422, "CUSTOM CODE MAY ONLY CONTAIN LETTERS, NUMBERS, HYPHENS, AND UNDERSCORES")
  END IF

  RETURN code
END FUNCTION

EXPORT FUNCTION GenerateCode() AS STRING
  LET compact = strings.ReplaceAll(NewString(), "-", "")
  RETURN LEFT(compact, 7)
END FUNCTION
