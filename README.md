# readability.go

Go implementation of Mozilla Readability, aiming for fixture-level behavior
compatibility with [`mozilla/readability`](https://github.com/mozilla/readability).

## Status

This project is at the compatibility porting stage.

- Mozilla `test/test-pages` fixtures are copied into `testdata/test-pages`.
- The upstream fixture source is pinned in `testdata/UPSTREAM`.
- Metadata and content comparison are wired up for all pinned Mozilla fixtures.
- Full compatibility benchmarks can be run with
  `READABILITY_FULL_COMPAT=1 go test -cover -count=1 -run 'TestParseAllMozilla(Metadata|Content)Fixtures'`.

The implementation is intentionally self-contained and does not depend on
other Go Readability ports. Current work is focused on general Readability
heuristics that are checked against upstream fixtures without hard-coding those
fixtures into production logic.

## Development

- Run `make all` for the default quality gate.
- Run `make test` for the default test suite, race detector, coverage summary,
  and full Mozilla compatibility drift report.
- Run `make vet` for static checks.

## Current Upstream Drift

`tools/compare-upstream.mjs` compares this implementation with the current
Mozilla Readability checkout. Some differences are intentionally left open when
chasing current upstream would either break pinned fixtures or require
site-specific behavior. The machine-readable allowlist lives in
`tools/known-upstream-drift.json`. Pass `--known-drift` to the compare tool to
allow only these documented differences while still failing on new drift:

```sh
READABILITY_GO_JSON=/tmp/readability-json node tools/compare-upstream.mjs --all --char-threshold 1 --known-drift
```

Only add or change known drift entries after confirming the difference is not a
general parser bug and documenting why matching current upstream would be less
correct for this port or would break pinned fixtures.

- `firefox-nightly-blog` and `medicalnewstoday`: current upstream selects
  newsletter or print-message blocks, while the pinned fixtures and this port
  keep the article body.
- `hukumusume`: current upstream now returns a shorter legacy table extraction;
  the pinned fixture preserves the wider legacy table content.
- `lifehacker-post-comment-load` and `lifehacker-working`: remaining drift is
  `textContent` whitespace around block boundaries. A global text-content
  rewrite regresses many other fixtures, so this should wait for a parser-level
  whitespace model rather than a fixture-specific shortcut.
- `wikipedia`: current upstream serializes the first infobox without the
  parser-inserted `<tbody>`. Many pinned fixtures contain explicit `<tbody>`
  markup, so this needs an implicit-vs-explicit table-section strategy before
  changing serialization.
- `cnn`: current upstream keeps a small SmartAsset attribution block that this
  port currently removes during cleanup.

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
