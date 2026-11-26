'use strict';

// Keeps reference.js in step with the compiler's built-in and type lists.

const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const ref = require('../src/reference');

const checkGo = fs.readFileSync(path.join(__dirname, '../../../internal/compiler/check/check.go'), 'utf8');

function goMapKeys(name) {
  const m = new RegExp(`var ${name} = map\\[string\\]\\w+\\{([\\s\\S]*?)\\n\\}`).exec(checkGo);
  assert.ok(m, `map ${name} in check.go`);
  return [...m[1].matchAll(/"([A-Z]+)":/g)].map((x) => x[1]).sort();
}

test('built-ins match internal/compiler/check', () => {
  const compiler = [...goMapKeys('builtins'), ...goMapKeys('classicBuiltins')].sort();
  assert.deepEqual(ref.builtins.map((b) => b.name).sort(), compiler);
});

test('classic-only built-ins match classicBuiltins', () => {
  assert.deepEqual(ref.builtins.filter((b) => b.mode === 'classic').map((b) => b.name).sort(), goMapKeys('classicBuiltins'));
});

test('types match validTypeList', () => {
  const m = /var validTypeList = \[\]string\{([\s\S]*?)\}/.exec(checkGo);
  assert.deepEqual(ref.types.map((t) => t.name), [...m[1].matchAll(/"([A-Z]+)"/g)].map((x) => x[1]));
});

test('names are unique and upper case', () => {
  for (const list of [ref.builtins, ref.keywords, ref.types]) {
    const names = list.map((x) => x.name);
    assert.equal(new Set(names).size, names.length);
    for (const n of names) assert.equal(n, n.toUpperCase());
  }
});
