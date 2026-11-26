# RIP BASIC for VS Code

Editor support for [RIP BASIC](https://github.com/Emmanuelmelvin/rest-in-peace) `.bas` files:

- A ghost file icon: violet for `.bas` files, green for `.test.bas` test files.
- Syntax highlighting for classic (line-numbered) and structured code: keywords, types after `AS`, built-in functions, strings with `$1` SQL placeholders, `REM` and `'` comments, line numbers and `GOTO`/`GOSUB` targets.
- `'` comment toggling, bracket matching and auto-closing, and auto-indent for blocks (`FUNCTION`, `IF … THEN`, `FOR`, `TRY`, `ROUTE GROUP`, `TRANSACTION`, `TEST`, …).
- Completion for built-in functions, keywords, types after `AS`, HTTP methods after `ROUTE`, log levels after `LOG`, middleware after `USE`, and the functions, variables, constants and imports declared in the current file. At the start of a statement, block keywords also offer a template (`IF …`, `FOR …`, `TRY …`, `ROUTE …`).
- Hover docs for built-ins, keywords, types and names declared in the current file.
- Parameter hints while typing the arguments of a built-in or a function from the current file.
- Classic-only items (`GOTO`, `GOSUB`, `PARAM`, `QUERY`, `HEADER`, `BODY`) are offered only in classic (line-numbered) files, and their hover says so.

The keyword and built-in lists follow `.kiro/steering/language.md`. When that list changes, update both the grammar in `syntaxes/ripbasic.tmLanguage.json` and the docs in `src/reference.js`. `npm test` checks `src/reference.js` against the compiler's built-in and type lists.

### Limits

This is not a language server. It reads one file at a time with pattern matching, so it does not see names imported from other files (beyond their names), member names after `.`, or errors. Go-to-definition, cross-file completion and errors as you type are planned for a language server after v1.

## About the file icons

The icons are contributed through two languages: `ripbasic` for `.bas` and `ripbasic-test` for `.test.bas`. Both use the same grammar and editing rules; the test language exists only so test files get their own icon. Tools that key on the language ID (for example a future language server) should register for both.

Because the icons come from the languages, they show in any file icon theme that has no icon of its own for them, including the default Seti theme. Themes that map `.bas` to their own icon (for example Material Icon Theme) show theirs instead.

If `.bas` files still open as another language, remove any `"*.bas"` entry from `files.associations` in your VS Code settings.

## Test

```sh
npm test
```

The tests run with Node's built-in test runner and need no install.

## Install locally

From this folder:

```sh
npx @vscode/vsce package
code --install-extension rip-basic-0.3.0.vsix
```

Then reload VS Code (**Developer: Reload Window**).
