package readability

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// selectionFromHTML returns the first child of <body> wrapped as a goquery
// Selection. Used by excerpt unit tests so the input markup is what
// firstExcerptText would hand to excerptBeforeBreak in production.
func selectionFromHTML(t *testing.T, fragment string) *goquery.Selection {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader("<html><body>" + fragment + "</body></html>"))
	if err != nil {
		t.Fatalf("parse fragment: %v", err)
	}
	sel := doc.Find("body").Children().First()
	if sel.Length() == 0 {
		t.Fatalf("fragment produced no top-level child: %q", fragment)
	}
	return sel
}

// TestExcerptBeforeBreakNoBR confirms the fast-path: a selection that
// contains no <br> tags returns "" without parsing — callers fall back to
// the regular block text.
func TestExcerptBeforeBreakNoBR(t *testing.T) {
	sel := selectionFromHTML(t, `<p>plain prose with no break tags whatsoever inside this paragraph at all.</p>`)
	if got := excerptBeforeBreak(sel); got != "" {
		t.Fatalf("expected empty excerpt for no-<br> selection, got %q", got)
	}
}

// TestExcerptBeforeBreakSingleBR confirms a single <br> does not trigger
// truncation: the regex requires two or more consecutive <br>s. The
// selection's full text should come back instead.
func TestExcerptBeforeBreakSingleBR(t *testing.T) {
	sel := selectionFromHTML(t, `<p>first line<br>second line</p>`)
	got := excerptBeforeBreak(sel)
	if !strings.Contains(got, "first line") || !strings.Contains(got, "second line") {
		t.Fatalf("single <br> should not split, got %q", got)
	}
}

// TestExcerptBeforeBreakDoubleBRSplit verifies the core behavior: two
// consecutive <br>s mark a paragraph boundary, and only the prefix is
// returned.
func TestExcerptBeforeBreakDoubleBRSplit(t *testing.T) {
	sel := selectionFromHTML(t, `<p>summary line for the article<br><br>follow-up paragraph that should be excluded</p>`)
	got := excerptBeforeBreak(sel)
	if got != "summary line for the article" {
		t.Fatalf("got %q, want %q", got, "summary line for the article")
	}
}

// TestExcerptBeforeBreakSelfClosingBR mirrors the XHTML-style self-closing
// <br/> form to make sure the splitter regex is forgiving of both
// variants.
func TestExcerptBeforeBreakSelfClosingBR(t *testing.T) {
	sel := selectionFromHTML(t, `<p>headline-shaped lead text<br/><br/>body that should be cut</p>`)
	got := excerptBeforeBreak(sel)
	if got != "headline-shaped lead text" {
		t.Fatalf("got %q, want %q", got, "headline-shaped lead text")
	}
}

// TestExcerptBeforeBreakWithSpacing confirms whitespace between the two
// <br>s does not defeat the split — repeatedBRE allows arbitrary
// whitespace between break tags.
func TestExcerptBeforeBreakWithSpacing(t *testing.T) {
	sel := selectionFromHTML(t, "<p>summary text<br>   \n  <br>tail</p>")
	got := excerptBeforeBreak(sel)
	if got != "summary text" {
		t.Fatalf("got %q, want %q", got, "summary text")
	}
}

// TestExcerptBeforeBreakStripsInnerTags ensures inline elements before
// the break (e.g. <strong>, <a>) are flattened to plain text rather than
// surfacing as raw HTML in the excerpt.
func TestExcerptBeforeBreakStripsInnerTags(t *testing.T) {
	sel := selectionFromHTML(t, `<p>Lead with <strong>emphasis</strong> and <a href="x">a link</a><br><br>tail</p>`)
	got := excerptBeforeBreak(sel)
	want := "Lead with emphasis and a link"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// TestExcerptBeforeBreakReturnedFromFromReader is an end-to-end smoke
// test confirming the <br><br> excerpt logic actually triggers when a
// document has neither metadata description nor structured excerpt and
// the first qualifying paragraph contains a double-break.
func TestExcerptBeforeBreakReturnedFromFromReader(t *testing.T) {
	html := `<html><body><article>
<p>introductory summary line for the article body<br><br>longer follow-up paragraph that should NOT appear in the excerpt because it is after the double break boundary inside the same block.</p>
<p>second paragraph with enough words to keep the candidate substantial during conditional cleanup so the article body is preserved long enough for excerpt extraction.</p>
<p>third paragraph adding more text to comfortably exceed the extractor length thresholds without any double-break boundary inside this paragraph.</p>
</article></body></html>`

	article, err := FromReader(strings.NewReader(html), "", nil)
	if err != nil {
		t.Fatalf("FromReader: %v", err)
	}
	if !strings.Contains(article.Excerpt, "introductory summary line") {
		t.Fatalf("Excerpt missing prefix-before-<br><br>, got: %q", article.Excerpt)
	}
	if strings.Contains(article.Excerpt, "longer follow-up paragraph") {
		t.Fatalf("Excerpt should be truncated at <br><br>, got: %q", article.Excerpt)
	}
}
