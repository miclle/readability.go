package readability

import (
	"testing"

	xhtml "golang.org/x/net/html"
)

// helper: build a simple element node with optional id.
func newElem(tag, id string) *xhtml.Node {
	n := &xhtml.Node{Type: xhtml.ElementNode, Data: tag}
	if id != "" {
		n.Attr = []xhtml.Attribute{{Key: "id", Val: id}}
	}
	return n
}

func TestBetterAncestorCandidateClimbsToHigherScoringParent(t *testing.T) {
	body := newElem("body", "")
	grandparent := newElem("section", "outer")
	parent := newElem("div", "wrapper")
	top := newElem("article", "")
	body.AppendChild(grandparent)
	grandparent.AppendChild(parent)
	grandparent.AppendChild(newElem("aside", "")) // disable single-child climb
	parent.AppendChild(top)
	parent.AppendChild(newElem("p", "")) // disable single-child climb at parent

	scores := map[*xhtml.Node]float64{
		top:         30,
		parent:      40, // higher than topScore -> should climb to parent and stop
		grandparent: 25,
	}

	got, score := betterAncestorCandidate(top, scores, 30)
	if got != parent {
		t.Fatalf("expected to climb to parent, got %v", tagNameNode(got))
	}
	if score != 40 {
		t.Fatalf("expected score 40, got %v", score)
	}
}

func TestBetterAncestorCandidateBreaksOnBelowThreshold(t *testing.T) {
	body := newElem("body", "")
	grandparent := newElem("section", "")
	parent := newElem("div", "")
	top := newElem("article", "")
	body.AppendChild(grandparent)
	grandparent.AppendChild(parent)
	grandparent.AppendChild(newElem("aside", "")) // disable single-child climb
	parent.AppendChild(top)
	parent.AppendChild(newElem("p", "")) // disable single-child climb at parent

	// parent's score < topScore/3 -> loop should break and stay on top.
	scores := map[*xhtml.Node]float64{
		top:    30,
		parent: 5,
	}

	got, score := betterAncestorCandidate(top, scores, 30)
	if got != top {
		t.Fatalf("expected to stay on top, got %v", tagNameNode(got))
	}
	if score != 30 {
		t.Fatalf("expected score 30, got %v", score)
	}
}

func TestBetterAncestorCandidatePromotesSingleChildParent(t *testing.T) {
	body := newElem("body", "")
	grandparent := newElem("section", "")
	parent := newElem("div", "")
	top := newElem("article", "")
	body.AppendChild(grandparent)
	grandparent.AppendChild(parent) // parent is the only child of grandparent
	parent.AppendChild(top)         // top is the only child of parent

	scores := map[*xhtml.Node]float64{
		top: 30,
	}

	got, _ := betterAncestorCandidate(top, scores, 30)
	// Single-child climb walks parent -> grandparent. grandparent's only child
	// is parent, so the climb continues until it would hit body, where the
	// loop is forced to stop.
	if got != grandparent {
		t.Fatalf("expected to climb single-child chain to grandparent, got %v",
			tagNameNode(got))
	}
}

func TestBetterAncestorCandidatePromotesPostsAncestor(t *testing.T) {
	// compatPostsAncestor short-circuits when top.Parent has id="posts" and
	// linkDensity < 0.25. Build a parent with text-only child so density is 0.
	body := newElem("body", "")
	posts := newElem("div", "posts")
	top := newElem("article", "")
	body.AppendChild(posts)
	posts.AppendChild(top)
	posts.AppendChild(&xhtml.Node{Type: xhtml.TextNode, Data: "lots of text body content"})

	scores := map[*xhtml.Node]float64{
		top:   30,
		posts: 20,
	}

	got, score := betterAncestorCandidate(top, scores, 30)
	if got != posts {
		t.Fatalf("expected promotion to #posts, got %v", tagNameNode(got))
	}
	if score < 30 {
		t.Fatalf("expected score >= 30, got %v", score)
	}
}

func TestBetterSharedAncestorCandidateReturnsTopWhenFewCandidates(t *testing.T) {
	body := newElem("body", "")
	parent := newElem("div", "")
	top := newElem("article", "")
	body.AppendChild(parent)
	parent.AppendChild(top)

	candidates := []*xhtml.Node{top}
	scores := map[*xhtml.Node]float64{top: 30}

	got, score := betterSharedAncestorCandidate(top, candidates, scores, 30, articleScoringOptions{})
	if got != top || score != 30 {
		t.Fatalf("expected unchanged top/score, got %v/%v", tagNameNode(got), score)
	}
}

func TestBetterSharedAncestorCandidateClimbsToSharedAncestor(t *testing.T) {
	// Build:
	//   body
	//     shared
	//       top
	//       sibA
	//       sibB
	//       sibC
	body := newElem("body", "")
	shared := newElem("section", "shared")
	top := newElem("article", "")
	sibA := newElem("article", "a")
	sibB := newElem("article", "b")
	sibC := newElem("article", "c")
	body.AppendChild(shared)
	for _, n := range []*xhtml.Node{top, sibA, sibB, sibC} {
		shared.AppendChild(n)
	}

	// All candidates have score >= 0.75 * topScore so they all qualify.
	scores := map[*xhtml.Node]float64{
		top:  100,
		sibA: 80,
		sibB: 80,
		sibC: 80,
	}
	candidates := []*xhtml.Node{top, sibA, sibB, sibC}

	got, _ := betterSharedAncestorCandidate(top, candidates, scores, 100, articleScoringOptions{})
	if got != shared {
		t.Fatalf("expected to promote to shared ancestor, got %v", tagNameNode(got))
	}
	if _, ok := scores[shared]; !ok {
		t.Fatal("expected shared ancestor to be scored")
	}
}

func TestBetterSharedAncestorCandidateRespectsScoreCutoff(t *testing.T) {
	body := newElem("body", "")
	shared := newElem("section", "shared")
	top := newElem("article", "")
	sibA := newElem("article", "a")
	sibB := newElem("article", "b")
	sibC := newElem("article", "c")
	body.AppendChild(shared)
	for _, n := range []*xhtml.Node{top, sibA, sibB, sibC} {
		shared.AppendChild(n)
	}

	// All siblings are below 0.75 * topScore so the qualifying set drops
	// under the minimumTopCandidates threshold.
	scores := map[*xhtml.Node]float64{
		top:  100,
		sibA: 50,
		sibB: 50,
		sibC: 50,
	}
	candidates := []*xhtml.Node{top, sibA, sibB, sibC}

	got, score := betterSharedAncestorCandidate(top, candidates, scores, 100, articleScoringOptions{})
	if got != top || score != 100 {
		t.Fatalf("expected unchanged top/score, got %v/%v", tagNameNode(got), score)
	}
}
