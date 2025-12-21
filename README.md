# llformat

A focused Go source formatter that applies a custom wrapping rule to log and
printf-style statements.

## Scope (initial)

This tool rewrites argument layout for the following calls:

- `log.Infof`, `log.Debugf`, `log.Tracef`, `log.Errorf`, `log.Warnf`
- `fmt.Printf`, `fmt.Sprintf`, `fmt.Errorf`

The formatter keeps the rest of the file content intact and only rewrites the
matched call expressions. It leverages Go's AST for the matched call itself
(`go/parser.ParseExpr`) to classify arguments reliably (e.g., flattening string
literal concatenations), while using a lightweight scanner to locate the call
boundaries. This hybrid approach remains robust even when the surrounding file
is not fully valid Go.

## Wrapping Rules

- **Line width**: Wrap at 80 columns. A line may only break before 80 if the
  next token would exceed the limit.
- **Flow left**: Pack as many arguments as possible from left to right on each
  line without exceeding 80 columns.
- **Text arguments**: If an argument is purely text (a string literal or a
  concatenation of string literals), split it across lines as needed. When
  splitting, join segments with `+` and ensure a space is preserved between
  words; the preceding segment ends with a space when continued. Example:

  ```go
  log.Infof("This is a long message " +
      "that continues on the next line")
  ```

  The `+` must remain before column 80.

- **Non-text arguments**: Treat as a single unit. If adding it to the current
  line would exceed 80 columns, move it to the next line. If it still exceeds
  80 on a fresh line, keep it intact (compiles, even if it surpasses 80).

- **Nesting**: Calls may be nested inside other arguments. The formatter only
  rewrites the outer targeted calls; content inside other expressions is kept
  intact except when the entire argument is a text expression (string literal
  or concatenation of literals) which is normalized as described.

## CLI

Builds an `llformat` binary with two behaviors:

- Default: Print formatted result to stdout.
- `-w` / `--write`: Overwrite the provided file in place.

Usage:

```
llformat [-w] <path-to-go-source>
```

The CLI runs the `next` pipeline by default (and the legacy CLI modes have been
removed):

## Tests

The test `logs` reads `testdata/logs/input.go`, formats in memory, and compares
against `testdata/logs/output_next.go` (the `next` pipeline golden fixtures).

## Notes and Future Work

- The initial implementation focuses on the specified calls and a pragmatic
  text-oriented parser to handle partially invalid files. We may extend it with
  deeper expression-level formatting, additional function targets, and richer
  indentation control as new edge cases surface.
