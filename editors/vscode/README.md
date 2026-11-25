# RIP BASIC for VS Code

Editor support for [RIP BASIC](https://github.com/Emmanuelmelvin/rest-in-peace) `.bas` files:

- A tombstone file icon for `.bas` files.
- Syntax highlighting for classic (line-numbered) and structured code: keywords, types after `AS`, built-in functions, strings with `$1` SQL placeholders, `REM` and `'` comments, line numbers and `GOTO`/`GOSUB` targets.
- `'` comment toggling, bracket matching and auto-closing, and auto-indent for blocks (`FUNCTION`, `IF … THEN`, `FOR`, `TRY`, `ROUTE GROUP`, `TRANSACTION`, `TEST`, …).

The keyword and built-in lists follow `.kiro/steering/language.md`. Update the grammar in `syntaxes/ripbasic.tmLanguage.json` when that list changes.

## About the file icon

The icon is contributed through the language, so it shows in any file icon theme that has no icon of its own for RIP BASIC, including the default Seti theme. Themes that map `.bas` to their own icon (for example Material Icon Theme) show theirs instead.

If `.bas` files still open as another language, remove any `"*.bas"` entry from `files.associations` in your VS Code settings.

## Install locally

From this folder:

```sh
npx @vscode/vsce package
code --install-extension rip-basic-0.1.0.vsix
```

Then reload VS Code (**Developer: Reload Window**).
