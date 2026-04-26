# readability.go

Go implementation of Mozilla Readability, aiming for fixture-level behavior
compatibility with [`mozilla/readability`](https://github.com/mozilla/readability).

## Status

This project is at the compatibility porting stage.

- Mozilla `test/test-pages` fixtures are copied into `testdata/test-pages`.
- The upstream fixture source is pinned in `testdata/UPSTREAM`.
- Metadata and content comparison are wired up for all pinned Mozilla fixtures.
- Full compatibility benchmarks can be run with
  `READABILITY_FULL_COMPAT=1 go test -run 'TestParseAllMozilla(Metadata|Content)Fixtures'`.

The implementation is intentionally self-contained and does not depend on
other Go Readability ports. Current work is focused on general Readability
heuristics that are checked against upstream fixtures without hard-coding those
fixtures into production logic.

## Development

- Run `make test` for the default test suite.
- Run `make compat-test` before changing parser, scoring, cleaning, or metadata
  behavior to inspect benchmark drift.
- `make strict-compat-test` is kept as a non-failing compatibility notice; exact
  fixture matching is not a maintenance gate for this general-purpose library.
- Run `make vet` for static checks.

## Implementation Layout

The public entry point lives in `article.go`. The parser implementation is
split by responsibility:

- `extract.go` coordinates article extraction and fallback selection.
- `score.go` scores article candidates and builds the final content tree.
- `clean.go`, `condition.go`, `normalize.go`, and `media.go` clean and
  normalize extracted content.
- `compat.go` and `legacy.go` hold fixture-proven compatibility behavior that
  is intentionally kept separate from the generic parser flow.
- `metadata.go`, `excerpt.go`, and `byline.go` extract document metadata.
- `dom.go` and `url.go` provide DOM and URL helpers used across the parser.

## Usage

```go
package main

import (
	"fmt"
	"log"
	"os"

	readability "github.com/miclle/readability.go"
)

func main() {
	f, err := os.Open("article.html")
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	article, err := readability.FromReader(f, "https://example.com/article", nil)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(article.Title)
	fmt.Println(article.TextContent)
}
```

## Upstream Test Data

Compatibility fixtures under `testdata/test-pages` are copied from
Mozilla Readability and are licensed under the Apache License, Version 2.0.
See `NOTICE` and `testdata/UPSTREAM` for source and copyright details.

## License

Apache License 2.0.
