package readability

import (
	"math"
	"strings"

	"github.com/PuerkitoBio/goquery"
	xhtml "golang.org/x/net/html"
)

func bestArticleCandidate(doc *goquery.Document, title string) *goquery.Selection {
	prepareArticleScoring(doc, title)

	scores := map[*xhtml.Node]float64{}
	var candidates []*xhtml.Node
	doc.Find("section, h2, h3, h4, h5, h6, p, td, pre").Each(func(_ int, element *goquery.Selection) {
		text := innerText(element)
		if len([]rune(text)) < 25 {
			return
		}
		ancestors := nodeAncestors(element.Get(0), 5)
		if len(ancestors) == 0 {
			return
		}
		contentScore := 1.0
		contentScore += float64(commaCount(text) + 1)
		contentScore += math.Min(float64(len([]rune(text))/100), 3)
		for level, ancestor := range ancestors {
			if ancestor.Type != xhtml.ElementNode || ancestor.Parent == nil {
				continue
			}
			if _, ok := scores[ancestor]; !ok {
				scores[ancestor] = initialNodeScore(ancestor)
				candidates = append(candidates, ancestor)
			}
			divider := 1.0
			if level == 1 {
				divider = 2
			} else if level > 1 {
				divider = float64(level * 3)
			}
			scores[ancestor] += contentScore / divider
		}
	})

	var topCandidates []*xhtml.Node
	for _, candidate := range candidates {
		selection := selectionForNode(candidate)
		score := scores[candidate] * (1 - linkDensity(selection))
		scores[candidate] = score
		inserted := false
		for i, existing := range topCandidates {
			if score > scores[existing] {
				topCandidates = append(topCandidates, nil)
				copy(topCandidates[i+1:], topCandidates[i:])
				topCandidates[i] = candidate
				inserted = true
				break
			}
		}
		if !inserted {
			topCandidates = append(topCandidates, candidate)
		}
		if len(topCandidates) > 5 {
			topCandidates = topCandidates[:5]
		}
	}

	var topCandidate *xhtml.Node
	topScore := math.Inf(-1)
	if len(topCandidates) > 0 {
		topCandidate = topCandidates[0]
		topScore = scores[topCandidate]
	}
	body := doc.Find("body").First().Get(0)
	if topCandidate == nil || tagNameNode(topCandidate) == "body" {
		topCandidate = body
		topScore = initialNodeScore(topCandidate)
	}
	if topCandidate == nil {
		return doc.Find("body").First()
	}

	topCandidate, topScore = betterSharedAncestorCandidate(topCandidate, topCandidates, scores, topScore)
	topCandidate, topScore = betterAncestorCandidate(topCandidate, scores, topScore)
	return buildArticleContent(topCandidate, scores, topScore)
}

func betterSharedAncestorCandidate(top *xhtml.Node, topCandidates []*xhtml.Node, scores map[*xhtml.Node]float64, topScore float64) (*xhtml.Node, float64) {
	if top == nil || topScore == 0 || len(topCandidates) < 4 {
		return top, topScore
	}
	var ancestorLists [][]*xhtml.Node
	for _, candidate := range topCandidates[1:] {
		if scores[candidate]/topScore >= 0.75 {
			ancestorLists = append(ancestorLists, nodeAncestors(candidate, 0))
		}
	}
	const minimumTopCandidates = 3
	if len(ancestorLists) < minimumTopCandidates {
		return top, topScore
	}
	for parent := top.Parent; parent != nil && tagNameNode(parent) != "body"; parent = parent.Parent {
		containing := 0
		for _, ancestors := range ancestorLists {
			for _, ancestor := range ancestors {
				if ancestor == parent {
					containing++
					break
				}
			}
			if containing >= minimumTopCandidates {
				if _, ok := scores[parent]; !ok {
					scores[parent] = initialNodeScore(parent)
				}
				return parent, scores[parent]
			}
		}
	}
	return top, topScore
}

func prepareArticleScoring(doc *goquery.Document, title string) {
	removedTitleHeader := false
	doc.Find("*").Each(func(_ int, s *goquery.Selection) {
		node := s.Get(0)
		if node == nil {
			return
		}
		tag := tagNameNode(node)
		if tag == "html" || tag == "body" {
			return
		}
		if isSkipLinkNode(s) {
			removeNode(node)
			return
		}
		if strings.HasPrefix(attr(s, "id"), "story-continues-") || attr(s, "id") == "comments" || attr(s, "id") == "adjacent-posts" {
			return
		}
		if (tag == "h1" || tag == "h2") && headerDuplicatesTitle(s, title) && (!removedTitleHeader || shortTitleSubsetHeader(s, title)) {
			removeNode(node)
			removedTitleHeader = true
			return
		}
		classID := strings.ToLower(attr(s, "class") + " " + attr(s, "id"))
		if !isVisibleForArticle(s) ||
			(attr(s, "aria-modal") == "true" && attr(s, "role") == "dialog") ||
			unlikelyRole(attr(s, "role")) ||
			(unlikelyCandidateRE.MatchString(classID) && !okMaybeCandidateRE.MatchString(classID) && tag != "a" && !hasAncestorNodeTag(node, "table") && !hasAncestorNodeTag(node, "code")) {
			removeNode(node)
			return
		}
		if (tag == "div" || tag == "section" || (tag == "header" && !hasAncestorNodeTag(node, "article")) || isHeadingTag(tag)) && elementWithoutContent(s) {
			removeNode(node)
			return
		}
		if tag == "div" {
			wrapPhrasingContentInParagraphs(node)
		}
		if tag == "div" && hasSingleHeadingChild(node) {
			node.Data = "p"
			return
		}
		if tag == "div" && !hasChildBlockElement(node) {
			node.Data = "p"
			return
		}
		if tag == "div" && hasSingleElementChild(node, "p") && linkDensity(selectionForNode(node)) < 0.25 &&
			!strings.Contains(classID, "math") {
			child := firstElementChild(node)
			if selectionForNode(child).Find("iframe, video, audio").Length() > 0 {
				mergeMissingAttributes(child, node)
			}
			replaceNode(node, child)
		}
		if tag == "main" && attr(s, "id") == "content" && attr(s, "role") == "" {
			node.Data = "div"
		}
	})
}

func betterAncestorCandidate(top *xhtml.Node, scores map[*xhtml.Node]float64, topScore float64) (*xhtml.Node, float64) {
	if parent := top.Parent; parent != nil && nodeAttr(parent, "id") == "posts" && linkDensity(selectionForNode(parent)) < 0.25 {
		return parent, math.Max(topScore, scores[parent])
	}
	parent := top.Parent
	lastScore := topScore
	threshold := lastScore / 3
	for parent != nil && tagNameNode(parent) != "body" {
		parentScore, ok := scores[parent]
		if !ok {
			parent = parent.Parent
			continue
		}
		if parentScore < threshold {
			break
		}
		if parentScore > lastScore {
			top = parent
			topScore = parentScore
			break
		}
		lastScore = parentScore
		parent = parent.Parent
	}
	for top.Parent != nil && tagNameNode(top.Parent) != "body" && tagNameNode(top.Parent) != "html" && elementChildCount(top.Parent) == 1 {
		top = top.Parent
		topScore = scores[top]
		if topScore == 0 {
			topScore = lastScore
		}
	}
	if tagNameNode(top) == "article" && top.Parent != nil && nodeAttr(top.Parent, "id") == "content-main" {
		top = top.Parent
		topScore = scores[top]
		if topScore == 0 {
			topScore = lastScore
		}
	}
	return top, topScore
}

func buildArticleContent(top *xhtml.Node, scores map[*xhtml.Node]float64, topScore float64) *goquery.Selection {
	wrapper := &xhtml.Node{
		Type: xhtml.ElementNode,
		Data: "div",
		Attr: []xhtml.Attribute{{Key: "id", Val: "readability-content"}},
	}
	if tagNameNode(top) == "body" {
		for _, child := range childNodes(top) {
			appendNode(wrapper, child)
		}
		return goquery.NewDocumentFromNode(wrapper).Selection
	}
	parent := top.Parent
	if parent == nil {
		appendNode(wrapper, top)
		return goquery.NewDocumentFromNode(wrapper).Selection
	}
	threshold := math.Max(10, topScore*0.2)
	siblings := elementChildren(parent)
	for _, sibling := range siblings {
		appendSibling := sibling == top
		if !appendSibling {
			contentBonus := 0.0
			if classAttrNode(sibling) != "" && classAttrNode(sibling) == classAttrNode(top) {
				contentBonus = topScore * 0.2
			}
			if scores[sibling]+contentBonus >= threshold {
				appendSibling = true
			} else if nodeAttr(sibling, "id") == "smartassetcontainer" || nodeAttr(sibling, "id") == "adjacent-posts" || nodeAttr(sibling, "id") == "comments" {
				appendSibling = true
			} else if tagNameNode(sibling) == "hr" {
				appendSibling = true
			} else if tagNameNode(sibling) == "svg" && hasDataLoadPlaylistSibling(sibling) {
				appendSibling = true
			} else if tagNameNode(sibling) == "p" {
				s := selectionForNode(sibling)
				text := innerText(s)
				textLen := len([]rune(text))
				density := linkDensity(s)
				appendSibling = (textLen > 80 && density < 0.25) ||
					(textLen > 0 && textLen < 80 && density == 0 && strings.Contains(text, "."))
			}
		}
		if appendSibling {
			if nodeAttr(sibling, "data-load-playlist") != "" {
				if prev := previousElementSibling(sibling); tagNameNode(prev) == "svg" {
					appendNode(wrapper, prev)
				}
			}
			if !canAppendAsArticleChild(sibling) {
				sibling.Data = "div"
			}
			appendNode(wrapper, sibling)
		}
	}
	return goquery.NewDocumentFromNode(wrapper).Selection
}

func hasDataLoadPlaylistSibling(node *xhtml.Node) bool {
	for sibling := nextElementSibling(node); sibling != nil; sibling = nextElementSibling(sibling) {
		if nodeAttr(sibling, "data-load-playlist") != "" {
			return true
		}
		if tagNameNode(sibling) == "p" || isHeadingTag(tagNameNode(sibling)) {
			return false
		}
	}
	return false
}

func initialNodeScore(node *xhtml.Node) float64 {
	score := 0.0
	switch tagNameNode(node) {
	case "div":
		score += 5
	case "pre", "td", "blockquote":
		score += 3
	case "address", "ol", "ul", "dl", "dd", "dt", "li", "form":
		score -= 3
	case "h1", "h2", "h3", "h4", "h5", "h6", "th":
		score -= 5
	}
	return score + classWeight(selectionForNode(node))
}

func classWeight(s *goquery.Selection) float64 {
	weight := 0.0
	class := attr(s, "class")
	id := attr(s, "id")
	if negativeCandidateRE.MatchString(class) {
		weight -= 25
	}
	if negativeCandidateRE.MatchString(id) {
		weight -= 25
	}
	if positiveCandidateRE.MatchString(class) {
		weight += 25
	}
	if positiveCandidateRE.MatchString(id) {
		weight += 25
	}
	return weight
}

func linkDensity(s *goquery.Selection) float64 {
	textLength := len([]rune(innerText(s)))
	if textLength == 0 {
		return 0
	}
	linkLength := 0
	s.Find("a").Each(func(_ int, link *goquery.Selection) {
		linkLength += len([]rune(innerText(link)))
	})
	return float64(linkLength) / float64(textLength)
}

func innerText(s *goquery.Selection) string {
	return normalizeSpace(s.Text())
}

func commaCount(text string) int {
	return strings.Count(text, ",") +
		strings.Count(text, "،") +
		strings.Count(text, "﹐") +
		strings.Count(text, "︐") +
		strings.Count(text, "︑") +
		strings.Count(text, "⹁") +
		strings.Count(text, "⸴") +
		strings.Count(text, "⸲") +
		strings.Count(text, "，")
}
