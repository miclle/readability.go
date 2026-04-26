package readability

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	xhtml "golang.org/x/net/html"
)

func TestCleanGenericBylineTrimsMetadataLines(t *testing.T) {
	got := cleanGenericByline("By JANE DOE\nEditor notes\n2h")
	if got != "By JANE DOE" {
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

func TestFirstSourceBylineSkipsFooterAuthors(t *testing.T) {
	data := []byte(`<html><body>
		<article><p>Useful article text with enough context to stand on its own.</p></article>
		<footer class="footer-author"><span itemprop="author"><span itemprop="name">Footer Author</span></span></footer>
		<div class="post-footer"><a rel="author">Post Footer Author</a></div>
	</body></html>`)

	if byline := firstSourceByline(data, ""); byline != "" {
		t.Fatalf("byline = %q", byline)
	}
}

func TestFirstSourceBylineSkipsProfileWidgets(t *testing.T) {
	data := []byte(`<html><body>
		<article><p>Useful article text with enough context to stand on its own.</p></article>
		<div class="widget Profile"><a rel="author" class="profile-name-link">Sidebar Author</a></div>
	</body></html>`)

	if byline := firstSourceByline(data, ""); byline != "" {
		t.Fatalf("byline = %q", byline)
	}
}

func TestFirstSourceBylineKeepsArticleAuthorProfileInfo(t *testing.T) {
	data := []byte(`<html><body>
		<div class="authorInfo" section="author">
			<div class="profileInfo"><a rel="author"><span itemprop="name">Article Author</span></a></div>
		</div>
	</body></html>`)

	if byline := firstSourceByline(data, ""); byline != "Article Author" {
		t.Fatalf("byline = %q", byline)
	}
}

func TestFirstSourceBylineSkipsCompoundAttributionBlocks(t *testing.T) {
	data := []byte(`<html><body>
		<article><p>Useful article text with enough context to stand on its own.</p></article>
		<div class="byline">
			<a class="byline__author">Jane Reporter</a>
			<div class="byline__title">Staff Writer</div>
		</div>
		<div class="article-author"><a><span>Joe Writer</span></a><time>Monday, February 29, 2016 @ 11:10 PM UTC</time></div>
	</body></html>`)

	if byline := firstSourceByline(data, ""); byline != "" {
		t.Fatalf("byline = %q", byline)
	}
}

func TestFirstSourceBylineKeepsDatedBylines(t *testing.T) {
	data := []byte(`<html><body>
		<div class="FeatureByline">By <b>Nathan Willis</b> March 25, 2015</div>
	</body></html>`)

	if byline := firstSourceByline(data, ""); byline != "By Nathan Willis March 25, 2015" {
		t.Fatalf("byline = %q", byline)
	}

	data = []byte(`<html><body>
		<div class="byline">Last Updated: January 7, 2025</div>
	</body></html>`)

	if byline := firstSourceByline(data, ""); byline != "Last Updated: January 7, 2025" {
		t.Fatalf("byline = %q", byline)
	}
}

func TestFirstSourceBylineSkipsEntryBylineMetadataLines(t *testing.T) {
	data := []byte(`<html><body>
		<div class="entry-byline">
			<span class="entry-author" itemprop="author"><a rel="author"><span itemprop="name">Entry Author</span></a></span>
			<time datetime="2017-03-09T18:16:02-04:00">March 9, 2017</time>
			<a class="comments-link" itemprop="discussionURL">13</a>
		</div>
	</body></html>`)

	if byline := firstSourceByline(data, ""); byline != "" {
		t.Fatalf("byline = %q", byline)
	}
}

func TestSelectionInnerHTMLPreservesTextQuotes(t *testing.T) {
	doc := mustTestDocument(t, `<div id="root"><p title="'quoted'">You're&nbsp;"ready"</p></div>`)

	html, err := selectionInnerHTML(doc.Find("#root"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `You're&nbsp;"ready"`) {
		t.Fatalf("text quotes were escaped: %q", html)
	}
	if !strings.Contains(html, `title="&apos;quoted&apos;"`) {
		t.Fatalf("attribute quotes were not escaped safely: %q", html)
	}
}

func TestSelectionInnerHTMLNormalizesAttributeQuoteEntities(t *testing.T) {
	doc := mustTestDocument(t, `<div id="root"><img alt="Photo by &quot;Jane&quot;" data-x="'single'"></div>`)

	html, err := selectionInnerHTML(doc.Find("#root"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `alt="Photo by &quot;Jane&quot;"`) {
		t.Fatalf("double quotes were not normalized in attributes: %q", html)
	}
	if !strings.Contains(html, `data-x="&apos;single&apos;"`) {
		t.Fatalf("single quotes were not normalized in attributes: %q", html)
	}
}

func TestNormalizeTextContentEntitiesPreservesNBSP(t *testing.T) {
	got := normalizeTextContentEntities("A\u00a0B", true)
	if got != "A&nbsp;B" {
		t.Fatalf("normalizeTextContentEntities = %q", got)
	}

	got = normalizeTextContentEntities("A\u00a0B", false)
	if got != "A\u00a0B" {
		t.Fatalf("normalizeTextContentEntities without named NBSP = %q", got)
	}
}

func TestSelectionInnerHTMLUsesPreferredNBSP(t *testing.T) {
	doc := mustTestDocument(t, `<div id="root"><p>A&nbsp;B</p></div>`)

	html, err := selectionInnerHTMLWithNBSP(doc.Find("#root"), "&#160;")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html, `A&#160;B`) {
		t.Fatalf("NBSP was not serialized with preferred entity: %q", html)
	}
}

func TestWrapArticleSelectionPreservesCandidateDirection(t *testing.T) {
	doc := mustTestDocument(t, `<html dir="ltr"><body><main id="story"><p>Story text.</p></main></body></html>`)

	wrapped := wrapArticleSelection(doc.Find("#story"))
	if dir := articleDirection(wrapped); dir != "ltr" {
		t.Fatalf("Dir = %q", dir)
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

func TestMetadataFromJSONLDIgnoresStringAuthor(t *testing.T) {
	result := metadataFromJSONLD(map[string]any{
		"@type":    "SocialMediaPosting",
		"headline": "Post title",
		"author":   "blog-name",
	})

	if result.Byline != "" {
		t.Fatalf("Byline = %q", result.Byline)
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

func TestFirstExcerptTextSkipsTableText(t *testing.T) {
	doc := mustTestDocument(t, `<article>
		<table><tr><td><p>Mozilla Corporation Mozilla Foundation</p></td></tr></table>
		<p>Mozilla is a free-software community, created in 1998 by members of Netscape.</p>
	</article>`)

	got := firstExcerptText(doc.Find("article"), "Mozilla - Wikipedia")
	if got != "Mozilla is a free-software community, created in 1998 by members of Netscape." {
		t.Fatalf("excerpt = %q", got)
	}
}

func TestFirstExcerptTextUsesFirstShortParagraph(t *testing.T) {
	doc := mustTestDocument(t, `<article>
		<p>Contents</p>
		<p>Once you have mastered the art of mutable history in a single repository, you can move up to the next level.</p>
	</article>`)

	got := firstExcerptText(doc.Find("article"), "Evolve")
	if got != "Contents" {
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

func TestCleanArticleCandidateRemovesLayoutTable(t *testing.T) {
	doc := mustTestDocument(t, `<article>
		<p>This article paragraph contains enough text to keep the article meaningful after cleanup.</p>
		<table><tr><td><a href="/one">One</a></td><td><a href="/two">Two</a></td></tr></table>
	</article>`)
	article := doc.Find("article").First()

	cleanArticleCandidate(article)

	if article.Find("table").Length() != 0 {
		html, _ := article.Html()
		t.Fatalf("layout table was kept in %s", html)
	}
}

func TestCleanArticleCandidateKeepsDataTable(t *testing.T) {
	doc := mustTestDocument(t, `<article>
		<table>
			<caption>Quarterly results</caption>
			<thead><tr><th>Quarter</th><th>Revenue</th></tr></thead>
			<tbody><tr><td>Q1</td><td>$10</td></tr><tr><td>Q2</td><td>$12</td></tr></tbody>
		</table>
	</article>`)
	article := doc.Find("article").First()

	cleanArticleCandidate(article)

	if article.Find("table").Length() != 1 {
		html, _ := article.Html()
		t.Fatalf("data table was removed from %s", html)
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
