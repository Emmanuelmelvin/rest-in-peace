import type { StreamParser } from "@codemirror/language";

// Keep this list in sync with .kiro/steering/language.md.
const keywords = new Set(
  (
    "AND AS CALL CASE CATCH CONST DBCONNECT DIM DO EACH ELSE ELSEIF END EXIT EXPECT EXPORT " +
    "FALSE FINALLY FOR FROM FUNCTION GO GOSUB GOTO GROUP IF IMPORT IN LET LOG LOOP MOD NEXT NIL " +
    "NOT ON OR PRINT RESPOND RETURN ROUTE SELECT SERVE SHARED STEP SUB TEST THEN THROW TO " +
    "TRANSACTION TRUE TRY UNTIL USE WEND WHILE GET POST PUT PATCH DELETE"
  ).split(" "),
);

// ripBasic is a CodeMirror stream mode for RIP BASIC syntax highlighting.
export const ripBasic: StreamParser<Record<string, never>> = {
  name: "ripbasic",
  startState: () => ({}),
  token(stream) {
    if (stream.sol() && stream.match(/^\d+/)) return "labelName"; // classic line number
    if (stream.eatSpace()) return null;
    if (stream.match(/^'.*/) || stream.match(/^REM\b.*/i)) return "comment";
    if (stream.match(/^"(?:[^"\\]|\\.)*"?/)) return "string";
    if (stream.match(/^\d+(\.\d+)?(e[+-]?\d+)?/i)) return "number";
    const word = stream.match(/^[A-Za-z_][A-Za-z0-9_]*/);
    if (word && typeof word !== "boolean") {
      return keywords.has(word[0].toUpperCase()) ? "keyword" : "variableName";
    }
    stream.next();
    return "operator";
  },
};
