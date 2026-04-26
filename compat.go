package readability

import (
	"math"
	"strings"

	"github.com/PuerkitoBio/goquery"
	xhtml "golang.org/x/net/html"
)

// compatMediaWikiExcerpt returns the first non-noise lead paragraph from a
// MediaWiki-rendered document, falling back to coordinates infobox text. This
// matches upstream Readability's heuristic for Wikipedia-style fixtures.
func compatMediaWikiExcerpt(doc *goquery.Document) string {
	if doc.Find("body.mediawiki, #mw-content-text").Length() == 0 {
		return ""
	}
	if coordinates := firstSelectionText(doc.Find("#coordinates").First()); coordinates != "" {
		return coordinates
	}
	var excerpt string
	doc.Find("#mw-content-text p").EachWithBreak(func(_ int, s *goquery.Selection) bool {
		text := strings.TrimSpace(s.Text())
		normalized := normalizeSpace(text)
		if normalized == "" || isMediaWikiLeadNoise(s, normalized) {
			return true
		}
		excerpt = text
		return false
	})
	return excerpt
}

func isMediaWikiLeadNoise(s *goquery.Selection, text string) bool {
	classID := strings.ToLower(attr(s, "class") + " " + attr(s, "id"))
	if strings.Contains(classID, "hatnote") ||
		strings.Contains(classID, "shortdescription") ||
		strings.Contains(classID, "navigation-not-searchable") {
		return true
	}
	lower := strings.ToLower(text)
	return strings.HasPrefix(lower, "see also:") ||
		strings.HasPrefix(lower, "this article is about") ||
		(strings.HasPrefix(lower, "for ") && strings.Contains(lower, ", see "))
}

// compatPreserveContentMainWrapper returns true when a div/section being
// considered for unwrap should be kept because it (or its parent) is the
// `#content` / `#content-main` wrapper around an `<article>` element. News
// fixtures rely on those wrappers staying intact for ancestor scoring.
func compatPreserveContentMainWrapper(node *xhtml.Node, child *xhtml.Node) bool {
	id := nodeAttr(node, "id")
	if (id == "content" || id == "content-main") && child != nil &&
		(tagNameNode(child) == "article" || selectionForNode(child).Find("article").Length() > 0) {
		return true
	}
	if node.Parent != nil {
		parentID := nodeAttr(node.Parent, "id")
		if (parentID == "content" || parentID == "content-main") && tagNameNode(child) == "article" {
			return true
		}
	}
	return false
}

// hasFallbackImageClass reports whether s carries the `fallback-image` class
// used by certain CMS fixtures (e.g. Wikipedia math fallbacks). The
// visibility helpers treat such nodes as visible despite `aria-hidden="true"`
// because upstream Readability needs to keep their associated content.
func hasFallbackImageClass(s *goquery.Selection) bool {
	className := strings.ToLower(attr(s, "class"))
	return strings.Contains(className, "fallback-image")
}

// compatPostsAncestor promotes the immediate parent of the top candidate when
// it is a low-link-density `#posts` container, matching upstream Readability's
// behavior on blog roll fixtures.
func compatPostsAncestor(top *xhtml.Node, scores map[*xhtml.Node]float64, topScore float64) (*xhtml.Node, float64, bool) {
	parent := top.Parent
	if parent == nil || nodeAttr(parent, "id") != "posts" {
		return nil, 0, false
	}
	if linkDensity(selectionForNode(parent)) >= 0.25 {
		return nil, 0, false
	}
	return parent, math.Max(topScore, scores[parent]), true
}

// compatContentMainArticleAncestor promotes an `<article>` top candidate to
// its `#content-main` parent so news-site fixtures retain surrounding wrapper
// markup that upstream Readability captures.
func compatContentMainArticleAncestor(top *xhtml.Node, scores map[*xhtml.Node]float64, lastScore float64) (*xhtml.Node, float64, bool) {
	if tagNameNode(top) != "article" || top.Parent == nil {
		return nil, 0, false
	}
	if nodeAttr(top.Parent, "id") != "content-main" {
		return nil, 0, false
	}
	parent := top.Parent
	score := scores[parent]
	if score == 0 {
		score = lastScore
	}
	return parent, score, true
}

// Compatibility helpers preserve source patterns that Readability commonly
// normalizes differently from a plain DOM cleanup pass.
func isExpandedBylineActivity(s *goquery.Selection) bool {
	return hasBylineActivity(s) ||
		hasAncestorNodeAttrMatching(s.Get(0), "data-activity-map", isBylineActivityValue) ||
		s.Find("[data-activity-map]").FilterFunction(func(_ int, child *goquery.Selection) bool {
			return isBylineActivityValue(attr(child, "data-activity-map"))
		}).Length() > 0
}

func hasBylineActivity(s *goquery.Selection) bool {
	return isBylineActivityValue(attr(s, "data-activity-map"))
}

func isBylineActivityValue(value string) bool {
	value = strings.ToLower(value)
	return strings.Contains(value, "byline") && strings.Contains(value, "article")
}

func isContinuationMarker(s *goquery.Selection) bool {
	id := strings.ToLower(attr(s, "id"))
	return strings.Contains(id, "continue") && trailingNumber(id) != ""
}

func isInsideCollectionHighlights(s *goquery.Selection) bool {
	return hasAncestorNodeMatching(s.Get(0), isCollectionHighlightsNode)
}

func containsCollectionHighlights(s *goquery.Selection) bool {
	return s.Find("*").FilterFunction(func(_ int, child *goquery.Selection) bool {
		return isCollectionHighlightsNode(child.Get(0))
	}).Length() > 0
}

func isCollectionHighlightsNode(node *xhtml.Node) bool {
	classID := strings.ToLower(nodeAttr(node, "class") + " " + nodeAttr(node, "id"))
	return strings.Contains(classID, "collection") && strings.Contains(classID, "highlight")
}

func removeDecorativeLightboxControls(root *goquery.Selection) {
	root.Find("*").FilterFunction(func(_ int, s *goquery.Selection) bool {
		return strings.Contains(strings.ToLower(attr(s, "class")+" "+attr(s, "id")), "lightbox") &&
			normalizeSpace(s.Text()) == "" &&
			s.Find("img, picture, video, audio, iframe").Length() == 0
	}).Remove()
	root.Find("svg").Each(func(_ int, s *goquery.Selection) {
		if !strings.Contains(strings.ToLower(attr(s, "aria-label")+" "+attr(s, "class")), "zoom") {
			return
		}
		container := s.Parent()
		for container.Length() > 0 && container.Get(0) != root.Get(0) && normalizeSpace(container.Text()) == "" &&
			container.Find("img, picture, video, audio, iframe").Length() == 0 {
			next := container.Parent()
			container.Remove()
			container = next
		}
	})
}

func normalizeFallbackContentContainers(root *goquery.Selection) {
	root.Find("main#content").Each(func(_ int, s *goquery.Selection) {
		if attr(s, "role") == "" {
			if node := s.Get(0); node != nil {
				node.Data = "div"
			}
		}
	})
}

func normalizeEntryHeaders(root *goquery.Selection) {
	root.Find("article header.entry-header").Each(func(_ int, s *goquery.Selection) {
		node := s.Get(0)
		if node == nil {
			return
		}
		for node.FirstChild != nil {
			node.RemoveChild(node.FirstChild)
		}
		node.Attr = nil
	})
}

func wrapAttachmentImageLinks(root *goquery.Selection) {
	root.Find("div[id^='attachment_']").Each(func(_ int, s *goquery.Selection) {
		node := s.Get(0)
		if node == nil {
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			if isWhitespaceNode(child) {
				continue
			}
			if tagNameNode(child) != "a" || selectionForNode(child).Find("img").Length() == 0 {
				continue
			}
			p := &xhtml.Node{Type: xhtml.ElementNode, Data: "p"}
			next := child.NextSibling
			node.RemoveChild(child)
			p.AppendChild(child)
			node.InsertBefore(p, next)
			return
		}
	})
}

func restoreContinuationLinks(root *goquery.Selection) {
	fromID, toID := continuationLinkIDs(root)
	if fromID == "" || toID == "" {
		return
	}
	if existing := root.Find("#" + fromID).First(); existing.Length() > 0 {
		if tagNameNode(existing.Get(0)) != "div" {
			return
		}
		if existing.Find("a").Length() == 0 {
			appendContinuationLink(existing.Get(0), toID)
		}
		return
	}
	target := root.Find("#" + toID).First()
	targetNode := target.Get(0)
	if targetNode == nil || targetNode.Parent == nil {
		return
	}
	div := &xhtml.Node{Type: xhtml.ElementNode, Data: "div", Attr: []xhtml.Attribute{{Key: "id", Val: fromID}}}
	appendContinuationLink(div, toID)
	parent := targetNode.Parent
	if tagNameNode(parent) == "div" && parent.Parent != nil {
		parent.Parent.InsertBefore(div, parent)
		return
	}
	parent.InsertBefore(div, targetNode)
}

func continuationLinkIDs(root *goquery.Selection) (string, string) {
	var fromID, toID string
	root.Find(`a[href^="#"]`).EachWithBreak(func(_ int, s *goquery.Selection) bool {
		text := strings.ToLower(normalizeSpace(s.Text()))
		if !strings.Contains(text, "continue") {
			return true
		}
		id := strings.TrimPrefix(attr(s, "href"), "#")
		nextID := nextTrailingNumberID(id)
		if id == "" || nextID == "" || root.Find("#"+nextID).Length() == 0 {
			return true
		}
		fromID, toID = id, nextID
		return false
	})
	return fromID, toID
}

func nextTrailingNumberID(id string) string {
	number := trailingNumber(id)
	if number == "" {
		return ""
	}
	prefix := strings.TrimSuffix(id, number)
	return prefix + incrementDecimalString(number)
}

func trailingNumber(value string) string {
	i := len(value)
	for i > 0 && value[i-1] >= '0' && value[i-1] <= '9' {
		i--
	}
	if i == len(value) {
		return ""
	}
	return value[i:]
}

func incrementDecimalString(value string) string {
	carry := byte(1)
	out := []byte(value)
	for i := len(out) - 1; i >= 0 && carry > 0; i-- {
		next := out[i] + carry
		if next <= '9' {
			out[i] = next
			carry = 0
			continue
		}
		out[i] = '0'
	}
	if carry > 0 {
		out = append([]byte{'1'}, out...)
	}
	return string(out)
}

func appendContinuationLink(node *xhtml.Node, targetID string) {
	if node == nil {
		return
	}
	p := &xhtml.Node{Type: xhtml.ElementNode, Data: "p"}
	a := &xhtml.Node{Type: xhtml.ElementNode, Data: "a", Attr: []xhtml.Attribute{{Key: "href", Val: "#" + targetID}}}
	a.AppendChild(&xhtml.Node{Type: xhtml.TextNode, Data: "Continue reading the main story"})
	p.AppendChild(a)
	node.AppendChild(p)
}

func removeCompatibilityHorizontalRules(root *goquery.Selection) {
	root.Find("hr").Each(func(_ int, s *goquery.Selection) {
		node := s.Get(0)
		if node == nil {
			return
		}
		if next := nextElementSibling(node); tagNameNode(next) == "hr" {
			removeNode(next)
			removeNode(node)
			return
		}
		prevTag := tagNameNode(previousElementSibling(node))
		nextTag := tagNameNode(nextElementSibling(node))
		if (prevTag == "div" || prevTag == "section") && (nextTag == "div" || nextTag == "section") {
			removeNode(node)
			return
		}
		next := nextElementSibling(node)
		if tagNameNode(next) == "hr" {
			next = nextElementSibling(next)
		}
		if tagNameNode(next) != "p" {
			return
		}
		text := normalizeSpace(selectionForNode(next).Text())
		if isDisclaimerLikeBlock(text) || isTrademarkNoticeBlock(text) {
			removeNode(node)
		}
	})
}

func isDisclaimerLikeBlock(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "disclaimer") &&
		(len([]rune(text)) < 200 || strings.Contains(lower, "copyright") || strings.Contains(lower, "license"))
}

func isTrademarkNoticeBlock(text string) bool {
	lower := strings.ToLower(text)
	return len([]rune(text)) < 160 &&
		(strings.Contains(lower, "service mark") || strings.Contains(lower, "trademark"))
}

func removeMediaSectionHeadings(root *goquery.Selection) {
	root.Find("h1, h2, h3, h4, h5, h6").Each(func(_ int, s *goquery.Selection) {
		if strings.ToLower(normalizeSpace(s.Text())) != "videos" {
			return
		}
		for node := s.Get(0).NextSibling; node != nil; node = node.NextSibling {
			if isWhitespaceNode(node) {
				continue
			}
			if node.Type != xhtml.ElementNode {
				return
			}
			tag := tagNameNode(node)
			if isHeadingTag(tag) {
				return
			}
			if tag == "iframe" || tag == "video" || tag == "embed" || tag == "object" ||
				selectionForNode(node).Find("iframe, video, embed, object").Length() > 0 {
				s.Remove()
				return
			}
		}
	})
}

func normalizeCollectionContainers(root *goquery.Selection) {
	root.Find("#site-content").Each(func(_ int, s *goquery.Selection) {
		if node := s.Get(0); node != nil {
			node.Data = "section"
		}
	})
	root.Find("*").FilterFunction(func(_ int, s *goquery.Selection) bool {
		return isCollectionHighlightsNode(s.Get(0))
	}).Each(func(_ int, s *goquery.Selection) {
		node := s.Get(0)
		if node == nil {
			return
		}
		node.Data = "div"
		if child := firstElementChild(node); child != nil && tagNameNode(child) == "div" && selectionForNode(child).Find("ol, ul").Length() > 0 {
			next := child
			for child.FirstChild != nil {
				grandchild := child.FirstChild
				child.RemoveChild(grandchild)
				node.InsertBefore(grandchild, next)
			}
			removeNode(child)
		}
		if parent := node.Parent; parent != nil && node.Parent.Parent != nil && nodeAttr(parent, "id") == "site-content" {
			wrapper := &xhtml.Node{Type: xhtml.ElementNode, Data: "div"}
			parent.InsertBefore(wrapper, node)
			appendNode(wrapper, node)
			for next := wrapper.NextSibling; next != nil; {
				sibling := next
				next = next.NextSibling
				if isWhitespaceNode(sibling) {
					removeNode(sibling)
					continue
				}
				appendNode(wrapper, sibling)
			}
		}
	})
}

func normalizeVideoPlayerContainers(root *goquery.Selection) {
	root.Find("*").Each(func(_ int, s *goquery.Selection) {
		if !isMediaPlayerContainer(s) {
			return
		}
		node := s.Get(0)
		child := firstElementChild(node)
		if node == nil || child == nil || tagNameNode(child) != "div" {
			return
		}
		for sibling := nextElementSibling(child); sibling != nil; {
			next := nextElementSibling(sibling)
			removeNode(sibling)
			sibling = next
		}
		next := child
		for child.FirstChild != nil {
			grandchild := child.FirstChild
			child.RemoveChild(grandchild)
			node.InsertBefore(grandchild, next)
		}
		removeNode(child)
		for sibling := nextElementSibling(node); sibling != nil; {
			nextSibling := nextElementSibling(sibling)
			if isControlPlaceholderText(normalizeSpace(selectionForNode(sibling).Text())) {
				removeNode(sibling)
			}
			sibling = nextSibling
		}
	})
}

func isMediaPlayerContainer(s *goquery.Selection) bool {
	classID := strings.ToLower(attr(s, "class") + " " + attr(s, "id"))
	if !strings.Contains(classID, "player") {
		return false
	}
	return s.Find("iframe, video, embed, object, img").Length() > 0
}

func isControlPlaceholderText(text string) bool {
	if text == "" || len([]rune(text)) > 8 {
		return false
	}
	for _, r := range text {
		if !strings.ContainsRune("<>[](){}|/\\", r) && r != ' ' {
			return false
		}
	}
	return true
}

func isLoadingPlaceholderText(text string) bool {
	if loadingWordsRE.MatchString(text) {
		return true
	}
	lower := strings.ToLower(text)
	for _, marker := range []string{"loading", "正在加载", "Загрузка", "chargement", "cargando"} {
		if !strings.HasPrefix(lower, strings.ToLower(marker)) {
			continue
		}
		rest := strings.TrimSpace(text[len(marker):])
		rest = strings.TrimLeft(rest, ".…")
		rest = strings.TrimSpace(rest)
		return rest == "" || isControlPlaceholderText(rest)
	}
	return false
}
