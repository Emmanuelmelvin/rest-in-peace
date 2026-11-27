' links.test.bas — Unit tests for URL validation and short-code helpers.
IMPORT { ValidateURL, ValidateCode, GenerateCode } FROM "../links.bas"

TEST "ACCEPTS VALID HTTP AND HTTPS URLS"
  EXPECT ValidateURL("https://example.com/path?q=1") = "https://example.com/path?q=1"
  EXPECT ValidateURL("  http://rip.dev  ") = "http://rip.dev"
END TEST

TEST "REJECTS INVALID OR UNSAFE URLS"
  LET badScheme = FALSE
  TRY
    LET _ = ValidateURL("javascript:alert(1)")
  CATCH e
    badScheme = (e.status = 422)
  END TRY
  EXPECT badScheme = TRUE

  LET missingHost = FALSE
  TRY
    LET _ = ValidateURL("https://")
  CATCH e
    missingHost = (e.status = 422)
  END TRY
  EXPECT missingHost = TRUE
END TEST

TEST "VALIDATES CUSTOM CODES"
  EXPECT ValidateCode("my-link_1") = "my-link_1"

  LET tooShort = FALSE
  TRY
    LET _ = ValidateCode("ab")
  CATCH e
    tooShort = (e.status = 422)
  END TRY
  EXPECT tooShort = TRUE

  LET reserved = FALSE
  TRY
    LET _ = ValidateCode("api")
  CATCH e
    reserved = (e.status = 422)
  END TRY
  EXPECT reserved = TRUE
END TEST

TEST "GENERATES 7-CHARACTER CODES"
  LET c = GenerateCode()
  EXPECT LEN(c) = 7
END TEST
