package readability

import (
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	xhtml "golang.org/x/net/html"
)

// TestInitialNodeScoreTagBuckets exercises every switch arm in
// initialNodeScoreWithOptions, including the WeightClasses path.
func TestInitialNodeScoreTagBuckets(t *testing.T) {
	cases := []struct {
		tag  string
		want float64
	}{
		{"div", 5},
		{"pre", 3},
		{"td", 3},
		{"blockquote", 3},
		{"address", -3},
		{"ol", -3},
		{"ul", -3},
		{"dl", -3},
		{"dd", -3},
		{"dt", -3},
		{"li", -3},
		{"form", -3},
		{"h1", -5},
		{"h2", -5},
		{"h3", -5},
		{"h4", -5},
		{"h5", -5},
		{"h6", -5},
		{"th", -5},
		{"span", 0}, // default arm
	}
	for _, c := range cases {
		got := initialNodeScoreWithOptions(newElem(c.tag, ""), articleScoringOptions{})
		if got != c.want {
			t.Errorf("tag=%s: got %v, want %v", c.tag, got, c.want)
		}
	}
}

func TestInitialNodeScoreWeightClasses(t *testing.T) {
	node := &xhtml.Node{Type: xhtml.ElementNode, Data: "div",
		Attr: []xhtml.Attribute{{Key: "class", Val: "article"}}}
	got := initialNodeScoreWithOptions(node, articleScoringOptions{WeightClasses: true})
	// div (+5) + positive class match (+25) = 30
	if got != 30 {
		t.Fatalf("WeightClasses score = %v, want 30", got)
	}
}

// TestBuildArticleContentBodyTopCandidate hits the body branch where all
// child nodes are appended directly into the wrapper.
func TestBuildArticleContentBodyTopCandidate(t *testing.T) {
	body := newElem("body", "")
	body.AppendChild(newElem("p", ""))
	body.AppendChild(newElem("p", ""))

	sel := buildArticleContent(body, map[*xhtml.Node]float64{}, 0)
	if sel.Length() == 0 {
		t.Fatal("expected non-empty selection")
	}
	if sel.Find("p").Length() != 2 {
		t.Fatalf("expected 2 paragraphs, got %d", sel.Find("p").Length())
	}
}

// TestBuildArticleContentSiblingBranches exercises the hr/p/long-p inclusion
// rules and the "rewrite to div" fallback.
func TestBuildArticleContentSiblingBranches(t *testing.T) {
	body := newElem("body", "")
	parent := newElem("div", "")
	body.AppendChild(parent)

	top := newElem("article", "")
	hr := newElem("hr", "")

	longP := newElem("p", "")
	longText := strings.Repeat("paragraph text with content. ", 6) // > 80 chars, has period
	longP.AppendChild(&xhtml.Node{Type: xhtml.TextNode, Data: longText})

	shortP := newElem("p", "")
	shortP.AppendChild(&xhtml.Node{Type: xhtml.TextNode, Data: "Short sentence."})

	rejectedP := newElem("p", "")
	// Empty p — no text, density 0, contains no period — should be rejected.

	li := newElem("li", "")
	// li scores high enough to pass the threshold but is not a valid article child.

	for _, n := range []*xhtml.Node{top, hr, longP, shortP, rejectedP, li} {
		parent.AppendChild(n)
	}

	scores := map[*xhtml.Node]float64{
		top: 50,
		li:  100, // forces threshold inclusion
	}

	sel := buildArticleContent(top, scores, 50)
	if sel.Find("hr").Length() != 1 {
		t.Errorf("expected hr included, got %d", sel.Find("hr").Length())
	}
	if sel.Find("p").Length() < 2 {
		t.Errorf("expected long+short paragraphs included, got %d", sel.Find("p").Length())
	}
	// li should have been rewritten to div.
	if sel.Find("div").Length() == 0 {
		t.Errorf("expected li rewritten to div")
	}
	if sel.Find("li").Length() != 0 {
		t.Errorf("expected no remaining li elements")
	}
}

func TestBuildArticleContentClassMatchBonus(t *testing.T) {
	body := newElem("body", "")
	parent := newElem("div", "")
	body.AppendChild(parent)

	top := &xhtml.Node{Type: xhtml.ElementNode, Data: "article",
		Attr: []xhtml.Attribute{{Key: "class", Val: "story"}}}
	sib := &xhtml.Node{Type: xhtml.ElementNode, Data: "div",
		Attr: []xhtml.Attribute{{Key: "class", Val: "story"}}}
	parent.AppendChild(top)
	parent.AppendChild(sib)

	// scores[sib]=11; threshold = max(10, 50*0.2) = 10. With a class-match
	// bonus of 50*0.2 = 10, the sibling sails over the threshold.
	scores := map[*xhtml.Node]float64{
		top: 50,
		sib: 11,
	}
	sel := buildArticleContent(top, scores, 50)
	// One class="story" article + one class="story" div should both appear.
	if sel.Find("[class='story']").Length() != 2 {
		t.Fatalf("expected class-match bonus to include sibling, got %d",
			sel.Find("[class='story']").Length())
	}
}

func TestHasDataLoadPlaylistSiblingShortCircuit(t *testing.T) {
	parent := newElem("div", "")
	target := newElem("svg", "")
	playlist := &xhtml.Node{Type: xhtml.ElementNode, Data: "div",
		Attr: []xhtml.Attribute{{Key: "data-load-playlist", Val: "1"}}}
	parent.AppendChild(target)
	parent.AppendChild(playlist)
	if !hasDataLoadPlaylistSibling(target) {
		t.Fatal("expected playlist sibling to be detected")
	}

	// p sibling between svg and playlist short-circuits the search.
	parent2 := newElem("div", "")
	target2 := newElem("svg", "")
	parent2.AppendChild(target2)
	parent2.AppendChild(newElem("p", ""))
	parent2.AppendChild(&xhtml.Node{Type: xhtml.ElementNode, Data: "div",
		Attr: []xhtml.Attribute{{Key: "data-load-playlist", Val: "1"}}})
	if hasDataLoadPlaylistSibling(target2) {
		t.Fatal("expected p sibling to short-circuit detection")
	}
}

func TestDirectionForCandidateBodyAndAncestor(t *testing.T) {
	html := &xhtml.Node{Type: xhtml.ElementNode, Data: "html",
		Attr: []xhtml.Attribute{{Key: "dir", Val: "rtl"}}}
	body := newElem("body", "")
	html.AppendChild(body)
	if got := directionForCandidate(body); got != "rtl" {
		t.Errorf("body candidate dir = %q, want rtl", got)
	}

	// nil → empty string
	if got := directionForCandidate(nil); got != "" {
		t.Errorf("nil dir = %q", got)
	}

	// ancestor walk for non-body candidate
	root := &xhtml.Node{Type: xhtml.ElementNode, Data: "div",
		Attr: []xhtml.Attribute{{Key: "dir", Val: "ltr"}}}
	mid := newElem("div", "")
	leaf := newElem("article", "")
	root.AppendChild(mid)
	mid.AppendChild(leaf)
	if got := directionForCandidate(leaf); got != "ltr" {
		t.Errorf("ancestor dir = %q, want ltr", got)
	}
}

// TestPrepareArticleScoringSkipAndDialog verifies prepareArticleScoring
// removes skip-links, aria-modal dialogs, and unlikely-role elements while
// keeping #comments / continuation markers intact.
func TestPrepareArticleScoringSkipAndDialog(t *testing.T) {
	html := `<html><body>
<a class="skip-link" href="#main">Skip to content</a>
<div role="dialog" aria-modal="true">popup</div>
<div role="navigation">menu</div>
<div id="comments">comments stay</div>
<main id="content"><p>The article body has enough useful text to be picked as the top candidate during scoring.</p></main>
</body></html>`

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	prepareArticleScoring(doc, "", articleScoringOptions{StripUnlikely: true})

	if doc.Find(".skip-link").Length() != 0 {
		t.Error("skip-link should have been removed")
	}
	if doc.Find("[aria-modal='true']").Length() != 0 {
		t.Error("aria-modal dialog should have been removed")
	}
	if doc.Find("[role='navigation']").Length() != 0 {
		t.Error("unlikely-role navigation should have been removed")
	}
	if doc.Find("#comments").Length() != 1 {
		t.Error("comments wrapper should be retained")
	}
	if doc.Find("main#content").Length() != 0 {
		t.Error("main#content should have been renamed to div")
	}
	if doc.Find("div#content").Length() != 1 {
		t.Error("expected main rewritten to div with id=content")
	}
}

func TestPrepareArticleScoringRemovesDuplicateHeader(t *testing.T) {
	html := `<html><body>
<h1>The Quick Brown Fox</h1>
<p>The article body has enough useful text to be picked as the top candidate during scoring.</p>
</body></html>`
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		t.Fatal(err)
	}
	prepareArticleScoring(doc, "The Quick Brown Fox", articleScoringOptions{StripUnlikely: true})
	if doc.Find("h1").Length() != 0 {
		t.Fatal("expected duplicate-title h1 to be removed")
	}
}
