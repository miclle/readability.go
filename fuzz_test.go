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
	seeds := []string{
		"",
		"<html></html>",
		"<html><body><p>Short</p></body></html>",
		"<html><body><article><p>" + strings.Repeat("text ", 100) + "</p></article></body></html>",
		"<html><body><div hidden><p>hidden</p></div></body></html>",
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
