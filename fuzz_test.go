package readability

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// FuzzFromReader exercises FromReader with arbitrary HTML byte sequences.
// The intent is to catch panics, infinite loops, and out-of-bounds errors
// produced by the parser pipeline when fed malformed or adversarial markup.
// It does not assert on output content; valid extractions, empty results,
// ErrBelowCharThreshold, and ErrTooManyElements are all acceptable.
func FuzzFromReader(f *testing.F) {
	filler := strings.Repeat("filler ", 30)
	seeds := []string{
		"",
		"<html></html>",
		"<html><body></body></html>",
		"<html><body><article><p>Hello world. This article is short.</p></article></body></html>",
		"<!doctype html><html><body><article><h1>Title</h1><p>" +
			strings.Repeat("Lorem ipsum dolor sit amet. ", 30) + "</p></article></body></html>",
		"<html><head><meta charset=\"utf-8\"><title>T</title></head><body><p>x</p></body></html>",
		"<html><body><script>var x=1;</script><style>.a{}</style><p>after</p></body></html>",
		"<html><body><div><div><div><div><p>nested</p></div></div></div></div></body></html>",
		"<html><body>" + strings.Repeat("<br>", 50) + "<p>x</p></body></html>",
		"<<<<>>>><html<body<<>><p>",
		// lazy-image recovery (mirrors media_clean_test.go cases so
		// fuzz mutators explore that branch).
		`<html><body><article><p>` + filler + `</p><img class="lazy" src="placeholder.gif" data-original="https://cdn.example.com/real.jpg"></article></body></html>`,
		`<html><body><article><p>` + filler + `</p><img src="data:image/gif;base64,R0lGODlhAQABAAAAACw=" data-src="https://cdn.example.com/real.jpg"></article></body></html>`,
		`<html><body><article><p>` + filler + `</p><img src="data:image/svg+xml;base64,PHN2Zy8+" data-src="https://cdn.example.com/x.jpg"></article></body></html>`,
		`<html><body><article><p>` + filler + `</p><figure data-src="https://cdn.example.com/synth.jpg"><figcaption>cap</figcaption></figure></article></body></html>`,
		`<html><body><article><p>` + filler + `</p><img class="lazy" data-srcset="https://cdn.example.com/s.jpg 1x, https://cdn.example.com/l.jpg 2x"></article></body></html>`,
		// javascript: anchor cleanup branches.
		`<html><body><article><p>` + filler + `</p><p>see <a href="javascript:doStuff()">click me</a> for more</p></article></body></html>`,
		`<html><body><article><p>` + filler + `</p><p><a href="javascript:doStuff()"><strong>bold</strong></a></p></article></body></html>`,
		// <br><br> excerpt split.
		`<html><body><article><p>summary lead<br><br>after-the-cut text</p><p>` + filler + `</p></article></body></html>`,
		// embedded media — preservable / removable counters.
		`<html><body><article><p>` + filler + `</p><audio src="https://example.com/p.mp3"></audio></article></body></html>`,
		`<html><body><article><p>` + filler + `</p><video src="blob:https://example.com/abc"></video></article></body></html>`,
		`<html><body><article><p>` + filler + `</p><video data-video-id="abc"></video></article></body></html>`,
		`<html><body><article><p>` + filler + `</p><iframe src="https://www.youtube.com/embed/abc"></iframe></article></body></html>`,
		`<html><body><article><p>` + filler + `</p><iframe src="https://ads.example.com/banner"></iframe></article></body></html>`,
		// JSON-LD + og:title metadata path.
		`<html><head><script type="application/ld+json">{"@context":"https://schema.org","@type":"NewsArticle","headline":"H","datePublished":"2024-01-01"}</script><meta property="og:title" content="O"></head><body><article><p>` + filler + `</p></article></body></html>`,
		// invalid UTF-8 inside otherwise-valid markup.
		"<html><body><article><p>prefix \xff\xfe middle " + filler + "</p></article></body></html>",
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		// Cap input size to keep individual iterations fast; the goal is
		// adversarial structure, not raw payload size.
		if len(data) > 64*1024 {
			data = data[:64*1024]
		}
		_, err := FromReader(bytes.NewReader(data), "https://example.com/page", &Options{
			CharThreshold:   1,
			MaxElemsToParse: 5000,
		})
		// Any non-sentinel error is treated as unexpected; the parser
		// should normally either succeed or return a known sentinel.
		if err != nil &&
			!errors.Is(err, ErrBelowCharThreshold) &&
			!errors.Is(err, ErrTooManyElements) {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}

// FuzzIsProbablyReaderable exercises the readerability pre-check on
// arbitrary HTML byte sequences. It must never panic and must never
// produce a non-nil error for inputs that goquery accepts.
func FuzzIsProbablyReaderable(f *testing.F) {
	filler := strings.Repeat("text ", 100)
	seeds := []string{
		"",
		"<html></html>",
		"<html><body><p>Short</p></body></html>",
		"<html><body><article><p>" + filler + "</p></article></body></html>",
		"<html><body><div hidden><p>hidden</p></div></body></html>",
		// readerable triage corner cases mirrored from readerable_test.go.
		`<html><body><p style="display:none">` + filler + `</p></body></html>`,
		`<html><body><div class="comment"><p>` + filler + `</p></div></body></html>`,
		`<html><body><ul><li><p>` + filler + `</p></li></ul></body></html>`,
		`<html><body><div>line one<br><br>` + filler + `</div></body></html>`,
		`<html><body><p aria-hidden="true">` + filler + `</p></body></html>`,
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 64*1024 {
			data = data[:64*1024]
		}
		if _, err := IsProbablyReaderable(bytes.NewReader(data)); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
