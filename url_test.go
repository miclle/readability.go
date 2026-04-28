package readability

import (
	"net/url"
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

// TestResolveSrcsetSingleEntry pins the simplest srcset: one URL with a
// density descriptor. Verifies the URL is resolved relative to base and
// the descriptor is preserved verbatim.
func TestResolveSrcsetSingleEntry(t *testing.T) {
	base, _ := url.Parse("https://example.com/articles/")
	got := resolveSrcset("photo.jpg 2x", base)
	if got != "https://example.com/articles/photo.jpg 2x" {
		t.Fatalf("got %q", got)
	}
}

// TestResolveSrcsetMultipleEntries verifies the comma-separated path:
// each candidate URL must be resolved independently against base, and
// commas plus inter-entry whitespace must round-trip unchanged.
func TestResolveSrcsetMultipleEntries(t *testing.T) {
	base, _ := url.Parse("https://example.com/a/b/")
	got := resolveSrcset("small.jpg 1x, /big.jpg 2x, https://cdn.example.com/huge.jpg 3x", base)
	want := "https://example.com/a/b/small.jpg 1x, https://example.com/big.jpg 2x, https://cdn.example.com/huge.jpg 3x"
	if got != want {
		t.Fatalf("got  %q\nwant %q", got, want)
	}
}

// TestResolveSrcsetWidthDescriptor exercises the "w" descriptor variant
// (width hint) in addition to "x" (density).
func TestResolveSrcsetWidthDescriptor(t *testing.T) {
	base, _ := url.Parse("https://example.com/")
	got := resolveSrcset("a.jpg 320w, b.jpg 640w", base)
	if got != "https://example.com/a.jpg 320w, https://example.com/b.jpg 640w" {
		t.Fatalf("got %q", got)
	}
}

// TestResolveSrcsetEmpty confirms the empty-input fast path returns
// empty without panicking — resolveDocumentURLs uses that signal to skip
// SetAttr.
func TestResolveSrcsetEmpty(t *testing.T) {
	base, _ := url.Parse("https://example.com/")
	if got := resolveSrcset("", base); got != "" {
		t.Fatalf("empty input should return empty, got %q", got)
	}
}

// TestResolveSrcsetUppercaseHostLowercased mirrors the host-folding
// behavior already applied to <a href> / <img src>: srcset URLs land in
// the same lowercase canonical form so deduplication and comparison are
// consistent across the document.
func TestResolveSrcsetUppercaseHostLowercased(t *testing.T) {
	base, _ := url.Parse("https://example.com/")
	got := resolveSrcset("https://CDN.Example.COM/p.jpg 2x", base)
	if got != "https://cdn.example.com/p.jpg 2x" {
		t.Fatalf("host should be lowercased, got %q", got)
	}
}

// TestResolveDocumentURLsResolvesAllResourceAttrs confirms every
// resource selector listed in resolveDocumentURLs (a/img/source/video/
// audio/iframe) gets its URL resolved when no <base> element is present.
// The previous tests focused on srcset / fragments / base href; this one
// nails down the per-tag coverage.
func TestResolveDocumentURLsResolvesAllResourceAttrs(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(`
		<html><body>
			<a href="story.html">story</a>
			<img src="image.png">
			<picture><source src="alt.webp"></picture>
			<video src="movie.mp4"></video>
			<audio src="podcast.mp3"></audio>
			<iframe src="https://player.example/embed"></iframe>
		</body></html>`))
	if err != nil {
		t.Fatal(err)
	}
	resolveDocumentURLs(doc, "https://example.com/articles/page.html")

	checks := map[string]string{
		"a":      "https://example.com/articles/story.html",
		"img":    "https://example.com/articles/image.png",
		"source": "https://example.com/articles/alt.webp",
		"video":  "https://example.com/articles/movie.mp4",
		"audio":  "https://example.com/articles/podcast.mp3",
		// iframe absolute URLs are left as-is, host lowercased.
		"iframe": "https://player.example/embed",
	}
	for sel, want := range checks {
		var got string
		if sel == "a" {
			got = attr(doc.Find(sel).First(), "href")
		} else {
			got = attr(doc.Find(sel).First(), "src")
		}
		if got != want {
			t.Fatalf("%s: got %q, want %q", sel, got, want)
		}
	}
}

// TestResolveDocumentURLsPreservesRelativeIframeProtocol pins the
// "//host/path" iframe special-case: protocol-relative iframe sources
// must NOT be rewritten to absolute, matching upstream behavior that
// preserves embed providers' protocol-agility.
func TestResolveDocumentURLsPreservesRelativeIframeProtocol(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(
		`<iframe src="//player.example/embed/abc"></iframe>`))
	if err != nil {
		t.Fatal(err)
	}
	resolveDocumentURLs(doc, "https://example.com/")
	if got := attr(doc.Find("iframe").First(), "src"); got != "//player.example/embed/abc" {
		t.Fatalf("iframe src should remain protocol-relative, got %q", got)
	}
}
