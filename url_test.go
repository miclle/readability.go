package readability

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

func TestResolveDocumentURLsUsesDocumentBase(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(`
		<html><head><base href="/base/"></head><body>
			<a href="story.html">story</a>
			<img src="image.png" srcset="/wide.jpg 2x, thumb.jpg 1x">
			<iframe src="//player.example/embed"></iframe>
		</body></html>`))
	if err != nil {
		t.Fatal(err)
	}

	resolveDocumentURLs(doc, "https://Example.COM/articles/page.html")

	if got := attr(doc.Find("a").First(), "href"); got != "https://example.com/base/story.html" {
		t.Fatalf("a href = %q", got)
	}
	if got := attr(doc.Find("img").First(), "src"); got != "https://example.com/base/image.png" {
		t.Fatalf("img src = %q", got)
	}
	if got := attr(doc.Find("img").First(), "srcset"); got != "https://example.com/wide.jpg 2x, https://example.com/base/thumb.jpg 1x" {
		t.Fatalf("img srcset = %q", got)
	}
	if got := attr(doc.Find("iframe").First(), "src"); got != "//player.example/embed" {
		t.Fatalf("iframe src = %q", got)
	}
}

func TestResolveDocumentURLsKeepsFragmentWithoutBaseElement(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(`<a href="#section">section</a>`))
	if err != nil {
		t.Fatal(err)
	}

	resolveDocumentURLs(doc, "https://example.com/article")

	if got := attr(doc.Find("a").First(), "href"); got != "#section" {
		t.Fatalf("fragment href = %q", got)
	}
}
