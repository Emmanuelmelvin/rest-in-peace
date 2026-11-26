# RIP BASIC for VS Code

Editor support for [RIP BASIC](https://github.com/Emmanuelmelvin/rest-in-peace) `.bas` files:

- A ghost file icon: violet for `.bas` files, green for `.test.bas` test files.
- Syntax highlighting for classic (line-numbered) and structured code: keywords, types after `AS`, built-in functions, strings with `$1` SQL placeholders, `REM` and `'` comments, line numbers and `GOTO`/`GOSUB` targets.
- `'` comment toggling, bracket matching and auto-closing, and auto-indent for blocks (`FUNCTION`, `IF … THEN`, `FOR`, `TRY`, `ROUTE GROUP`, `TRANSACTION`, `TEST`, …).

The keyword and built-in lists follow `.kiro/steering/language.md`. Update the grammar in `syntaxes/ripbasic.tmLanguage.json` when that list changes.

## About the file icons

The icons are contributed through two languages: `ripbasic` for `.bas` and `ripbasic-test` for `.test.bas`. Both use the same grammar and editing rules; the test language exists only so test files get their own icon. Tools that key on the language ID (for example a future language server) should register for both.

Because the icons come from the languages, they show in any file icon theme that has no icon of its own for them, including the default Seti theme. Themes that map `.bas` to their own icon (for example Material Icon Theme) show theirs instead.

If `.bas` files still open as another language, remove any `"*.bas"` entry from `files.associations` in your VS Code settings.

## Install locally

From this folder:

```sh
npx @vscode/vsce package
code --install-extension rip-basic-0.2.0.vsix
```

Then reload VS Code (**Developer: Reload Window**).
