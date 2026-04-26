package readability

import (
	"errors"
	"regexp"
	"strings"
	"testing"
)

func TestOptionsKeepClassesPreservesAllClasses(t *testing.T) {
	html := strings.NewReader(`<html><body><article><p class="lead highlight">This article has enough useful text to be extracted by the parser when running with KeepClasses enabled.</p></article></body></html>`)
	article, err := FromReader(html, "https://example.com/story", &Options{KeepClasses: true})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(article.Content, `class="lead highlight"`) {
		t.Fatalf("Content does not preserve original classes: %q", article.Content)
	}
}

func TestOptionsClassesToPreserveAllowsCustom(t *testing.T) {
	html := strings.NewReader(`<html><body><article><p class="custom-keep ad-promo">This article has enough useful text to be extracted by the parser when running with a custom ClassesToPreserve list.</p></article></body></html>`)
	article, err := FromReader(html, "https://example.com/story", &Options{ClassesToPreserve: []string{"custom-keep"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(article.Content, `class="custom-keep"`) {
		t.Fatalf("Content does not contain preserved class: %q", article.Content)
	}
	if strings.Contains(article.Content, "ad-promo") {
		t.Fatalf("Content unexpectedly preserved unlisted class: %q", article.Content)
	}
}

func TestOptionsClassesToPreserveDefaultsToCaption(t *testing.T) {
	html := strings.NewReader(`<html><body><article><figure><img src="/x.jpg"><figcaption class="caption">A caption that should survive cleanup intact.</figcaption></figure><p>This article has enough useful text to be extracted by the parser using the default ClassesToPreserve list.</p></article></body></html>`)
	article, err := FromReader(html, "https://example.com/story", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(article.Content, `class="caption"`) {
		t.Fatalf("Content does not preserve default caption class: %q", article.Content)
	}
}

func TestOptionsAllowedVideoRegexExtendsAllowList(t *testing.T) {
	const sample = `<html><body><article><p>This article has enough useful text to be extracted by the parser even when an embedded iframe references an unusual provider.</p><p>A second paragraph keeps the candidate substantial so cleanup does not strip the article body during conditional cleanup.</p><iframe src="https://videos.example.com/embed/abc123"></iframe></article></body></html>`
	defaultArticle, err := FromReader(strings.NewReader(sample), "https://example.com/story", nil)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(defaultArticle.Content, "videos.example.com") {
		t.Fatalf("Default video allow-list unexpectedly preserved iframe: %q", defaultArticle.Content)
	}

	customArticle, err := FromReader(strings.NewReader(sample), "https://example.com/story", &Options{
		AllowedVideoRegex: regexp.MustCompile(`videos\.example\.com`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(customArticle.Content, "videos.example.com") {
		t.Fatalf("Custom AllowedVideoRegex did not preserve iframe: %q", customArticle.Content)
	}
}

func TestOptionsMaxElemsToParseAborts(t *testing.T) {
	var builder strings.Builder
	builder.WriteString(`<html><body><article>`)
	for i := 0; i < 50; i++ {
		builder.WriteString(`<p>filler paragraph that exists only to inflate the document element count beyond the configured threshold.</p>`)
	}
	builder.WriteString(`</article></body></html>`)

	_, err := FromReader(strings.NewReader(builder.String()), "https://example.com/story", &Options{MaxElemsToParse: 10})
	if !errors.Is(err, ErrTooManyElements) {
		t.Fatalf("expected ErrTooManyElements, got %v", err)
	}
}

func TestOptionsDisableJSONLDSkipsStructuredMetadata(t *testing.T) {
	html := `<html><head>
<script type="application/ld+json">{"@context":"https://schema.org","@type":"NewsArticle","headline":"Structured Title","datePublished":"2024-01-01"}</script>
<meta property="og:title" content="OG Fallback Title">
</head><body><article><p>This article body has enough useful text to make the parser return a non-empty article structure for the title comparison.</p></article></body></html>`

	defaultArticle, err := FromReader(strings.NewReader(html), "https://example.com/story", nil)
	if err != nil {
		t.Fatal(err)
	}
	if defaultArticle.Title != "Structured Title" {
		t.Fatalf("Default Title = %q, want %q", defaultArticle.Title, "Structured Title")
	}

	disabledArticle, err := FromReader(strings.NewReader(html), "https://example.com/story", &Options{DisableJSONLD: true})
	if err != nil {
		t.Fatal(err)
	}
	if disabledArticle.Title != "OG Fallback Title" {
		t.Fatalf("DisableJSONLD Title = %q, want %q", disabledArticle.Title, "OG Fallback Title")
	}
}

func TestOptionsNbTopCandidatesUsesProvidedValue(t *testing.T) {
	html := strings.NewReader(`<html><body>
		<article><p>This article body has enough useful text to be extracted regardless of how many top candidates the scorer is configured to track.</p></article>
	</body></html>`)
	article, err := FromReader(html, "https://example.com/story", &Options{NbTopCandidates: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(article.TextContent, "enough useful text") {
		t.Fatalf("TextContent = %q", article.TextContent)
	}
}
