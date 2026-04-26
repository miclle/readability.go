package readability

import (
	"testing"

	xhtml "golang.org/x/net/html"
)

func TestFirstRunes(t *testing.T) {
	tests := []struct {
		name string
		in   string
		n    int
		want string
	}{
		{"ascii prefix", "hello world", 5, "hello"},
		{"n exceeds length", "abc", 10, "abc"},
		{"zero", "abc", 0, ""},
		{"multibyte", "中文测试abc", 3, "中文测"},
		{"empty", "", 4, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := firstRunes(tt.in, tt.n); got != tt.want {
				t.Fatalf("firstRunes(%q, %d) = %q, want %q", tt.in, tt.n, got, tt.want)
			}
		})
	}
}

func TestRemoveNodeAttr(t *testing.T) {
	node := &xhtml.Node{
		Type: xhtml.ElementNode,
		Data: "div",
		Attr: []xhtml.Attribute{
			{Key: "id", Val: "a"},
			{Key: "data-reactid", Val: "1"},
			{Key: "DATA-reactid", Val: "2"}, // case-insensitive removal
			{Key: "class", Val: "b"},
		},
	}
	removeNodeAttr(node, "data-reactid")
	if len(node.Attr) != 2 {
		t.Fatalf("expected 2 attrs left, got %d: %+v", len(node.Attr), node.Attr)
	}
	if node.Attr[0].Key != "id" || node.Attr[1].Key != "class" {
		t.Fatalf("unexpected remaining attrs: %+v", node.Attr)
	}
}

func TestRemoveFollowingWhitespace(t *testing.T) {
	parent := &xhtml.Node{Type: xhtml.ElementNode, Data: "div"}
	target := &xhtml.Node{Type: xhtml.ElementNode, Data: "p"}
	ws1 := &xhtml.Node{Type: xhtml.TextNode, Data: "  \n"}
	ws2 := &xhtml.Node{Type: xhtml.TextNode, Data: "\t"}
	tail := &xhtml.Node{Type: xhtml.ElementNode, Data: "span"}

	parent.AppendChild(target)
	parent.AppendChild(ws1)
	parent.AppendChild(ws2)
	parent.AppendChild(tail)

	removeFollowingWhitespace(target)

	if target.NextSibling != tail {
		t.Fatalf("expected target.NextSibling == tail, got %+v", target.NextSibling)
	}
	if tail.PrevSibling != target {
		t.Fatalf("expected tail.PrevSibling == target, got %+v", tail.PrevSibling)
	}
}

func TestTrailingNumber(t *testing.T) {
	tests := map[string]string{
		"":            "",
		"abc":         "",
		"abc1":        "1",
		"page42":      "42",
		"continue123": "123",
		"42":          "42",
		"abc0":        "0",
	}
	for in, want := range tests {
		if got := trailingNumber(in); got != want {
			t.Fatalf("trailingNumber(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNextTrailingNumberID(t *testing.T) {
	tests := map[string]string{
		"continue1":   "continue2",
		"continue9":   "continue10",
		"page99":      "page100",
		"no-number":   "",
		"":            "",
		"abc009":      "abc010",
	}
	for in, want := range tests {
		if got := nextTrailingNumberID(in); got != want {
			t.Fatalf("nextTrailingNumberID(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIncrementDecimalString(t *testing.T) {
	tests := map[string]string{
		"0":   "1",
		"9":   "10",
		"19":  "20",
		"99":  "100",
		"100": "101",
		"999": "1000",
	}
	for in, want := range tests {
		if got := incrementDecimalString(in); got != want {
			t.Fatalf("incrementDecimalString(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCleanTitle(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"pipe greater than 2", "Site | Cat | Article", "Site | Cat"},
		{"middot two parts", "An interesting article headline · Site Name", "An interesting article headline"},
		{"colon prefix short tag", "Tag: This is the actual title body", "This is the actual title body"},
		{"no separator", "Plain title", "Plain title"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cleanTitle(tt.in); got != tt.want {
				t.Fatalf("cleanTitle(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestArticleAuthor(t *testing.T) {
	if got := articleAuthor("https://example.com/author/jane"); got != "" {
		t.Fatalf("articleAuthor(URL) = %q, want empty", got)
	}
	if got := articleAuthor("Jane Doe"); got != "Jane Doe" {
		t.Fatalf("articleAuthor(name) = %q, want %q", got, "Jane Doe")
	}
}

func TestNormalizeSpace(t *testing.T) {
	tests := map[string]string{
		"":                       "",
		"   ":                    "",
		"foo  bar":               "foo bar",
		"\tfoo\nbar\r\n":         "foo bar",
		"  leading and trailing ": "leading and trailing",
		"a\f\fb":                 "a b",
	}
	for in, want := range tests {
		if got := normalizeSpace(in); got != want {
			t.Fatalf("normalizeSpace(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUnescapeMetadataString(t *testing.T) {
	got := unescapeMetadataString("a&nbsp;b &amp; c")
	want := "a&nbsp;b & c"
	if got != want {
		t.Fatalf("unescapeMetadataString = %q, want %q", got, want)
	}
}
