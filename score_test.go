package readability

import (
	"math"
	"testing"

	xhtml "golang.org/x/net/html"
)

func TestHasDataLoadPlaylistSibling(t *testing.T) {
	parent := &xhtml.Node{Type: xhtml.ElementNode, Data: "div"}
	svg := &xhtml.Node{Type: xhtml.ElementNode, Data: "svg"}
	playlist := &xhtml.Node{
		Type: xhtml.ElementNode,
		Data: "div",
		Attr: []xhtml.Attribute{{Key: "data-load-playlist", Val: "1"}},
	}
	parent.AppendChild(svg)
	parent.AppendChild(&xhtml.Node{Type: xhtml.TextNode, Data: "\n"})
	parent.AppendChild(playlist)

	if !hasDataLoadPlaylistSibling(svg) {
		t.Fatal("expected playlist sibling after svg")
	}
}

func TestHasDataLoadPlaylistSiblingStopsAtParagraph(t *testing.T) {
	parent := &xhtml.Node{Type: xhtml.ElementNode, Data: "div"}
	svg := &xhtml.Node{Type: xhtml.ElementNode, Data: "svg"}
	paragraph := &xhtml.Node{Type: xhtml.ElementNode, Data: "p"}
	playlist := &xhtml.Node{
		Type: xhtml.ElementNode,
		Data: "div",
		Attr: []xhtml.Attribute{{Key: "data-load-playlist", Val: "1"}},
	}
	parent.AppendChild(svg)
	parent.AppendChild(paragraph)
	parent.AppendChild(playlist)

	if hasDataLoadPlaylistSibling(svg) {
		t.Fatal("paragraph boundary should stop playlist search")
	}
}

func TestLinkDensityDiscountsHashLinks(t *testing.T) {
	doc := mustTestDocument(t, `<div>abcdefghij<a href="#section">klmnopqrst</a></div>`)

	got := linkDensity(doc.Find("div").First())
	if math.Abs(got-0.15) > 0.0001 {
		t.Fatalf("linkDensity = %f, want 0.15", got)
	}
}
