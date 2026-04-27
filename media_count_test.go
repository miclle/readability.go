package readability

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
)

// TestPreservableMediaCountAudioWithSrc covers the <audio src=…> branch:
// an audio element with a source URL counts as preservable media even
// when no other attribute matches the video allow-list.
func TestPreservableMediaCountAudioWithSrc(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(
		`<div><audio src="https://example.com/podcast.mp3"></audio></div>`))
	if err != nil {
		t.Fatal(err)
	}
	cfg := defaultParserConfig()
	got := preservableMediaCount(doc.Find("div"), cfg)
	if got != 1 {
		t.Fatalf("preservableMediaCount = %d, want 1 (audio with src)", got)
	}
}

// TestPreservableMediaCountAudioWithoutSrc verifies the audio branch
// requires a non-empty src; bare <audio> tags do not count.
func TestPreservableMediaCountAudioWithoutSrc(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(`<div><audio></audio></div>`))
	if err != nil {
		t.Fatal(err)
	}
	cfg := defaultParserConfig()
	got := preservableMediaCount(doc.Find("div"), cfg)
	if got != 0 {
		t.Fatalf("preservableMediaCount = %d, want 0 (audio without src)", got)
	}
}

// TestPreservableMediaCountVideoBlobSrc covers the video blob: branch:
// player elements often surface their bound stream as a blob: URL, which
// must be treated as legitimate media even though the URL itself is
// useless to readers.
func TestPreservableMediaCountVideoBlobSrc(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(
		`<div><video src="blob:https://example.com/abc-123"></video></div>`))
	if err != nil {
		t.Fatal(err)
	}
	cfg := defaultParserConfig()
	got := preservableMediaCount(doc.Find("div"), cfg)
	if got != 1 {
		t.Fatalf("preservableMediaCount = %d, want 1 (video blob:)", got)
	}
}

// TestPreservableMediaCountVideoDataVideoID covers the data-video-id
// branch: many CMS templates render an empty <video> shell that gets
// hydrated by JS using data-video-id; this must still count as media.
func TestPreservableMediaCountVideoDataVideoID(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(
		`<div><video data-video-id="abc123"></video></div>`))
	if err != nil {
		t.Fatal(err)
	}
	cfg := defaultParserConfig()
	got := preservableMediaCount(doc.Find("div"), cfg)
	if got != 1 {
		t.Fatalf("preservableMediaCount = %d, want 1 (video data-video-id)", got)
	}
}

// TestPreservableMediaCountIframeAllowed covers the generic allow-list
// branch: an iframe whose src matches cfg.videoAllowed counts as
// preservable media.
func TestPreservableMediaCountIframeAllowed(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(
		`<div><iframe src="https://www.youtube.com/embed/abc"></iframe></div>`))
	if err != nil {
		t.Fatal(err)
	}
	cfg := defaultParserConfig()
	got := preservableMediaCount(doc.Find("div"), cfg)
	if got != 1 {
		t.Fatalf("preservableMediaCount = %d, want 1 (iframe in allow-list)", got)
	}
}

// TestPreservableMediaCountIframeUnrecognized verifies that iframes
// whose attributes do NOT match the allow-list are not counted as
// preservable media — they fall through to removableEmbedCount instead.
func TestPreservableMediaCountIframeUnrecognized(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(
		`<div><iframe src="https://ads.example.com/banner"></iframe></div>`))
	if err != nil {
		t.Fatal(err)
	}
	cfg := defaultParserConfig()
	got := preservableMediaCount(doc.Find("div"), cfg)
	if got != 0 {
		t.Fatalf("preservableMediaCount = %d, want 0 (unrecognized iframe)", got)
	}
}

// TestRemovableEmbedCountUnrecognized confirms that an iframe outside
// the allow-list bumps removable embed count, which conditional cleanup
// uses to push the candidate toward removal.
func TestRemovableEmbedCountUnrecognized(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(
		`<div><iframe src="https://ads.example.com/banner"></iframe></div>`))
	if err != nil {
		t.Fatal(err)
	}
	cfg := defaultParserConfig()
	got := removableEmbedCount(doc.Find("div"), cfg)
	if got != 1 {
		t.Fatalf("removableEmbedCount = %d, want 1 (unrecognized iframe)", got)
	}
}

// TestRemovableEmbedCountAllowedAttribute confirms that an iframe whose
// attribute matches the allow-list is NOT counted — preservableMediaCount
// will pick it up instead.
func TestRemovableEmbedCountAllowedAttribute(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(
		`<div><iframe src="https://www.youtube.com/embed/abc"></iframe></div>`))
	if err != nil {
		t.Fatal(err)
	}
	cfg := defaultParserConfig()
	got := removableEmbedCount(doc.Find("div"), cfg)
	if got != 0 {
		t.Fatalf("removableEmbedCount = %d, want 0 (iframe in allow-list)", got)
	}
}

// TestRemovableEmbedCountAllowedInnerHTML covers the inner-HTML allow
// path: an embed/object that holds the recognized URL inside a child
// node rather than on its own attributes (e.g. <object><param value=…/>
// </object>) must also be considered allowed.
func TestRemovableEmbedCountAllowedInnerHTML(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(
		`<div><object><param name="movie" value="https://www.youtube.com/embed/abc"/></object></div>`))
	if err != nil {
		t.Fatal(err)
	}
	cfg := defaultParserConfig()
	got := removableEmbedCount(doc.Find("div"), cfg)
	if got != 0 {
		t.Fatalf("removableEmbedCount = %d, want 0 (allowed via inner HTML)", got)
	}
}

// TestRemovableEmbedCountObjectAndEmbed covers the multi-element path:
// object and embed tags are inspected alongside iframe.
func TestRemovableEmbedCountObjectAndEmbed(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(
		`<div>
			<object data="https://ads.example.com/x"></object>
			<embed src="https://ads.example.com/y">
			<iframe src="https://ads.example.com/z"></iframe>
		</div>`))
	if err != nil {
		t.Fatal(err)
	}
	cfg := defaultParserConfig()
	got := removableEmbedCount(doc.Find("div"), cfg)
	if got != 3 {
		t.Fatalf("removableEmbedCount = %d, want 3 (three unrecognized embeds)", got)
	}
}
