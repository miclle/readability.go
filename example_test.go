package readability_test

import (
	"errors"
	"fmt"
	"strings"

	readability "github.com/miclle/readability.go"
)

// Demonstrates extracting an article from an HTML stream.
func ExampleFromReader() {
	html := `<html>
<head><title>The Quick Brown Fox</title></head>
<body><article>
<h1>The Quick Brown Fox</h1>
<p>The quick brown fox jumps over the lazy dog. This sentence contains every
letter of the English alphabet at least once and is long enough for the
extractor to consider this paragraph as part of the article body.</p>
<p>A second paragraph keeps the candidate substantial so the cleanup pass
will not strip the article body during conditional cleanup.</p>
</article></body>
</html>`

	article, err := readability.FromReader(strings.NewReader(html), "https://example.com/post", nil)
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Println("Title:", article.Title)
	fmt.Println("Length >", 100, ":", article.Length > 100)
	// Output:
	// Title: The Quick Brown Fox
	// Length > 100 : true
}

// Demonstrates configuring extractor options. CharThreshold lets callers
// reject documents that produce too little text, which is reported via
// the typed sentinel ErrBelowCharThreshold.
func ExampleFromReader_options() {
	html := `<html><body><article><p>Too short.</p></article></body></html>`

	_, err := readability.FromReader(
		strings.NewReader(html),
		"https://example.com/post",
		&readability.Options{CharThreshold: 500},
	)

	switch {
	case errors.Is(err, readability.ErrBelowCharThreshold):
		fmt.Println("below threshold")
	case err != nil:
		fmt.Println("other error:", err)
	default:
		fmt.Println("ok")
	}
	// Output:
	// below threshold
}

// Demonstrates the fast pre-check used to triage candidates before
// paying the full extraction cost.
func ExampleIsProbablyReaderable() {
	html := `<html><body><article>
<p>This article body has enough useful text to be picked as a readerable
candidate by the fast pre-check. The sqrt-shaped score accumulates over
qualifying paragraphs until the threshold is reached.</p>
<p>A second paragraph pushes the score over the default minimum so the
function returns true without running the full extraction pipeline.</p>
</article></body></html>`

	ok, err := readability.IsProbablyReaderable(strings.NewReader(html))
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Println("readerable:", ok)
	// Output:
	// readerable: true
}
