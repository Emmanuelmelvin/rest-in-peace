'use strict';

// Completion, hover and signature help for RIP BASIC, without a language
// server. Built-ins and keywords come from reference.js; names declared in
// the current file come from analysis.js. A full language server (go-to-
// definition across files, errors as you type) is a post-v1 roadmap item.

const vscode = require('vscode');
const ref = require('./reference');
const an = require('./analysis');

const SELECTOR = [
  { language: 'ripbasic' },
  { language: 'ripbasic-test' },
];

const builtinsByName = new Map(ref.builtins.map((b) => [b.name, b]));
const keywordsByName = new Map(ref.keywords.map((k) => [k.name, k]));
const typesByName = new Map(ref.types.map((t) => [t.name, t]));

function signature(b) {
  return `${b.name}(${b.params.join(', ')})${b.middleware ? '' : ` → ${b.returns}`}`;
}

function codeBlock(text) {
  return new vscode.MarkdownString().appendCodeblock(text, 'ripbasic');
}

function builtinDoc(b) {
  const md = codeBlock(signature(b));
  md.appendMarkdown(b.doc);
  return md;
}

function keywordDoc(k) {
  const md = codeBlock(k.syntax || k.name);
  md.appendMarkdown(k.doc);
  return md;
}

function allowed(item, classic) {
  return item.mode !== 'classic' || classic;
}

// Inserts NAME($1) with tab stops for each parameter, or NAME() for none.
function callSnippet(name, params) {
  const required = params.filter((p) => !p.startsWith('['));
  if (params.length === 0) return new vscode.SnippetString(`${name}()`);
  if (required.length === 0) return new vscode.SnippetString(`${name}($0)`);
  const stops = required.map((p, i) => `\${${i + 1}:${p.replace(/[$}\\]/g, '\\$&')}}`);
  return new vscode.SnippetString(`${name}(${stops.join(', ')})`);
}

function builtinItem(b) {
  const item = new vscode.CompletionItem(b.name, vscode.CompletionItemKind.Function);
  item.detail = signature(b);
  item.documentation = builtinDoc(b);
  item.insertText = callSnippet(b.name, b.params);
  item.command = b.params.length ? { command: 'editor.action.triggerParameterHints', title: '' } : undefined;
  return item;
}

function keywordItem(k, statement) {
  const item = new vscode.CompletionItem(k.name, vscode.CompletionItemKind.Keyword);
  item.detail = k.syntax;
  item.documentation = keywordDoc(k);
  if (statement && k.snippet) {
    // Offer the plain keyword and the block template as two entries.
    const snip = new vscode.CompletionItem(`${k.name} …`, vscode.CompletionItemKind.Snippet);
    snip.filterText = k.name;
    snip.detail = k.syntax;
    snip.documentation = keywordDoc(k);
    snip.insertText = new vscode.SnippetString(k.snippet);
    snip.sortText = `0${k.name}`;
    return [item, snip];
  }
  return [item];
}

function symbolKind(sym) {
  switch (sym.kind) {
    case 'function':
    case 'sub':
      return vscode.CompletionItemKind.Function;
    case 'const':
      return vscode.CompletionItemKind.Constant;
    case 'import':
      return vscode.CompletionItemKind.Module;
    default:
      return vscode.CompletionItemKind.Variable;
  }
}

function symbolItem(sym, refSet) {
  // Builtin and keyword names already have their own entries.
  if (refSet.has(sym.name.toUpperCase())) return null;
  const item = new vscode.CompletionItem(sym.name, symbolKind(sym));
  item.detail = sym.detail;
  if (sym.kind === 'function' || sym.kind === 'sub') {
    const names = sym.params.map((p) => p.split(/\s|=/)[0]);
    item.insertText = callSnippet(sym.name, names.map((n, i) => (/=/.test(sym.params[i]) ? `[${n}]` : n)));
  }
  item.sortText = `1${sym.name}`;
  return item;
}

const refNames = new Set([...builtinsByName.keys(), ...keywordsByName.keys(), ...typesByName.keys()]);

const completionProvider = {
  provideCompletionItems(document, position) {
    const prefix = document.lineAt(position.line).text.slice(0, position.character);
    const ctx = an.completionContext(prefix);
    if (ctx === 'none' || ctx === 'member') return [];

    if (ctx === 'type') {
      return ref.types.map((t) => {
        const item = new vscode.CompletionItem(t.name, vscode.CompletionItemKind.TypeParameter);
        item.documentation = new vscode.MarkdownString(t.doc);
        return item;
      });
    }
    if (ctx === 'method') {
      const items = ref.httpMethods.map((m) => new vscode.CompletionItem(m, vscode.CompletionItemKind.EnumMember));
      return items.concat(keywordItem(keywordsByName.get('GROUP'), false));
    }
    if (ctx === 'logLevel') {
      return ref.logLevels.map((l) => new vscode.CompletionItem(l, vscode.CompletionItemKind.EnumMember));
    }

    const text = document.getText();
    const classic = an.isClassic(text);
    const symbols = an.scanSymbols(text);

    if (ctx === 'middleware') {
      const mw = ref.builtins.filter((b) => b.middleware).map(builtinItem);
      const fns = symbols.filter((s) => s.kind === 'function' || s.kind === 'sub' || s.kind === 'import');
      return mw.concat(fns.map((s) => {
        const item = new vscode.CompletionItem(s.name, symbolKind(s));
        item.detail = s.detail;
        return item;
      }));
    }

    const statement = ctx === 'statement';
    const items = [];
    for (const b of ref.builtins) {
      if (allowed(b, classic) && !b.middleware) items.push(builtinItem(b));
    }
    for (const k of ref.keywords) {
      if (allowed(k, classic)) items.push(...keywordItem(k, statement));
    }
    for (const s of symbols) {
      const item = symbolItem(s, refNames);
      if (item) items.push(item);
    }
    return items;
  },
};

const hoverProvider = {
  provideHover(document, position) {
    const line = document.lineAt(position.line).text;
    const w = an.wordAt(line, position.character);
    if (!w || w.member) return null;
    const { inString, inComment } = an.scanPrefix(line.slice(0, w.start));
    if (inString || inComment) return null;

    const range = new vscode.Range(position.line, w.start, position.line, w.end);
    const name = w.word.toUpperCase();
    const classic = an.isClassic(document.getText());

    // A name followed by `(` is a call, so prefer the built-in function over
    // the type of the same name (DECIMAL).
    const isCall = /^\s*\(/.test(line.slice(w.end));
    const afterAs = /\bAS\s+$/i.test(line.slice(0, w.start));

    const b = builtinsByName.get(name);
    if (b && !afterAs && (isCall || !typesByName.has(name))) {
      const md = builtinDoc(b);
      if (!allowed(b, classic)) md.appendMarkdown('\n\n**Classic mode only.** Structured handlers read `req.params`, `req.query`, `req.headers` and `req.body` instead.');
      return new vscode.Hover(md, range);
    }
    const t = typesByName.get(name);
    if (t && (afterAs || !keywordsByName.has(name))) {
      return new vscode.Hover(codeBlock(`AS ${t.name}`).appendMarkdown(t.doc), range);
    }
    const k = keywordsByName.get(name);
    if (k) {
      const md = keywordDoc(k);
      if (!allowed(k, classic)) md.appendMarkdown('\n\n**Classic mode only.** This is a compile error in structured mode.');
      return new vscode.Hover(md, range);
    }

    const sym = an.scanSymbols(document.getText()).find((s) => s.name.toUpperCase() === name);
    if (sym) {
      const md = codeBlock(sym.detail);
      md.appendMarkdown(`Declared on line ${sym.line + 1}.`);
      return new vscode.Hover(md, range);
    }
    return null;
  },
};

const signatureProvider = {
  provideSignatureHelp(document, position) {
    const prefix = document.lineAt(position.line).text.slice(0, position.character);
    const call = an.callAt(prefix);
    if (!call) return null;
    const name = call.name.toUpperCase();

    let label;
    let params;
    let doc;
    const b = builtinsByName.get(name);
    if (b) {
      label = signature(b);
      params = b.params;
      doc = b.doc;
    } else {
      const sym = an.scanSymbols(document.getText())
        .find((s) => (s.kind === 'function' || s.kind === 'sub') && s.name.toUpperCase() === name);
      if (!sym) return null;
      label = sym.detail.replace(/^EXPORT\s+/, '');
      params = sym.params;
    }
    if (params.length === 0) return null;

    const info = new vscode.SignatureInformation(label, doc ? new vscode.MarkdownString(doc) : undefined);
    // Parameter labels are [start, end] offsets into the signature label.
    let from = label.indexOf('(') + 1;
    info.parameters = params.map((p) => {
      const start = label.indexOf(p, from);
      from = start + p.length;
      return new vscode.ParameterInformation([start, start + p.length]);
    });
    const help = new vscode.SignatureHelp();
    help.signatures = [info];
    help.activeSignature = 0;
    // Variadic parameters (args…) stay highlighted for every extra argument.
    const last = params.length - 1;
    help.activeParameter = call.argIndex > last && params[last].endsWith('…') ? last : Math.min(call.argIndex, last);
    return help;
  },
};

function activate(context) {
  context.subscriptions.push(
    vscode.languages.registerCompletionItemProvider(SELECTOR, completionProvider),
    vscode.languages.registerHoverProvider(SELECTOR, hoverProvider),
    vscode.languages.registerSignatureHelpProvider(SELECTOR, signatureProvider, '(', ','),
  );
}

function deactivate() {}

module.exports = { activate, deactivate, completionProvider, hoverProvider, signatureProvider };
