'use strict';

// Reference data for completion, hover and signature help. It follows
// .kiro/steering/language.md and .kiro/specs/rip-basic/requirements.md.
// When either file changes, update this list and the grammar in
// syntaxes/ripbasic.tmLanguage.json together.
//
// mode: "classic" means the item is valid only in classic (line-numbered)
// files. Everything else is valid in both modes.

/** @typedef {{ name: string, params: string[], returns: string, doc: string, mode?: string, middleware?: boolean }} Builtin */

/** @type {Builtin[]} */
const builtins = [
  // Strings
  { name: 'LEN', params: ['value'], returns: 'INTEGER', doc: 'Number of characters in a STRING, items in an ARRAY, or keys in a MAP. `LEN(NIL)` is `0`.' },
  { name: 'UPPER', params: ['s'], returns: 'STRING', doc: 'The string in upper case.' },
  { name: 'LOWER', params: ['s'], returns: 'STRING', doc: 'The string in lower case.' },
  { name: 'TRIM', params: ['s'], returns: 'STRING', doc: 'The string with leading and trailing white space removed.' },
  { name: 'LEFT', params: ['s', 'n'], returns: 'STRING', doc: 'The first `n` characters of `s`.' },
  { name: 'RIGHT', params: ['s', 'n'], returns: 'STRING', doc: 'The last `n` characters of `s`.' },
  { name: 'MID', params: ['s', 'start', '[n]'], returns: 'STRING', doc: '`n` characters of `s` (or the rest of it) from position `start`. Positions count from 1, as in classic BASIC.' },
  { name: 'STR', params: ['value'], returns: 'STRING', doc: 'The value as text, the same way `PRINT` shows it.' },
  { name: 'VAL', params: ['s'], returns: 'NUMBER', doc: 'The number at the start of a STRING, or `0` if there is none. Leading and trailing spaces are ignored.' },

  // Numbers
  { name: 'INT', params: ['x'], returns: 'INTEGER', doc: 'The largest whole number not greater than `x`, so `INT(-2.5)` is `-3`.' },
  { name: 'ABS', params: ['x'], returns: 'NUMBER', doc: 'The absolute value of `x`. An INTEGER stays an INTEGER.' },
  { name: 'ROUND', params: ['x', 'places'], returns: 'DECIMAL', doc: 'Rounds `x` to `places` decimal places.' },
  { name: 'DECIMAL', params: ['s'], returns: 'DECIMAL', doc: 'An exact decimal value for money math, for example `DECIMAL("19.99")`. Supports `+ - * /` and comparisons. Mixing DECIMAL with NUMBER gives DECIMAL.' },
  { name: 'NOW', params: [], returns: 'DATE', doc: 'The current date and time.' },

  // Config
  { name: 'ENV', params: ['"NAME"', '[default]'], returns: 'STRING', doc: 'Reads an environment variable, or returns `default` if it is not set. `rip dev` loads a `.env` file.' },
  { name: 'ENVREQUIRE', params: ['"NAME"'], returns: 'STRING', doc: 'Reads an environment variable and stops startup with a clear error if it is missing.' },

  // Database
  { name: 'SQL', params: ['query', 'args…'], returns: 'ARRAY', doc: 'Runs a query and returns an array of row maps. Use `$1, $2…` placeholders; arguments are always sent as parameters.' },
  { name: 'SQLONE', params: ['query', 'args…'], returns: 'MAP', doc: 'Runs a query and returns the first row, or `NIL` if there are none. Use `$1, $2…` placeholders.' },
  { name: 'SQLEXEC', params: ['query', 'args…'], returns: 'INTEGER', doc: 'Runs a statement and returns the number of affected rows. Use `$1, $2…` placeholders.' },

  // HTTP
  { name: 'FETCH', params: ['url', '[options]'], returns: 'MAP', doc: 'Calls another API and returns `{"status", "headers", "body"}`, with JSON bodies parsed. Options: `method`, `headers`, `body`, `timeout` (default 10 s).' },
  { name: 'REQUEST', params: ['method', 'path', '[body]', '[headers]'], returns: 'MAP', doc: 'In tests: calls the app\'s own routes in-process, with no network, and returns `{status, headers, body}`.' },

  // Errors and Go interop
  { name: 'ERROR', params: ['"message"'], returns: 'ERROR', doc: 'A user error to `THROW`. The caught value exposes `e.message`, `e.kind`, `e.file` and `e.line`.' },
  { name: 'HTTPERROR', params: ['status', '"message"', '[details]'], returns: 'ERROR', doc: 'An HTTP error to `THROW`. If it escapes a handler, the response uses `status` and `{"error": message, "details": …}`.' },
  { name: 'NEW', params: ['pkg.Type'], returns: 'ANY', doc: 'Constructs the zero value of an imported Go type.' },

  // Classic-mode request helpers
  { name: 'PARAM', params: ['"name"'], returns: 'STRING', doc: 'Classic mode: a path parameter of the current request (`:name` in the route).', mode: 'classic' },
  { name: 'QUERY', params: ['"name"'], returns: 'STRING', doc: 'Classic mode: a query-string value of the current request.', mode: 'classic' },
  { name: 'HEADER', params: ['"name"'], returns: 'STRING', doc: 'Classic mode: a header of the current request. Names are lowercase.', mode: 'classic' },
  { name: 'BODY', params: [], returns: 'MAP', doc: 'Classic mode: the parsed JSON body of the current request, `{}` when there is none.', mode: 'classic' },

  // Built-in middleware
  { name: 'CORS', params: ['options'], returns: 'middleware', doc: 'Built-in middleware that adds CORS headers. Use with `USE`.', middleware: true },
  { name: 'RATELIMIT', params: ['perMinute'], returns: 'middleware', doc: 'Built-in middleware that limits requests per minute. Use with `USE`.', middleware: true },
  { name: 'REQUESTLOG', params: [], returns: 'middleware', doc: 'Built-in middleware that logs each request. Use with `USE`.', middleware: true },
  { name: 'APIKEY', params: ['"ENV_NAME"'], returns: 'middleware', doc: 'Built-in middleware that checks the `x-api-key` header against the secret in the named environment variable. Use with `USE`.', middleware: true },
  { name: 'BEARERJWT', params: ['"ENV_SECRET"'], returns: 'middleware', doc: 'Built-in middleware that verifies an HS256 JWT and sets `req.ctx.user`. Use with `USE`.', middleware: true },
];

/** @typedef {{ name: string, doc: string, syntax?: string, snippet?: string, mode?: string }} Keyword */

// Statement keywords. `snippet` uses VS Code snippet syntax and is inserted
// when the completion is picked at the start of a statement.
/** @type {Keyword[]} */
const keywords = [
  { name: 'LET', syntax: 'LET name = value', doc: 'Assigns a value. `LET` is optional.' },
  { name: 'PRINT', syntax: 'PRINT value', doc: 'Writes plain text to stdout (and to the playground log).' },
  { name: 'DIM', syntax: 'DIM name AS TYPE [= value]', doc: 'Declares a typed variable. Assigning a value of the wrong type raises `?TYPE MISMATCH ERROR`.', snippet: 'DIM ${1:name} AS ${2|NUMBER,INTEGER,STRING,BOOLEAN,MAP,ARRAY,DECIMAL,DATE,ANY|}' },
  { name: 'SHARED', syntax: 'SHARED DIM name AS TYPE', doc: 'Declares a variable shared by all requests.' },
  { name: 'CONST', syntax: 'CONST name = value', doc: 'Declares a value that cannot change.', snippet: 'CONST ${1:NAME} = ${2:value}' },
  { name: 'IF', syntax: 'IF cond THEN … [ELSEIF cond THEN …] [ELSE …] END IF', doc: 'Conditional. A single-line form `IF cond THEN stmt` also works.', snippet: 'IF ${1:condition} THEN\n\t$0\nEND IF' },
  { name: 'THEN', doc: 'Follows the condition of `IF` or `ELSEIF`.' },
  { name: 'ELSEIF', syntax: 'ELSEIF cond THEN', doc: 'Another branch of an `IF` block.' },
  { name: 'ELSE', doc: 'The fallback branch of an `IF` block.' },
  { name: 'END', syntax: 'END [IF|FUNCTION|SUB|SELECT|TRY|GROUP|TRANSACTION|TEST]', doc: 'Ends a block. On its own, `END` stops the program.' },
  { name: 'FOR', syntax: 'FOR i = a TO b [STEP s] … NEXT  ·  FOR EACH x IN arr … NEXT', doc: 'Counted loop, or a loop over the items of an array.', snippet: 'FOR ${1:i} = ${2:1} TO ${3:10}\n\t$0\nNEXT' },
  { name: 'EACH', syntax: 'FOR EACH x IN arr', doc: 'Loops over the items of an array.' },
  { name: 'IN', doc: 'Used in `FOR EACH x IN arr`.' },
  { name: 'TO', doc: 'Used in `FOR i = a TO b`.' },
  { name: 'STEP', doc: 'The increment of a `FOR` loop.' },
  { name: 'NEXT', doc: 'Ends a `FOR` loop.' },
  { name: 'WHILE', syntax: 'WHILE cond … WEND', doc: 'Loops while the condition is true.', snippet: 'WHILE ${1:condition}\n\t$0\nWEND' },
  { name: 'WEND', doc: 'Ends a `WHILE` loop.' },
  { name: 'DO', syntax: 'DO … LOOP [UNTIL cond]', doc: 'Loops until the condition is true, or forever.', snippet: 'DO\n\t$0\nLOOP UNTIL ${1:condition}' },
  { name: 'LOOP', doc: 'Ends a `DO` loop.' },
  { name: 'UNTIL', doc: 'The stop condition of `DO … LOOP UNTIL`.' },
  { name: 'SELECT', syntax: 'SELECT CASE x … CASE 1, 2 … CASE ELSE … END SELECT', doc: 'Picks a branch by value.', snippet: 'SELECT CASE ${1:value}\n\tCASE ${2:1}\n\t\t$0\n\tCASE ELSE\n\t\t\nEND SELECT' },
  { name: 'CASE', doc: 'A branch of `SELECT CASE`. `CASE ELSE` is the fallback.' },
  { name: 'EXIT', syntax: 'EXIT FOR|WHILE|DO', doc: 'Leaves the innermost loop of that kind.' },
  { name: 'FUNCTION', syntax: 'FUNCTION Name(param [AS TYPE] [= default], …) [AS TYPE] … END FUNCTION', doc: 'Declares a function. Use `RETURN value`. Functions can be called before their definition.', snippet: 'FUNCTION ${1:Name}(${2:params})\n\t$0\nEND FUNCTION' },
  { name: 'SUB', syntax: 'SUB Name(params) … END SUB', doc: 'Declares a procedure with no return value.', snippet: 'SUB ${1:Name}(${2:params})\n\t$0\nEND SUB' },
  { name: 'RETURN', syntax: 'RETURN [value]', doc: 'Returns from a function. In a structured handler, `RETURN v` responds `200` with `v`. In classic mode, returns from a `GOSUB`.' },
  { name: 'EXPORT', syntax: 'EXPORT FUNCTION|SUB|CONST …', doc: 'Makes a declaration importable from other files.' },
  { name: 'IMPORT', syntax: 'IMPORT "./file.bas"  ·  IMPORT { A, B } FROM "./file.bas"  ·  IMPORT pkg FROM GO "path"', doc: 'Imports another `.bas` file\'s exports, or a Go package. Braces always mean named imports.', snippet: 'IMPORT { ${1:Name} } FROM "${2:./file.bas}"' },
  { name: 'FROM', doc: 'Used in `IMPORT … FROM "./file.bas"` and `IMPORT … FROM GO "path"`.' },
  { name: 'GO', doc: 'Used in `IMPORT pkg FROM GO "github.com/…"` to import a Go package.' },
  { name: 'TRY', syntax: 'TRY … CATCH e … [FINALLY …] END TRY', doc: 'Catches errors. `e` exposes `e.message`, `e.kind`, `e.file`, `e.line`, and `e.status` for HTTP errors. `RESPOND` is never caught.', snippet: 'TRY\n\t$0\nCATCH ${1:e}\n\t\nEND TRY' },
  { name: 'CATCH', syntax: 'CATCH e', doc: 'Handles an error raised in the `TRY` block.' },
  { name: 'FINALLY', doc: 'Runs after `TRY`/`CATCH`, including after `RESPOND`.' },
  { name: 'THROW', syntax: 'THROW ERROR("msg")  ·  THROW HTTPERROR(status, "msg" [, details])', doc: 'Raises an error.' },
  { name: 'ROUTE', syntax: 'ROUTE <METHOD> "<path>" CALL Fn  ·  ROUTE <METHOD> "<path>" GOSUB n  ·  ROUTE GROUP "/p" … END GROUP', doc: 'Declares an HTTP route. Methods: `GET POST PUT PATCH DELETE`. Paths support `:params` and a trailing `*`.', snippet: 'ROUTE ${1|GET,POST,PUT,PATCH,DELETE|} "${2:/path}" CALL ${3:Handler}' },
  { name: 'GROUP', syntax: 'ROUTE GROUP "/prefix" … END GROUP', doc: 'Prefixes the routes inside it.' },
  { name: 'CALL', doc: 'Names the handler function of a `ROUTE`.' },
  { name: 'USE', syntax: 'USE Fn [ON "/path/*"]', doc: 'Registers middleware that runs, in order, before every handler (or only on matching paths).' },
  { name: 'ON', doc: 'Scopes `USE` to a path, names a connection in `TRANSACTION ON`, or (classic) `ON x GOTO a, b, c`.' },
  { name: 'RESPOND', syntax: 'RESPOND status, value [, headers]', doc: 'Sends the response and ends the handler. Never caught by `CATCH`; runs `FINALLY`; commits `TRANSACTION`.', snippet: 'RESPOND ${1:200}, ${2:value}' },
  { name: 'SERVE', syntax: 'SERVE [port]', doc: 'Starts the server after all top-level statements have run. The port defaults to `ENV("PORT")` or `8080`.' },
  { name: 'DBCONNECT', syntax: 'DBCONNECT url [AS name]', doc: 'Opens a connection pool. v1 supports `postgres://…` and `sqlite://…` (a file or `:memory:`).', snippet: 'DBCONNECT ENV("${1:DATABASE_URL}")' },
  { name: 'TRANSACTION', syntax: 'TRANSACTION [ON name] … END TRANSACTION', doc: 'Commits on normal exit (including `RESPOND`) and rolls back if an error escapes. Every SQL call inside uses the transaction.', snippet: 'TRANSACTION\n\t$0\nEND TRANSACTION' },
  { name: 'LOG', syntax: 'LOG level, msg [, map]', doc: 'Writes one JSON log line. Levels: `DEBUG INFO WARN ERROR`. Secret-like fields are redacted.', snippet: 'LOG ${1|INFO,DEBUG,WARN,ERROR|}, "${2:message}"' },
  { name: 'TEST', syntax: 'TEST "name" … END TEST', doc: 'A test block, allowed in `*.test.bas` files.', snippet: 'TEST "${1:name}"\n\t$0\nEND TEST' },
  { name: 'EXPECT', syntax: 'EXPECT cond [, "msg"]', doc: 'Fails the test with the source location if the condition is false.' },
  { name: 'AS', doc: 'Gives a type: `DIM x AS STRING`, a parameter or return type, or a connection name in `DBCONNECT url AS name`.' },
  { name: 'REM', doc: 'A comment to the end of the line. `\'` also starts a comment.' },
  { name: 'AND', doc: 'Logical and. Short-circuits.' },
  { name: 'OR', doc: 'Logical or. Short-circuits.' },
  { name: 'NOT', doc: 'Logical not.' },
  { name: 'MOD', doc: 'Remainder of division.' },
  { name: 'TRUE', doc: 'The BOOLEAN true.' },
  { name: 'FALSE', doc: 'The BOOLEAN false.' },
  { name: 'NIL', doc: 'No value. A missing map key is `NIL`; reading a member of `NIL` raises `?NIL ACCESS ERROR`.' },

  // Classic mode only
  { name: 'GOTO', syntax: 'GOTO line', doc: 'Classic mode: jumps to a line number. A compile error in structured mode.', mode: 'classic' },
  { name: 'GOSUB', syntax: 'GOSUB line', doc: 'Classic mode: calls the subroutine at a line number; `RETURN` comes back. A compile error in structured mode.', mode: 'classic' },
];

// Route methods and log levels are offered after ROUTE and LOG.
const httpMethods = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE'];
const logLevels = ['DEBUG', 'INFO', 'WARN', 'ERROR'];

/** @type {{ name: string, doc: string }[]} */
const types = [
  { name: 'NUMBER', doc: '64-bit floating point number.' },
  { name: 'INTEGER', doc: '64-bit whole number.' },
  { name: 'STRING', doc: 'Text.' },
  { name: 'BOOLEAN', doc: '`TRUE` or `FALSE`.' },
  { name: 'MAP', doc: 'Key/value map that keeps insertion order, written `{"k": v}`. Keys are case-sensitive.' },
  { name: 'ARRAY', doc: 'Zero-based list, written `[a, b]`.' },
  { name: 'DECIMAL', doc: 'Exact decimal for money math. Database NUMERIC/DECIMAL columns map to it.' },
  { name: 'DATE', doc: 'A date and time.' },
  { name: 'NIL', doc: 'No value.' },
  { name: 'ANY', doc: 'Any value (the default when no type is given).' },
];

module.exports = { builtins, keywords, types, httpMethods, logLevels };
