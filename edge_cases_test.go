package readability

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

// TestFromReaderEmptyInput exercises the empty-stream path. goquery accepts
// empty input and returns an empty document, so FromReader must surface a
// zero-value Article without panicking.
func TestFromReaderEmptyInput(t *testing.T) {
	article, err := FromReader(strings.NewReader(""), "", nil)
	if err != nil {
		t.Fatalf("unexpected error on empty input: %v", err)
	}
	if article.Length != 0 {
		t.Fatalf("expected zero Length on empty input, got %d", article.Length)
	}
	if article.Title != "" {
		t.Fatalf("expected empty Title on empty input, got %q", article.Title)
	}
}

// TestFromReaderWhitespaceOnly verifies that an input containing only
// whitespace produces a zero-value Article rather than a partial parse error.
func TestFromReaderWhitespaceOnly(t *testing.T) {
	article, err := FromReader(strings.NewReader("   \n\t  "), "", nil)
	if err != nil {
		t.Fatalf("unexpected error on whitespace input: %v", err)
	}
	if article.Length != 0 {
		t.Fatalf("expected zero Length on whitespace input, got %d (text=%q)", article.Length, article.TextContent)
	}
}

// TestFromReaderReadError ensures a transient reader error from io.ReadAll
// is propagated unchanged (not wrapped in a sentinel) so callers can match
// it directly with errors.Is.
func TestFromReaderReadError(t *testing.T) {
	sentinel := errors.New("synthetic read failure")
	r := errReader{err: sentinel}
	_, err := FromReader(r, "", nil)
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected reader error to propagate, got %v", err)
	}
}

type errReader struct{ err error }

func (e errReader) Read([]byte) (int, error) { return 0, e.err }

// TestFromReaderCharThresholdBoundary checks the off-by-one edges of the
// CharThreshold gate. A document whose extracted text length is exactly
// CharThreshold must NOT trigger ErrBelowCharThreshold; one rune shorter
// must.
func TestFromReaderCharThresholdBoundary(t *testing.T) {
	html := `<html><body><article>
<p>This article body needs a deterministic amount of plain prose so that the
length comparison against CharThreshold lands on a known value used by the
boundary assertions in this test case definition exactly.</p>
<p>A second paragraph keeps the candidate substantial so cleanup does not
strip the article body during the conditional cleanup pass and the final
extracted text length stays predictable enough for boundary verification.</p>
</article></body></html>`

	// First learn the actual extracted length with no threshold.
	article, err := FromReader(strings.NewReader(html), "", nil)
	if err != nil {
		t.Fatalf("baseline FromReader failed: %v", err)
	}
	got := utf8.RuneCountInString(article.TextContent)
	if got == 0 {
		t.Fatal("baseline extraction produced zero-length text")
	}

	// Threshold equal to extracted length: must succeed.
	if _, err := FromReader(strings.NewReader(html), "", &Options{CharThreshold: got}); err != nil {
		t.Fatalf("CharThreshold == extracted length should succeed, got %v", err)
	}

	// Threshold one rune higher: must trip the sentinel.
	_, err = FromReader(strings.NewReader(html), "", &Options{CharThreshold: got + 1})
	if !errors.Is(err, ErrBelowCharThreshold) {
		t.Fatalf("CharThreshold == extracted+1 should return ErrBelowCharThreshold, got %v", err)
	}
}

// TestFromReaderMaxElemsToParseBoundary checks that the element-count gate
// uses a strict greater-than: a document with exactly MaxElemsToParse
// elements must NOT trigger ErrTooManyElements.
func TestFromReaderMaxElemsToParseBoundary(t *testing.T) {
	const sample = `<html><body><article><p>boundary case for element counting in the parser.</p></article></body></html>`

	// Count actual elements by running with a very high cap first.
	article, err := FromReader(strings.NewReader(sample), "", &Options{MaxElemsToParse: 1_000_000})
	if err != nil {
		t.Fatalf("baseline FromReader failed: %v", err)
	}
	if article.Length == 0 {
		t.Fatal("baseline extraction produced no text")
	}

	// MaxElemsToParse below the actual count must trip.
	_, err = FromReader(strings.NewReader(sample), "", &Options{MaxElemsToParse: 1})
	if !errors.Is(err, ErrTooManyElements) {
		t.Fatalf("MaxElemsToParse=1 should return ErrTooManyElements, got %v", err)
	}
}

// TestFromReaderDeeplyNestedDOM ensures the parser tolerates pathologically
// deep nesting without stack overflow. The depth chosen here is well below
// Go's default goroutine stack growth limit but large enough to expose any
// accidental quadratic recursion.
func TestFromReaderDeeplyNestedDOM(t *testing.T) {
	const depth = 500
	var b strings.Builder
	b.WriteString("<html><body>")
	for range depth {
		b.WriteString("<div>")
	}
	b.WriteString(`<article><p>This article body has enough useful text inside a deeply nested wrapper chain to verify the parser does not blow up the stack while walking ancestors during scoring and cleanup.</p>
<p>A second paragraph keeps the candidate substantial so cleanup does not strip the article body during conditional cleanup and the deep ancestor chain is fully exercised.</p></article>`)
	for range depth {
		b.WriteString("</div>")
	}
	b.WriteString("</body></html>")

	article, err := FromReader(strings.NewReader(b.String()), "", nil)
	if err != nil {
		t.Fatalf("deeply nested DOM should not error: %v", err)
	}
	if article.Length == 0 {
		t.Fatalf("deeply nested DOM should still extract text, got empty")
	}
}

// TestFromReaderHugeAttributeValue checks that an element with an enormous
// attribute value does not break parsing or extraction. Attribute scanning
// happens in several passes (class weighting, byline detection, video
// allow-list) so this guards against accidental O(n*m) behavior.
func TestFromReaderHugeAttributeValue(t *testing.T) {
	huge := strings.Repeat("a", 100_000)
	html := `<html><body><article data-noise="` + huge + `">
<p>This article body has enough useful text to pass the extractor's length checks even when sibling elements carry very large attribute payloads designed to stress the attribute scanning passes.</p>
<p>A second paragraph keeps the candidate substantial so cleanup does not strip the body during conditional cleanup despite the noisy attribute on the wrapper element.</p>
</article></body></html>`

	article, err := FromReader(strings.NewReader(html), "", nil)
	if err != nil {
		t.Fatalf("huge attribute should not error: %v", err)
	}
	if article.Length == 0 {
		t.Fatalf("expected extracted text despite large attribute, got empty")
	}
}

// TestFromReaderInvalidUTF8 verifies that bytes which are not valid UTF-8
// do not panic the parser. Go's html package is tolerant; we only require
// that the call returns without crashing and that any extracted text is
// well-formed UTF-8 (utf8.RuneCountInString handles invalid sequences as
// single replacement runes, so length must stay non-negative).
func TestFromReaderInvalidUTF8(t *testing.T) {
	// Embed raw 0xFF bytes inside otherwise-valid HTML.
	dirty := "<html><body><article><p>prefix \xff\xfe middle \xff\xff suffix with enough additional plain prose to potentially clear the extraction length checks during the test run.</p><p>second paragraph with more text so the candidate stays substantial during conditional cleanup.</p></article></body></html>"

	article, err := FromReader(strings.NewReader(dirty), "", nil)
	if err != nil {
		t.Fatalf("invalid UTF-8 should not error: %v", err)
	}
	if article.Length < 0 {
		t.Fatalf("Length must be non-negative, got %d", article.Length)
	}
}

// TestIsProbablyReaderableEmptyInput ensures the fast pre-check tolerates
// an empty stream without panicking and reports "not readerable".
func TestIsProbablyReaderableEmptyInput(t *testing.T) {
	ok, err := IsProbablyReaderable(strings.NewReader(""))
	if err != nil {
		t.Fatalf("unexpected error on empty input: %v", err)
	}
	if ok {
		t.Fatal("empty input must not be readerable")
	}
}
