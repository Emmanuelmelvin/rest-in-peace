'use strict';

// Text analysis for completion, hover and signature help. This file does not
// depend on the vscode module, so it can be tested with plain `node --test`.
// It reads one file at a time with regular expressions; it is not a parser.

const LINE_NUMBER = /^\s*\d+(?:\s|$)/;
const COMMENT_LINE = /^\s*(?:'|REM\b)/i;

/** True when the file is in classic mode: its first non-blank, non-comment line starts with a line number. */
function isClassic(text) {
  for (const line of text.split(/\r?\n/)) {
    if (line.trim() === '' || COMMENT_LINE.test(line)) continue;
    return LINE_NUMBER.test(line);
  }
  return false;
}

/**
 * Splits a line prefix (the text before the cursor) into code and what the
 * cursor is inside of. Strings are replaced with blanks in `code` so that
 * regular expressions never match inside them.
 * @returns {{ code: string, inString: boolean, inComment: boolean }}
 */
function scanPrefix(prefix) {
  let code = '';
  let inString = false;
  for (let i = 0; i < prefix.length; i++) {
    const ch = prefix[i];
    if (inString) {
      if (ch === '"') inString = false;
      code += ch === '"' ? '"' : ' ';
      continue;
    }
    if (ch === '"') {
      inString = true;
      code += '"';
      continue;
    }
    if (ch === "'") return { code, inString: false, inComment: true };
    code += ch;
  }
  // REM at the start of a statement: after an optional line number or a `:`.
  const inComment = /(?:^|:)\s*(?:\d+\s+)?REM(?:\s|$)/i.test(code);
  return { code, inString, inComment };
}

/**
 * Describes where the cursor is, for choosing which completions to offer.
 * @returns {'none'|'type'|'method'|'logLevel'|'middleware'|'member'|'statement'|'expression'}
 */
function completionContext(prefix) {
  const { code, inString, inComment } = scanPrefix(prefix);
  if (inString || inComment) return 'none';
  // Strip the word being typed, so the rules below look at what comes before it.
  const before = code.replace(/[A-Za-z_][A-Za-z0-9_]*$/, '');
  if (/\.\s*$/.test(before)) return 'member';
  if (/\bAS\s+$/i.test(before)) return 'type';
  if (/\bROUTE\s+$/i.test(before)) return 'method';
  if (/\bLOG\s+$/i.test(before)) return 'logLevel';
  if (/\bUSE\s+$/i.test(before)) return 'middleware';
  // A line number is being typed after GOTO/GOSUB (or ON x GOTO a, b).
  if (/\b(?:GOTO|GOSUB)\s+(?:\d+\s*,\s*)*\d*$/i.test(code)) return 'none';
  if (/(?:^|:)\s*(?:\d+\s+)?$/.test(before) || /\b(?:THEN|ELSE)\s+$/i.test(before)) return 'statement';
  return 'expression';
}

/**
 * @typedef {{ name: string, kind: 'function'|'sub'|'variable'|'const'|'import', line: number, detail: string, params?: string[], exported?: boolean }} Symbol
 */

/**
 * Finds the names declared in one file: functions, subs, DIM and CONST
 * declarations, imported names, FOR loop variables, CATCH variables and
 * function parameters. Names are case-insensitive, so the first declaration
 * of each name wins.
 * @returns {Symbol[]}
 */
function scanSymbols(text) {
  /** @type {Map<string, Symbol>} */
  const seen = new Map();
  const add = (sym) => {
    const key = sym.name.toUpperCase();
    if (!seen.has(key)) seen.set(key, sym);
  };
  const lines = text.split(/\r?\n/);
  lines.forEach((raw, line) => {
    const { code } = scanPrefix(raw);
    // Statements on one line are separated by `:` (mostly classic mode).
    for (let stmt of code.split(':')) {
      stmt = stmt.replace(LINE_NUMBER, '').trim();
      let m;
      if ((m = /^(EXPORT\s+)?(FUNCTION|SUB)\s+([A-Za-z_]\w*)\s*(?:\(([^)]*)\))?\s*(?:AS\s+([A-Za-z_]\w*))?/i.exec(stmt))) {
        const kind = m[2].toUpperCase() === 'SUB' ? 'sub' : 'function';
        // Parameter text comes from the raw line so default strings survive.
        const rawParams = paramsFromRaw(raw, m[3]);
        const params = rawParams ? splitTopLevel(rawParams).map((p) => p.trim()).filter(Boolean) : [];
        const ret = m[5] ? ` AS ${m[5].toUpperCase()}` : '';
        add({
          name: m[3],
          kind,
          line,
          params,
          exported: Boolean(m[1]),
          detail: `${m[1] ? 'EXPORT ' : ''}${kind.toUpperCase()} ${m[3]}(${params.join(', ')})${ret}`,
        });
        for (const p of params) {
          const pm = /^([A-Za-z_]\w*)(?:\s+AS\s+([A-Za-z_]\w*))?/i.exec(p);
          if (pm) add({ name: pm[1], kind: 'variable', line, detail: `parameter ${pm[1]}${pm[2] ? ` AS ${pm[2].toUpperCase()}` : ''}` });
        }
        continue;
      }
      if ((m = /^(SHARED\s+)?DIM\s+([A-Za-z_]\w*)(?:\s+AS\s+([A-Za-z_]\w*))?/i.exec(stmt))) {
        add({ name: m[2], kind: 'variable', line, detail: `${m[1] ? 'SHARED ' : ''}DIM ${m[2]}${m[3] ? ` AS ${m[3].toUpperCase()}` : ''}` });
        continue;
      }
      if ((m = /^(EXPORT\s+)?CONST\s+([A-Za-z_]\w*)/i.exec(stmt))) {
        add({ name: m[2], kind: 'const', line, exported: Boolean(m[1]), detail: `${m[1] ? 'EXPORT ' : ''}CONST ${m[2]}` });
        continue;
      }
      if ((m = /^IMPORT\s*\{([^}]*)\}\s*FROM\s*(GO\s*)?"?/i.exec(stmt))) {
        const from = importSource(raw);
        for (const n of m[1].split(',').map((s) => s.trim()).filter(Boolean)) {
          add({ name: n, kind: 'import', line, detail: `IMPORT { ${n} } FROM ${m[2] ? 'GO ' : ''}"${from}"` });
        }
        continue;
      }
      if ((m = /^IMPORT\s+([A-Za-z_]\w*)\s+FROM\s+GO\b/i.exec(stmt))) {
        add({ name: m[1], kind: 'import', line, detail: `IMPORT ${m[1]} FROM GO "${importSource(raw)}"` });
        continue;
      }
      if ((m = /^FOR\s+(?:EACH\s+)?([A-Za-z_]\w*)/i.exec(stmt))) {
        add({ name: m[1], kind: 'variable', line, detail: `loop variable ${m[1]}` });
        continue;
      }
      if ((m = /^CATCH\s+([A-Za-z_]\w*)/i.exec(stmt))) {
        add({ name: m[1], kind: 'variable', line, detail: `caught error ${m[1]} (.message, .kind, .file, .line, .status)` });
        continue;
      }
      if ((m = /^(?:LET\s+)?([A-Za-z_]\w*)\s*=(?!=)/i.exec(stmt))) {
        add({ name: m[1], kind: 'variable', line, detail: `variable ${m[1]}` });
      }
    }
  });
  return [...seen.values()];
}

// The parameter list of FUNCTION/SUB `name` on a raw source line.
function paramsFromRaw(raw, name) {
  const at = raw.search(new RegExp(`\\b${name}\\s*\\(`, 'i'));
  if (at < 0) return '';
  const open = raw.indexOf('(', at);
  let depth = 0;
  let inString = false;
  for (let i = open; i < raw.length; i++) {
    const ch = raw[i];
    if (ch === '"') inString = !inString;
    if (inString) continue;
    if (ch === '(') depth++;
    if (ch === ')' && --depth === 0) return raw.slice(open + 1, i);
  }
  return raw.slice(open + 1);
}

function importSource(raw) {
  const m = /FROM\s+(?:GO\s+)?"([^"]*)"/i.exec(raw);
  return m ? m[1] : '';
}

// Splits on commas that are not inside brackets or strings.
function splitTopLevel(s) {
  const parts = [];
  let depth = 0;
  let inString = false;
  let start = 0;
  for (let i = 0; i < s.length; i++) {
    const ch = s[i];
    if (ch === '"') inString = !inString;
    if (inString) continue;
    if ('([{'.includes(ch)) depth++;
    else if (')]}'.includes(ch)) depth--;
    else if (ch === ',' && depth === 0) {
      parts.push(s.slice(start, i));
      start = i + 1;
    }
  }
  parts.push(s.slice(start));
  return parts;
}

/**
 * Finds the function call the cursor is inside of, for signature help.
 * @returns {{ name: string, argIndex: number } | null}
 */
function callAt(prefix) {
  const { code, inComment } = scanPrefix(prefix);
  if (inComment) return null;
  let depth = 0;
  let commas = 0;
  let inString = false;
  for (let i = code.length - 1; i >= 0; i--) {
    const ch = code[i];
    if (ch === '"') {
      inString = !inString;
      continue;
    }
    if (inString) continue;
    if (')]}'.includes(ch)) depth++;
    else if ('[{'.includes(ch)) {
      if (depth === 0) return null;
      depth--;
    } else if (ch === '(') {
      if (depth > 0) {
        depth--;
        continue;
      }
      const m = /([A-Za-z_][\w.]*)\s*$/.exec(code.slice(0, i));
      if (!m) return null;
      // A Go call such as uuid.NewString( is not ours to describe.
      if (m[1].includes('.')) return null;
      return { name: m[1], argIndex: commas };
    } else if (ch === ',' && depth === 0) commas++;
  }
  return null;
}

/** The identifier at `col` in `line`, and whether it follows a `.` (a member name). */
function wordAt(line, col) {
  const re = /[A-Za-z_][A-Za-z0-9_]*/g;
  let m;
  while ((m = re.exec(line))) {
    if (col >= m.index && col <= m.index + m[0].length) {
      const member = /\.\s*$/.test(line.slice(0, m.index));
      return { word: m[0], start: m.index, end: m.index + m[0].length, member };
    }
  }
  return null;
}

module.exports = { isClassic, scanPrefix, completionContext, scanSymbols, callAt, wordAt, splitTopLevel };
