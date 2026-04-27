package readability

import (
	"strings"
	"testing"
)

// TestFixLazyImagesPromotesDataAttribute verifies the lazy-loading recovery
// path: an <img> with a placeholder src and a data-* attribute holding the
// real image URL must end up with src/srcset populated from that data
// attribute, mirroring mozilla/readability's _fixLazyImages behavior.
func TestFixLazyImagesPromotesDataAttribute(t *testing.T) {
	html := `<html><body><article>
<p>This article body has enough useful text to clear the extractor length checks while we exercise the lazy-image recovery path embedded inside the article body for testing fixLazyImages explicitly.</p>
<p>A second paragraph keeps the candidate substantial during conditional cleanup so the lazy image survives long enough for assertions to inspect the final HTML output.</p>
<figure><img class="lazy" src="placeholder.gif" data-original="https://cdn.example.com/real-image.jpg"></figure>
</article></body></html>`

	article, err := FromReader(strings.NewReader(html), "", nil)
	if err != nil {
		t.Fatalf("FromReader: %v", err)
	}
	if !strings.Contains(article.Content, "real-image.jpg") {
		t.Fatalf("expected lazy image to be promoted to real URL, got: %s", article.Content)
	}
}

// TestFixLazyImagesPromotesSrcset checks the srcset promotion branch:
// when a data-* attribute contains a srcset-shaped value (URL + descriptor),
// it must be copied to the srcset attribute, not src.
func TestFixLazyImagesPromotesSrcset(t *testing.T) {
	html := `<html><body><article>
<p>This article body has enough useful text to clear the extractor length checks while we exercise the srcset recovery path embedded inside the article body for testing fixLazyImages.</p>
<p>A second paragraph keeps the candidate substantial during conditional cleanup so the lazy image survives long enough for assertions to inspect the final HTML output.</p>
<img class="lazy" data-srcset="https://cdn.example.com/small.jpg 1x, https://cdn.example.com/large.jpg 2x">
</article></body></html>`

	article, err := FromReader(strings.NewReader(html), "", nil)
	if err != nil {
		t.Fatalf("FromReader: %v", err)
	}
	if !strings.Contains(article.Content, "srcset=") || !strings.Contains(article.Content, "large.jpg 2x") {
		t.Fatalf("expected srcset to be populated from data-srcset, got: %s", article.Content)
	}
}

// TestFixLazyImagesPreservesRealSrc ensures images that already have a
// usable src and no "lazy" class hint are left alone — the recovery path
// must not stomp on legitimate sources.
func TestFixLazyImagesPreservesRealSrc(t *testing.T) {
	html := `<html><body><article>
<p>This article body has enough useful text to clear the extractor length checks while we make sure non-lazy images with an existing src attribute are not touched by the recovery pass.</p>
<p>A second paragraph keeps the candidate substantial during conditional cleanup so the image survives long enough for assertions to inspect the final HTML output verifying the src preservation.</p>
<img src="https://cdn.example.com/already-good.jpg" data-original="https://other.example.com/promoted-on-error.jpg">
</article></body></html>`

	article, err := FromReader(strings.NewReader(html), "", nil)
	if err != nil {
		t.Fatalf("FromReader: %v", err)
	}
	if !strings.Contains(article.Content, `src="https://cdn.example.com/already-good.jpg"`) {
		t.Fatalf("expected real src to survive, got: %s", article.Content)
	}
	// The recovery path must not have copied data-original into src.
	if strings.Contains(article.Content, `src="https://other.example.com/promoted-on-error.jpg"`) {
		t.Fatalf("data-original must not overwrite an already-set src on a non-lazy image, got: %s", article.Content)
	}
}

// TestReplaceJavascriptLinksTextOnly converts a javascript: anchor whose
// only child is a text node into a bare text node, dropping the inert link.
func TestReplaceJavascriptLinksTextOnly(t *testing.T) {
	html := `<html><body><article>
<p>This article body has enough useful text to clear the extractor length checks while we verify inert links containing only text get unwrapped to plain text by the cleanup pass.</p>
<p>A second paragraph keeps the candidate substantial during conditional cleanup. <a href="javascript:doStuff()">click me</a> Some trailing text continues the paragraph.</p>
</article></body></html>`

	article, err := FromReader(strings.NewReader(html), "", nil)
	if err != nil {
		t.Fatalf("FromReader: %v", err)
	}
	if strings.Contains(article.Content, `href="javascript:`) {
		t.Fatalf("javascript: anchor should be removed from Content, got: %s", article.Content)
	}
	if !strings.Contains(article.TextContent, "click me") {
		t.Fatalf("inner text should be preserved, got TextContent: %q", article.TextContent)
	}
}

// TestReplaceJavascriptLinksWithChildElement converts a javascript: anchor
// whose children include element nodes into a <span> wrapper instead of a
// text node — preserving the structural children while dropping the inert
// link target.
func TestReplaceJavascriptLinksWithChildElement(t *testing.T) {
	html := `<html><body><article>
<p>This article body has enough useful text to clear the extractor length checks while we verify inert links wrapping an inline element get rewritten to a span.</p>
<p>A second paragraph keeps the candidate substantial during conditional cleanup. <a href="javascript:doStuff()"><strong>bold call to action</strong></a> Some trailing text follows the link.</p>
</article></body></html>`

	article, err := FromReader(strings.NewReader(html), "", nil)
	if err != nil {
		t.Fatalf("FromReader: %v", err)
	}
	if strings.Contains(article.Content, `href="javascript:`) {
		t.Fatalf("javascript: anchor should be removed from Content, got: %s", article.Content)
	}
	if !strings.Contains(article.Content, "<strong>bold call to action</strong>") {
		t.Fatalf("inline element child should be preserved, got: %s", article.Content)
	}
	if !strings.Contains(article.Content, "<span>") {
		t.Fatalf("expected anchor with element children to be rewritten to a <span>, got: %s", article.Content)
	}
}

// TestReplaceJavascriptLinksLeavesNormalAnchors confirms that http(s) anchors
// are untouched by the javascript: rewriter — only the inert protocol is
// targeted.
func TestReplaceJavascriptLinksLeavesNormalAnchors(t *testing.T) {
	html := `<html><body><article>
<p>This article body has enough useful text to clear the extractor length checks while we verify normal http anchors are not affected by the javascript: cleanup pass at all.</p>
<p>A second paragraph keeps the candidate substantial during conditional cleanup. See <a href="https://example.com/related">the related article</a> for more context and trailing text continues here.</p>
</article></body></html>`

	article, err := FromReader(strings.NewReader(html), "", nil)
	if err != nil {
		t.Fatalf("FromReader: %v", err)
	}
	if !strings.Contains(article.Content, `href="https://example.com/related"`) {
		t.Fatalf("normal anchor href should be preserved, got: %s", article.Content)
	}
}
