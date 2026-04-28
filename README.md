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
- `cnn`: current upstream keeps the outer `smartassetcontainer` (with only the
  "Powered by SmartAsset.com" attribution paragraph) while stripping the nested
  iframe/script payload. This port's embed cleanup removes the whole subtree.
  Reviewed under a 30-minute time box: replicating upstream would require a
  site-specific "attribution-bearing embed wrapper" heuristic that risks
  regressing other widget/embed fixtures (calculators, social embeds, chart
  containers), so the drift is intentionally left in place.

## Project Position

This port deliberately optimizes for **fixture-level compatibility with
the pinned mozilla/readability checkout** rather than tracking upstream
HEAD or matching any other Go port byte-for-byte. Concrete consequences:

- The 130 Mozilla `test/test-pages` fixtures are the regression suite.
  Any change must keep them green; deviations from current upstream that
  would break a pinned fixture are documented in
  `tools/known-upstream-drift.json` instead of being chased blindly.
- Compatibility behaviors that come from a single fixture (a CMS quirk,
  a news-site template) live in `compat.go` / `legacy.go` and are kept
  out of the generic parser flow so they cannot leak into other paths.
- No dependency on other Go Readability ports. Algorithms are
  re-implemented from the upstream source so behavioral differences are
  intentional and documented, not inherited.

If you need a port that tracks upstream HEAD aggressively, or one that
ships extra heuristics on top, this is not it. If you need predictable
behavior pinned to a known mozilla/readability snapshot with a
machine-checkable drift report, this is the right project.

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

### Options

`FromReader` accepts an optional `*Options` to mirror the upstream parser
configuration knobs:

```go
opts := &readability.Options{
	CharThreshold:       500,                                    // skip docs shorter than 500 chars
	ClassesToPreserve:   []string{"caption", "highlight"},      // extra classes kept during cleanup
	KeepClasses:         false,                                  // true keeps every class attribute
	NbTopCandidates:     5,                                      // candidate pool size during scoring
	DisableJSONLD:       false,                                  // skip JSON-LD metadata extraction
	AllowedVideoRegex:   nil,                                    // override built-in video allow list
	MaxElemsToParse:     0,                                      // > 0 aborts on huge documents with ErrTooManyElements
	LinkDensityModifier: 0,                                      // shifts conditional-cleanup link-density thresholds (positive = looser)
}
article, err := readability.FromReader(f, pageURL, opts)
```

When `MaxElemsToParse` is exceeded the call returns `readability.ErrTooManyElements`.
When the extracted text is shorter than `CharThreshold`, the call returns
`readability.ErrBelowCharThreshold` along with a zero-value `Article`. Use
`errors.Is` to distinguish that case from other failures.

### Probing readerability

For a fast pre-check that does not run the full extractor:

```go
ok, err := readability.IsProbablyReaderable(f)
if err != nil {
	log.Fatal(err)
}
if !ok {
	return
}
```

`IsProbablyReaderable` accepts an optional `ReaderableOptions` to tune
`MinContentLength` and `MinScore`.

## Benchmarks

A benchmark suite covering small / medium / large / visibility-heavy fixtures
lives in `bench_test.go`:

```sh
make bench                       # quick run, no baseline write
make bench-baseline              # refresh testdata/bench-baseline.txt (6 samples)
make bench-compare               # benchstat current vs committed baseline
```

`testdata/bench-baseline.txt` is captured on the maintainer's hardware
(Apple M4 Pro, darwin/arm64) and is intended for local developer reference
only. CI uses a dynamic baseline: the `bench-compare` job records both
`origin/main` and the PR head on the same runner and posts a `benchstat`
report as a build artifact. This neutralizes the CPU / scheduler variance
that would otherwise make a hardware-pinned baseline unreliable in CI.

A regression gate (`tools/bench-regression-gate.sh`) parses the benchstat
CSV output and fails the job when any benchmark regresses by ≥ 10% AND
benchstat marks the change as statistically significant (p < 0.05).
Improvements and noise (`~`) are ignored. The threshold is loose on
purpose — GitHub-hosted runners are noisy and tighter limits produce
false positives more often than they catch real regressions; tune it in
`.github/workflows/ci.yml` if your fork has access to dedicated runners.

Direct `go test` invocation is still supported for ad-hoc runs:

```sh
go test -bench=. -benchmem -benchtime=2s -run=^$
```

## Fuzzing

`fuzz_test.go` defines fuzz harnesses for both `FromReader` and
`IsProbablyReaderable`. They check that arbitrary HTML byte sequences do not
trigger panics or unexpected (non-sentinel) errors:

```sh
go test -run=^$ -fuzz=FuzzFromReader -fuzztime=30s .
go test -run=^$ -fuzz=FuzzIsProbablyReaderable -fuzztime=30s .
```

Run them in CI or locally before shipping changes that touch parsing,
cleanup, or visibility logic.

## Upstream Test Data

Compatibility fixtures under `testdata/test-pages` are copied from
Mozilla Readability and are licensed under the Apache License, Version 2.0.
See `NOTICE` and `testdata/UPSTREAM` for source and copyright details.

## License

Apache License 2.0.
