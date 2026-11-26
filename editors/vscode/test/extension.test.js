'use strict';

// Runs the providers against a small stand-in for the vscode module, so the
// glue code is checked without starting VS Code.

const test = require('node:test');
const assert = require('node:assert/strict');
const Module = require('node:module');

class MarkdownString {
  constructor(v = '') { this.value = v; }
  appendMarkdown(s) { this.value += s; return this; }
  appendCodeblock(s, lang) { this.value += `\n\`\`\`${lang}\n${s}\n\`\`\`\n`; return this; }
}
const vscodeStub = {
  CompletionItem: class { constructor(label, kind) { this.label = label; this.kind = kind; } },
  CompletionItemKind: { Function: 'fn', Keyword: 'kw', Snippet: 'snip', Variable: 'var', Constant: 'const', Module: 'mod', TypeParameter: 'type', EnumMember: 'enum' },
  SnippetString: class { constructor(v) { this.value = v; } },
  MarkdownString,
  Range: class { constructor(...a) { this.a = a; } },
  Hover: class { constructor(contents, range) { this.contents = contents; this.range = range; } },
  SignatureInformation: class { constructor(label, doc) { this.label = label; this.documentation = doc; } },
  ParameterInformation: class { constructor(label) { this.label = label; } },
  SignatureHelp: class {},
  languages: {},
};
const resolve = Module._resolveFilename;
Module._resolveFilename = function (req, ...rest) {
  return req === 'vscode' ? 'vscode' : resolve.call(this, req, ...rest);
};
require.cache.vscode = { id: 'vscode', filename: 'vscode', loaded: true, exports: vscodeStub };

const ext = require('../src/extension');

function doc(text) {
  const lines = text.split('\n');
  return { getText: () => text, lineAt: (n) => ({ text: lines[n] }) };
}
// `|` marks the cursor.
function at(text) {
  const lines = text.split('\n');
  const line = lines.findIndex((l) => l.includes('|'));
  const character = lines[line].indexOf('|');
  lines[line] = lines[line].replace('|', '');
  return [doc(lines.join('\n')), { line, character }];
}
const labels = (items) => items.map((i) => i.label);

test('structured files get built-ins, keywords, snippets and local names, but no classic-only items', () => {
  const items = ext.completionProvider.provideCompletionItems(...at('FUNCTION Total(a, b = 1)\nEND FUNCTION\n|'));
  const l = labels(items);
  for (const want of ['LEN', 'SQLONE', 'IF', 'IF …', 'ROUTE …', 'Total']) assert.ok(l.includes(want), want);
  for (const not of ['GOTO', 'GOSUB', 'PARAM', 'BODY', 'CORS']) assert.ok(!l.includes(not), not);
  const total = items.find((i) => i.label === 'Total');
  assert.equal(total.insertText.value, 'Total(${1:a})');
  assert.equal(items.find((i) => i.label === 'MID').insertText.value, 'MID(${1:s}, ${2:start})');
  assert.equal(items.find((i) => i.label === 'NOW').insertText.value, 'NOW()');
});

test('classic files also get GOTO, GOSUB and the request helpers', () => {
  const l = labels(ext.completionProvider.provideCompletionItems(...at('10 PRINT "HI"\n20 |')));
  for (const want of ['GOTO', 'GOSUB', 'PARAM', 'BODY']) assert.ok(l.includes(want), want);
});

test('block snippets are offered only at the start of a statement', () => {
  const l = labels(ext.completionProvider.provideCompletionItems(...at('x = 1 + |')));
  assert.ok(l.includes('LEN'));
  assert.ok(!l.includes('IF …'));
});

test('contexts narrow the list', () => {
  assert.deepEqual(labels(ext.completionProvider.provideCompletionItems(...at('DIM x AS |'))),
    ['NUMBER', 'INTEGER', 'STRING', 'BOOLEAN', 'MAP', 'ARRAY', 'DECIMAL', 'DATE', 'NIL', 'ANY']);
  assert.deepEqual(labels(ext.completionProvider.provideCompletionItems(...at('ROUTE |'))),
    ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'GROUP']);
  assert.deepEqual(labels(ext.completionProvider.provideCompletionItems(...at('SUB Auth(req)\nEND SUB\nUSE |'))),
    ['CORS', 'RATELIMIT', 'REQUESTLOG', 'APIKEY', 'BEARERJWT', 'Auth']);
  assert.deepEqual(ext.completionProvider.provideCompletionItems(...at('PRINT "abc |"')), []);
  assert.deepEqual(ext.completionProvider.provideCompletionItems(...at('PRINT req.|')), []);
});

test('hover shows built-in, keyword, type and local docs', () => {
  const hover = (src) => ext.hoverProvider.provideHover(...at(src));
  assert.match(hover('x = SQL|ONE("q")').contents.value, /SQLONE\(query, args…\) → MAP/);
  assert.match(hover('TRANS|ACTION').contents.value, /rolls back/);
  assert.match(hover('DIM d AS DEC|IMAL').contents.value, /AS DECIMAL/);
  assert.match(hover('d = DEC|IMAL("1.5")').contents.value, /DECIMAL\(s\) → DECIMAL/);
  assert.match(hover('GO|TO 10').contents.value, /Classic mode only/);
  assert.match(hover('PRINT PAR|AM("id")').contents.value, /req\.params/);
  assert.match(hover('FUNCTION Tax(a AS NUMBER) AS DECIMAL\nEND FUNCTION\nPRINT Ta|x(1)').contents.value,
    /FUNCTION Tax\(a AS NUMBER\) AS DECIMAL[\s\S]*line 1/);
  assert.equal(hover('PRINT req.bo|dy'), null);
  assert.equal(hover('PRINT "LE|N"'), null);
  assert.equal(hover('PRINT unkno|wn'), null);
});

test('signature help highlights the current argument', () => {
  const sig = (src) => ext.signatureProvider.provideSignatureHelp(...at(src));
  const mid = sig('x = MID(s, |');
  assert.equal(mid.signatures[0].label, 'MID(s, start, [n]) → STRING');
  assert.equal(mid.activeParameter, 1);
  assert.deepEqual(mid.signatures[0].parameters[1].label, [7, 12]);

  const sql = sig('rows = SQL("q", a, b, |');
  assert.equal(sql.activeParameter, 1);

  const local = sig('FUNCTION Tax(amount AS NUMBER, rate)\nEND FUNCTION\nPRINT Tax(1, |');
  assert.equal(local.signatures[0].label, 'FUNCTION Tax(amount AS NUMBER, rate)');
  assert.equal(local.activeParameter, 1);

  assert.equal(sig('x = NOW(|'), null);
  assert.equal(sig('x = Missing(|'), null);
});
