package readability

import "testing"

// TestB64DataURLRE pins the data: URL detector used by fixLazyImages
// to spot base64 placeholders. Captures both positive and negative cases
// so the regex cannot silently drift to a more or less permissive form.
func TestB64DataURLRE(t *testing.T) {
	cases := []struct {
		name  string
		input string
		match bool
		mime  string
	}{
		{"png base64", "data:image/png;base64,AAAA", true, "image/png"},
		{"gif base64", "data:image/gif;base64,abc=", true, "image/gif"},
		{"svg base64", "data:image/svg+xml;base64,xyz=", true, "image/svg+xml"},
		{"upper-case prefix", "DATA:image/png;BASE64,Z", true, "image/png"},
		{"whitespace around mime", "data: image/png ; base64 ,Z", true, "image/png"},
		{"jpeg base64", "data:image/jpeg;base64,A", true, "image/jpeg"},
		{"missing base64 token", "data:image/png,AAAA", false, ""},
		{"plain http URL", "https://example.com/x.png", false, ""},
		{"empty", "", false, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := b64DataURLRE.MatchString(tc.input)
			if got != tc.match {
				t.Fatalf("MatchString(%q) = %v, want %v", tc.input, got, tc.match)
			}
			if !tc.match {
				return
			}
			matches := b64DataURLRE.FindStringSubmatch(tc.input)
			if len(matches) < 2 || matches[1] != tc.mime {
				t.Fatalf("captured mime = %q, want %q", matches, tc.mime)
			}
		})
	}
}

// TestImageURLRE pins the single-URL image detector. fixLazyImages uses
// it to decide whether a sibling attribute holds a real image URL that
// would justify dropping a base64 src placeholder.
func TestImageURLRE(t *testing.T) {
	cases := []struct {
		name  string
		input string
		match bool
	}{
		{"plain jpg", "https://cdn.example.com/photo.jpg", true},
		{"plain png", "/static/photo.png", true},
		{"webp", "/static/photo.webp", true},
		{"jpeg", "/static/photo.jpeg", true},
		{"with query string", "/photo.png?v=1", true},
		{"with fragment", "/photo.png#anchor", true},
		// imageURLRE deliberately requires .ext to be followed by " <digit>"
		// (srcset descriptor) or one of ?#$. Bare "x.jpg foo" should NOT
		// match — that's the imageSrcsetRE's job, but only when the
		// trailing token is a digit.
		{"srcset descriptor", "https://cdn.example.com/photo.jpg 2x", true},
		{"bare extension followed by space+text", "/photo.jpg foo", false},
		{"unsupported gif", "/static/photo.gif", false},
		{"no extension", "/static/photo", false},
		{"empty", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := imageURLRE.MatchString(tc.input)
			if got != tc.match {
				t.Fatalf("MatchString(%q) = %v, want %v", tc.input, got, tc.match)
			}
		})
	}
}

// TestImageSrcsetRE pins the srcset shape detector. fixLazyImages uses
// it to decide whether a data-* attribute should be copied into srcset
// (URL + density descriptor) versus src (single URL).
func TestImageSrcsetRE(t *testing.T) {
	cases := []struct {
		name  string
		input string
		match bool
	}{
		{"single url + 1x", "/photo.jpg 1x", true},
		{"single url + 2x", "https://cdn.example.com/large.png 2x", true},
		{"single url + width", "/photo.webp 800w", true},
		{"comma-separated set", "/s.jpg 1x, /l.jpg 2x", true},
		// Plain URLs without a descriptor should NOT match — they would
		// be copied to src instead.
		{"plain url", "/photo.jpg", false},
		{"plain url with query", "/photo.png?v=1", false},
		{"text only", "no image here", false},
		{"empty", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := imageSrcsetRE.MatchString(tc.input)
			if got != tc.match {
				t.Fatalf("MatchString(%q) = %v, want %v", tc.input, got, tc.match)
			}
		})
	}
}
