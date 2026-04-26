package readability

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseMozillaFixture001(t *testing.T) {
	sourcePath := filepath.Join("testdata", "test-pages", "001", "source.html")
	f, err := os.Open(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	article, err := FromReader(f, "http://fakehost/code/2013/get-your-frontend-javascript-code-covered/", nil)
	if err != nil {
		t.Fatal(err)
	}

	if article.Title != "Get your Frontend JavaScript Code Covered | Code" {
		t.Fatalf("Title = %q", article.Title)
	}
	if article.Byline != "Nicolas Perriault" {
		t.Fatalf("Byline = %q", article.Byline)
	}
	if article.Excerpt != "Nicolas Perriault's homepage." {
		t.Fatalf("Excerpt = %q", article.Excerpt)
	}
	if article.Lang != "en" {
		t.Fatalf("Lang = %q", article.Lang)
	}
	if !strings.Contains(article.TextContent, "testing your frontend JavaScript code") {
		t.Fatalf("TextContent does not contain expected article text")
	}
	if !strings.Contains(article.Content, `<div id="readability-page-1" class="page">`) {
		t.Fatalf("Content does not contain readability page wrapper")
	}
}

func TestIsProbablyReaderableMozillaFixture001(t *testing.T) {
	sourcePath := filepath.Join("testdata", "test-pages", "001", "source.html")
	f, err := os.Open(sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	ok, err := IsProbablyReaderable(f)
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("fixture 001 should be readerable")
	}
}

func TestFromReaderHonorsCharThreshold(t *testing.T) {
	html := strings.NewReader(`<html><body><article><p>This article has enough useful text to be extracted by the parser.</p></article></body></html>`)
	article, err := FromReader(html, "https://example.com/article", &Options{CharThreshold: 10})
	if err != nil {
		t.Fatal(err)
	}
	if article.TextContent == "" {
		t.Fatal("TextContent is empty below CharThreshold")
	}

	html = strings.NewReader(`<html><body><article><p>This article has enough useful text to be extracted by the parser.</p></article></body></html>`)
	article, err = FromReader(html, "https://example.com/article", &Options{CharThreshold: 500})
	if err != nil {
		t.Fatal(err)
	}
	if article.Content != "" || article.TextContent != "" || article.Length != 0 {
		t.Fatalf("article below CharThreshold was returned: length=%d content=%q", article.Length, article.Content)
	}
}

func TestFromReaderRetriesWhenOnlyArticleLooksUnlikely(t *testing.T) {
	html := strings.NewReader(`<html><body>
		<div class="comment">
			<p>This is the actual article body with enough useful text to survive the readability character threshold once unlikely stripping is relaxed.</p>
		</div>
	</body></html>`)

	article, err := FromReader(html, "https://example.com/story", &Options{CharThreshold: 50})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(article.TextContent, "actual article body") {
		t.Fatalf("TextContent = %q", article.TextContent)
	}
}

func TestFromReaderRetriesWithoutClassWeights(t *testing.T) {
	html := strings.NewReader(`<html><body>
		<div class="article"><p>This short teaser is not enough.</p></div>
		<div class="comment">
			<p>This is the actual article body with substantially more useful text than the teaser, and it should win once class weighting is relaxed by the retry flow.</p>
			<p>Additional real article text makes the candidate long enough to pass the configured character threshold.</p>
		</div>
	</body></html>`)

	article, err := FromReader(html, "https://example.com/story", &Options{CharThreshold: 100})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(article.TextContent, "substantially more useful text") {
		t.Fatalf("TextContent = %q", article.TextContent)
	}
}

func TestFromReaderRetriesWithoutConditionalCleanup(t *testing.T) {
	html := strings.NewReader(`<html><body>
		<div class="comment">
			<p><a href="/story">This linked article body is still the only meaningful content on the page, and it is long enough that the parser should return it once conditional cleanup is relaxed.</a></p>
			<p><a href="/story2">A second linked paragraph keeps the text substantial even though the link density is intentionally very high.</a></p>
		</div>
	</body></html>`)

	article, err := FromReader(html, "https://example.com/story", &Options{CharThreshold: 100})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(article.TextContent, "only meaningful content") {
		t.Fatalf("TextContent = %q", article.TextContent)
	}
}
