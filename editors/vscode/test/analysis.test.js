'use strict';

const test = require('node:test');
const assert = require('node:assert/strict');
const an = require('../src/analysis');

test('isClassic looks at the first non-comment line', () => {
  assert.equal(an.isClassic('REM hello\n\n10 PRINT "HI"\n20 END'), true);
  assert.equal(an.isClassic("' top\nPRINT \"HI\""), false);
  assert.equal(an.isClassic(''), false);
});

test('completionContext skips strings and comments', () => {
  assert.equal(an.completionContext('PRINT "hel'), 'none');
  assert.equal(an.completionContext("x = 1 ' LE"), 'none');
  assert.equal(an.completionContext('10 REM LE'), 'none');
  assert.equal(an.completionContext('PRINT "a" + LE'), 'expression');
});

test('completionContext finds statements, types, methods, levels and middleware', () => {
  assert.equal(an.completionContext(''), 'statement');
  assert.equal(an.completionContext('  FO'), 'statement');
  assert.equal(an.completionContext('10 PR'), 'statement');
  assert.equal(an.completionContext('10 X = 1 : PR'), 'statement');
  assert.equal(an.completionContext('IF x THEN PR'), 'statement');
  assert.equal(an.completionContext('DIM total AS DEC'), 'type');
  assert.equal(an.completionContext('FUNCTION F(a AS '), 'type');
  assert.equal(an.completionContext('ROUTE G'), 'method');
  assert.equal(an.completionContext('LOG '), 'logLevel');
  assert.equal(an.completionContext('USE RATE'), 'middleware');
  assert.equal(an.completionContext('PRINT u.na'), 'member');
  assert.equal(an.completionContext('20 GOTO 1'), 'none');
  assert.equal(an.completionContext('PRINT UPPER(na'), 'expression');
});

test('scanSymbols finds declarations', () => {
  const src = [
    'IMPORT { CalcTax, CalcTotal } FROM "./billing.bas"',
    'IMPORT uuid FROM GO "github.com/google/uuid"',
    'CONST RATE = 0.2',
    'DIM total AS decimal = DECIMAL("0")',
    'EXPORT FUNCTION Price(amount AS NUMBER, label = "x, y") AS DECIMAL',
    '  FOR EACH item IN items',
    '  NEXT',
    'END FUNCTION',
    'SUB Hello()',
    'END SUB',
    'TRY',
    'CATCH e',
    'END TRY',
    'count = 0',
    "' FUNCTION Hidden()",
  ].join('\n');
  const syms = an.scanSymbols(src);
  const byName = Object.fromEntries(syms.map((s) => [s.name, s]));

  assert.equal(byName.CalcTax.kind, 'import');
  assert.equal(byName.CalcTotal.detail, 'IMPORT { CalcTotal } FROM "./billing.bas"');
  assert.equal(byName.uuid.detail, 'IMPORT uuid FROM GO "github.com/google/uuid"');
  assert.equal(byName.RATE.kind, 'const');
  assert.equal(byName.total.detail, 'DIM total AS DECIMAL');
  assert.deepEqual(byName.Price.params, ['amount AS NUMBER', 'label = "x, y"']);
  assert.equal(byName.Price.detail, 'EXPORT FUNCTION Price(amount AS NUMBER, label = "x, y") AS DECIMAL');
  assert.equal(byName.Price.line, 4);
  assert.equal(byName.amount.detail, 'parameter amount AS NUMBER');
  assert.equal(byName.item.kind, 'variable');
  assert.equal(byName.Hello.kind, 'sub');
  assert.deepEqual(byName.Hello.params, []);
  assert.equal(byName.e.kind, 'variable');
  assert.equal(byName.count.kind, 'variable');
  assert.equal(byName.Hidden, undefined);
});

test('scanSymbols reads classic lines with several statements', () => {
  const syms = an.scanSymbols('10 X = 1 : Y = "a:b" : PRINT X\n20 FOR I = 1 TO 3');
  assert.deepEqual(syms.map((s) => s.name), ['X', 'Y', 'I']);
});

test('callAt finds the call and argument under the cursor', () => {
  assert.deepEqual(an.callAt('x = MID('), { name: 'MID', argIndex: 0 });
  assert.deepEqual(an.callAt('x = MID(s, 2'), { name: 'MID', argIndex: 1 });
  assert.deepEqual(an.callAt('x = MID(LEFT(s, 3), '), { name: 'MID', argIndex: 1 });
  assert.deepEqual(an.callAt('x = SQL("a, b, (c", '), { name: 'SQL', argIndex: 1 });
  assert.deepEqual(an.callAt('x = SQL("SELECT", [1, 2], '), { name: 'SQL', argIndex: 2 });
  assert.equal(an.callAt('x = LEFT(s, 1) + '), null);
  assert.equal(an.callAt('x = uuid.NewString('), null);
  assert.equal(an.callAt("' LEFT("), null);
});

test('wordAt reports the word and whether it is a member', () => {
  assert.deepEqual(an.wordAt('PRINT LEN(x)', 7), { word: 'LEN', start: 6, end: 9, member: false });
  assert.equal(an.wordAt('PRINT req.body', 11).member, true);
  assert.equal(an.wordAt('   ', 1), null);
});
