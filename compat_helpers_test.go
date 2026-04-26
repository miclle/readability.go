package readability

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	xhtml "golang.org/x/net/html"
)

func TestCleanGenericBylineTrimsMetadataLines(t *testing.T) {
	got := cleanGenericByline("By JANE DOE\nEditor notes\n2h")
	if got != "By Jane Doe" {
		t.Fatalf("cleanGenericByline = %q", got)
	}

	got = cleanGenericByline("Jane Doe • Updated today")
	if got != "Jane Doe" {
		t.Fatalf("cleanGenericByline bullet trim = %q", got)
	}
}

func TestBylineSemanticHelpers(t *testing.T) {
	doc := mustTestDocument(t, `
		<div class="author-bio"><p>About the author <span id="bio">Jane</span></p></div>
		<a id="semantic" rel="author">Jane</a>
		<div class="authors"><span id="inline">Jane Doe</span></div>
		<div class="authors"><time>2024</time><span id="timed">Jane Doe</span></div>`)

	if !isAuthorBioSection(doc.Find("#bio")) {
		t.Fatal("expected author bio section")
	}
	if !isAuthorSemanticNode(doc.Find("#semantic")) {
		t.Fatal("expected semantic author node")
	}
	if !isInlineAuthorsAttribution(doc.Find("#inline")) {
		t.Fatal("expected inline authors attribution")
	}
	if isInlineAuthorsAttribution(doc.Find("#timed")) {
		t.Fatal("timed attribution should not be treated as inline authors")
	}
}

func TestExpandedBylineActivityUsesSemanticActivityValue(t *testing.T) {
	doc := mustTestDocument(t, `<article data-activity-map="article-byline-footer"><span id="name">Jane</span></article>`)

	if !isExpandedBylineActivity(doc.Find("#name")) {
		t.Fatal("expected byline activity to be detected from ancestor activity map")
	}
}

func TestCollectionHighlightsDetectionUsesClassAndIDMeaning(t *testing.T) {
	doc := mustTestDocument(t, `<section id="feature-collection" class="story-highlights"><p id="item">Story</p></section>`)

	if !isInsideCollectionHighlights(doc.Find("#item")) {
		t.Fatal("expected collection highlight ancestor to be detected")
	}
	if !containsCollectionHighlights(doc.Selection) {
		t.Fatal("expected collection highlights descendant to be detected")
	}
}

func TestRestoreContinuationLinksAddsMissingAnchorForMatchingTargets(t *testing.T) {
	doc := mustTestDocument(t, `
		<article>
			<p><a href="#chapter-break-1">Continue reading below</a></p>
			<section><div id="chapter-break-2">Rest of story</div></section>
		</article>`)
	root := doc.Find("article").First()

	restoreContinuationLinks(root)

	if root.Find("#chapter-break-1 a[href='#chapter-break-2']").Length() != 1 {
		html, _ := root.Html()
		t.Fatalf("missing restored story continue link in %s", html)
	}
}

func TestNormalizeVideoPlayerContainersUnwrapsGenericPlayer(t *testing.T) {
	doc := mustTestDocument(t, `
		<article>
			<div id="custom-player"><div><iframe src="video"></iframe></div><p>Remove me</p></div>
			<p>&lt; &gt;</p>
			<p>Keep me</p>
		</article>`)
	root := doc.Find("article").First()

	normalizeVideoPlayerContainers(root)

	if root.Find("#custom-player > iframe").Length() != 1 {
		html, _ := root.Html()
		t.Fatalf("player iframe was not unwrapped in %s", html)
	}
	if strings.Contains(root.Text(), "Remove me") || strings.Contains(root.Text(), "< >") {
		t.Fatalf("video player cleanup left removable text: %q", normalizeSpace(root.Text()))
	}
	if !strings.Contains(root.Text(), "Keep me") {
		t.Fatal("video player cleanup removed following content")
	}
}

func TestCleanArticleCandidateKeepsAllowedObjectVideo(t *testing.T) {
	doc := mustTestDocument(t, `<article>
		<object data="//www.youtube.com/embed/example"></object>
		<embed src="//example.com/widget"></embed>
	</article>`)
	article := doc.Find("article").First()

	cleanArticleCandidate(article)

	if article.Find("object").Length() != 1 {
		html, _ := article.Html()
		t.Fatalf("allowed object video was removed from %s", html)
	}
	if article.Find("embed").Length() != 0 {
		html, _ := article.Html()
		t.Fatalf("disallowed embed was kept in %s", html)
	}
}

func TestNormalizeLegacyFileURLs(t *testing.T) {
	doc := mustTestDocument(t, `<article><a href="file:///C%7C/path/doc.html">Doc</a><img src="file:///C|/path/img.png"></article>`)
	article := doc.Find("article").First()

	normalizeLegacyFileURLs(article)

	if got := attr(article.Find("a").First(), "href"); got != "file:///C:/path/doc.html" {
		t.Fatalf("legacy file URL = %q", got)
	}
	if got := attr(article.Find("img").First(), "src"); got != "file:///C:/path/img.png" {
		t.Fatalf("legacy image URL = %q", got)
	}
}

func TestCollapseLegacySidebarLinkSpacing(t *testing.T) {
	td := &xhtml.Node{Type: xhtml.ElementNode, Data: "td"}
	td.AppendChild(&xhtml.Node{Type: xhtml.TextNode, Data: "Text before "})
	a := &xhtml.Node{Type: xhtml.ElementNode, Data: "a", Attr: []xhtml.Attribute{{Key: "href", Val: "next.html"}}}
	a.AppendChild(&xhtml.Node{Type: xhtml.TextNode, Data: "Next"})
	td.AppendChild(a)

	collapseLegacySidebarLinkSpacing(td)

	if got := td.FirstChild.Data; got != "Text before" {
		t.Fatalf("sidebar text = %q", got)
	}
}

func TestWrapPhrasingContentInParagraphs(t *testing.T) {
	doc := mustTestDocument(t, `<div id="root"><br>Lead <em>text</em><br><section>Block</section>Tail</div>`)
	root := doc.Find("#root").First().Get(0)

	wrapPhrasingContentInParagraphs(root)

	html, _ := doc.Find("#root").Html()
	if !strings.Contains(html, `<p>Lead <em>text</em></p>`) {
		t.Fatalf("phrasing content was not wrapped in %s", html)
	}
	if !strings.Contains(html, `<section>Block</section><p>Tail</p>`) {
		t.Fatalf("block boundary was not preserved in %s", html)
	}
}

func TestMetadataFromJSONLDUsesNameForGenericHeadline(t *testing.T) {
	result := metadataFromJSONLD(map[string]any{
		"@graph": []any{
			map[string]any{"@type": "WebPage", "headline": "Ignored"},
			map[string]any{
				"@type":         []any{"NewsArticle", "Thing"},
				"headline":      "latest article",
				"name":          "Specific story title",
				"author":        map[string]any{"name": "Jane Doe"},
				"description":   "Story summary",
				"publisher":     map[string]any{"name": "Example News"},
				"datePublished": "2024-01-02",
			},
		},
	})

	if result.Title != "Specific story title" {
		t.Fatalf("Title = %q", result.Title)
	}
	if result.Byline != "Jane Doe" {
		t.Fatalf("Byline = %q", result.Byline)
	}
	if result.SiteName != "Example News" {
		t.Fatalf("SiteName = %q", result.SiteName)
	}
}

func TestMetadataFromJSONLDRecognizesArticleSubtypes(t *testing.T) {
	result := metadataFromJSONLD(map[string]any{
		"@type":       "APIReference",
		"headline":    "API title",
		"description": "API summary",
	})

	if result.Title != "API title" {
		t.Fatalf("metadataFromJSONLD = %+v", result)
	}
}

func TestHiddenImageRequiresFallbackImageClass(t *testing.T) {
	hidden := mustTestDocument(t, `<img aria-hidden="true" src="photo.jpg">`).Find("img").First()
	if !isHidden(hidden) {
		t.Fatal("aria-hidden image without fallback class should be hidden")
	}

	fallback := mustTestDocument(t, `<img class="mwe-math-fallback-image" aria-hidden="true" src="math.png">`).Find("img").First()
	if isHidden(fallback) {
		t.Fatal("fallback math image should remain visible")
	}
}

func TestStructuredExcerptSkipsMediaWikiHatnotes(t *testing.T) {
	data := []byte(`<html><body class="mediawiki">
		<div id="mw-content-text">
			<p class="hatnote">For albums with similar titles, see another page.</p>
			<p>The actual article summary starts here with enough context to describe the page.</p>
		</div>
	</body></html>`)

	got := firstStructuredSourceExcerpt(data, "Example page")
	if got != "The actual article summary starts here with enough context to describe the page." {
		t.Fatalf("excerpt = %q", got)
	}
}

func TestUnwrapSingleCellTablesWrapsPhrasingContent(t *testing.T) {
	doc := mustTestDocument(t, `<article><table><tr><td>Lead <em>text</em></td></tr></table></article>`)
	article := doc.Find("article").First()

	unwrapSingleCellTables(article)

	html, _ := article.Html()
	if html != `<p>Lead <em>text</em></p>` {
		t.Fatalf("single-cell table html = %s", html)
	}
}

func TestInlineSVGAttributeCasingAndImageSelection(t *testing.T) {
	doc := mustTestDocument(t, `<figure><span> <img src="photo.jpg"> </span></figure><svg viewBox="0 0 1 1"></svg>`)

	if !isSingleImageSelection(doc.Find("figure").First()) {
		t.Fatal("figure should be treated as a single image selection")
	}

	svg := doc.Find("svg").First().Get(0)
	normalizeInlineSVGAttributeCasing(svg)
	if got := nodeAttr(svg, "viewbox"); got != "0 0 1 1" {
		t.Fatalf("normalized svg viewbox = %q", got)
	}
}

func TestWrapArticleSelectionMovesNodeIntoReadabilityWrapper(t *testing.T) {
	doc := mustTestDocument(t, `<main><article><p>Article body text</p></article></main>`)

	wrapped := wrapArticleSelection(doc.Find("article").First())

	if attr(wrapped, "id") != "readability-content" {
		t.Fatal("missing readability wrapper")
	}
	if wrapped.Find("article p").Text() != "Article body text" {
		html, _ := wrapped.Html()
		t.Fatalf("article was not moved into wrapper: %s", html)
	}
	if doc.Find("main article").Length() != 0 {
		t.Fatal("article should be moved out of its original parent")
	}
}

func mustTestDocument(t *testing.T, html string) *goquery.Document {
	t.Helper()
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	return doc
}
